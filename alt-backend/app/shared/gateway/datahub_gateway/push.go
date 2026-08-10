package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// PushSubscriptionGateway is cmd/backend's route to push_subscriptions.
//
// userID is an argument on every method rather than something read from the
// context. alt-data-hub authenticates the peer certificate, which names
// alt-backend; the person whose devices these are comes from the
// X-Alt-Backend-Token subject that alt-backend already validated, so it has to
// travel explicitly or every caller would resolve to one service account.
//
// Nothing here logs the endpoint. It is a capability URL — whoever holds it
// can push to that device — and the errors below name the operation rather
// than the row.
type PushSubscriptionGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewPushSubscriptionGateway(client datahubv1connect.DataHubServiceClient) *PushSubscriptionGateway {
	if client == nil {
		panic("datahub_gateway: PushSubscriptionGateway requires a DataHubService client — " +
			"a nil client would make every subscribe answer an error indistinguishable from " +
			"a browser that never granted permission (see .claude/rules/di-wiring.md)")
	}
	return &PushSubscriptionGateway{client: client}
}

// Upsert stores a subscription, replacing the key material and preferences of
// an endpoint already registered. The bool reports whether the row was new.
func (g *PushSubscriptionGateway) Upsert(ctx context.Context, sub domain.PushSubscription) (bool, error) {
	resp, err := g.client.UpsertPushSubscription(ctx, connect.NewRequest(&datahubv1.UpsertPushSubscriptionRequest{
		Subscription: &datahubv1.PushSubscription{
			UserId:              sub.UserID,
			Endpoint:            sub.Endpoint,
			P256Dh:              sub.P256dh,
			Auth:                sub.Auth,
			Preferences:         preferencesToProto(sub.Preferences),
			VapidKeyFingerprint: sub.VAPIDKeyFingerprint,
		},
	}))
	if err != nil {
		return false, fmt.Errorf("upsert push subscription: %w", err)
	}
	return resp.Msg.GetCreated(), nil
}

// Get returns one device's subscription, or nil when this user has none at
// that endpoint. Nil is not an error: the settings page reads it as "not
// subscribed on this device" and offers the subscribe button.
func (g *PushSubscriptionGateway) Get(ctx context.Context, userID, endpoint string) (*domain.PushSubscription, error) {
	resp, err := g.client.GetPushSubscription(ctx, connect.NewRequest(&datahubv1.GetPushSubscriptionRequest{
		UserId:   userID,
		Endpoint: endpoint,
	}))
	if err != nil {
		return nil, fmt.Errorf("get push subscription: %w", err)
	}
	msg := resp.Msg.GetSubscription()
	if msg == nil {
		return nil, nil
	}
	sub := pushSubscriptionFromProto(msg)
	return &sub, nil
}

// UpdatePreferences changes which kinds are delivered without touching the key
// material. The bool is false when the pair matched no row, which the caller
// turns into NotFound rather than reporting a change that did not happen.
func (g *PushSubscriptionGateway) UpdatePreferences(ctx context.Context, userID, endpoint string, prefs domain.NotificationPreferences) (bool, error) {
	resp, err := g.client.UpdatePushSubscriptionPreferences(ctx,
		connect.NewRequest(&datahubv1.UpdatePushSubscriptionPreferencesRequest{
			UserId:      userID,
			Endpoint:    endpoint,
			Preferences: preferencesToProto(prefs),
		}))
	if err != nil {
		return false, fmt.Errorf("update push subscription preferences: %w", err)
	}
	return resp.Msg.GetUpdated(), nil
}

// Delete removes one device. Idempotent: false means there was nothing to
// delete, which is not an error.
func (g *PushSubscriptionGateway) Delete(ctx context.Context, userID, endpoint string) (bool, error) {
	resp, err := g.client.DeletePushSubscription(ctx, connect.NewRequest(&datahubv1.DeletePushSubscriptionRequest{
		UserId:   userID,
		Endpoint: endpoint,
	}))
	if err != nil {
		return false, fmt.Errorf("delete push subscription: %w", err)
	}
	return resp.Msg.GetDeleted(), nil
}

// ListForUser returns every device of one user — the fan-out list.
func (g *PushSubscriptionGateway) ListForUser(ctx context.Context, userID string) ([]domain.PushSubscription, error) {
	resp, err := g.client.ListPushSubscriptionsForUser(ctx,
		connect.NewRequest(&datahubv1.ListPushSubscriptionsForUserRequest{UserId: userID}))
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions for user %s: %w", userID, err)
	}

	subs := make([]domain.PushSubscription, 0, len(resp.Msg.GetSubscriptions()))
	for _, msg := range resp.Msg.GetSubscriptions() {
		subs = append(subs, pushSubscriptionFromProto(msg))
	}
	return subs, nil
}

// PushDeliveryGateway is the Web Push dispatcher's route to push_deliveries.
//
// push_deliveries is the dispatcher's queue, not a producer outbox: the
// producers' business state lives in pre-processor-db, acolyte-db and
// recap-db, so each keeps its own outbox beside the state it describes and a
// relay forwards to data-hub. Enqueue here is the relay's call, one per
// notification, fanned out provider-side to every device.
//
// It has no caller in this repository yet — the dispatcher and the relay are a
// separate change. It is written now, with the storage it drives, for the
// reason AutoFulltextGateway above gives: the alternative is a dispatcher
// built later against a wire shape nothing has ever exercised, and the CDC
// pact recorded against this gateway is what pins the claim/lease semantics
// before there is a second implementation to disagree with.
//
// Because it is unwired, no composition root constructs it, and that is
// visible rather than silent: there is no DI option that could be left nil and
// no branch that could no-op, so nothing can report this capability as enabled
// while it is absent.
type PushDeliveryGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewPushDeliveryGateway(client datahubv1connect.DataHubServiceClient) *PushDeliveryGateway {
	if client == nil {
		panic("datahub_gateway: PushDeliveryGateway requires a DataHubService client — " +
			"a nil client would make every dispatcher tick fail identically to an empty queue " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &PushDeliveryGateway{client: client}
}

// Enqueue fans one notification out to a user's devices, returning how many
// rows were created and how many older unsent digests were superseded.
//
// A zero delivery count is an ordinary answer: the user may have no device,
// may have turned the kind off, or the relay may have delivered this enqueue
// twice.
func (g *PushDeliveryGateway) Enqueue(ctx context.Context, in domain.NotificationEnqueue) (delivered, superseded int, err error) {
	resp, err := g.client.EnqueueNotification(ctx, connect.NewRequest(&datahubv1.EnqueueNotificationRequest{
		DedupeKey:  in.DedupeKey,
		UserId:     in.UserID,
		Kind:       in.Kind,
		Payload:    in.Payload,
		OccurredAt: timeToProto(in.OccurredAt),
		ExpiresAt:  timeToProto(in.ExpiresAt),
	}))
	if err != nil {
		return 0, 0, fmt.Errorf("enqueue notification %s: %w", in.DedupeKey, err)
	}
	return int(resp.Msg.GetDeliveryCount()), int(resp.Msg.GetSupersededCount()), nil
}

// ClaimBatch takes ownership of up to limit due rows for the length of the
// lease. Rows come back already SENDING, with the key material needed to send
// them.
func (g *PushDeliveryGateway) ClaimBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.PushDelivery, error) {
	resp, err := g.client.ClaimNotificationBatch(ctx, connect.NewRequest(&datahubv1.ClaimNotificationBatchRequest{
		LockedBy:     lockedBy,
		Limit:        safeconv.Int32(limit),
		LeaseSeconds: int64(lease.Seconds()),
	}))
	if err != nil {
		return nil, fmt.Errorf("claim notification batch: %w", err)
	}

	deliveries := make([]domain.PushDelivery, 0, len(resp.Msg.GetDeliveries()))
	for _, msg := range resp.Msg.GetDeliveries() {
		deliveries = append(deliveries, pushDeliveryFromProto(msg))
	}
	return deliveries, nil
}

// MarkSent records delivery. statusCode is the push service's, kept even on
// success as the forensic record of which service accepted it.
func (g *PushDeliveryGateway) MarkSent(ctx context.Context, id string, statusCode int) error {
	_, err := g.client.MarkNotificationSent(ctx, connect.NewRequest(&datahubv1.MarkNotificationSentRequest{
		Id:         id,
		StatusCode: safeconv.Int32(statusCode),
	}))
	if err != nil {
		return fmt.Errorf("mark push delivery %s sent: %w", id, err)
	}
	return nil
}

// Release returns a claimed row to PENDING with the caller's backoff.
func (g *PushDeliveryGateway) Release(ctx context.Context, id string, nextAttemptAt time.Time, errorMessage string) error {
	_, err := g.client.ReleaseNotification(ctx, connect.NewRequest(&datahubv1.ReleaseNotificationRequest{
		Id:            id,
		NextAttemptAt: timeToProto(nextAttemptAt),
		ErrorMessage:  errorMessage,
	}))
	if err != nil {
		return fmt.Errorf("release push delivery %s: %w", id, err)
	}
	return nil
}

// MarkDead ends delivery for a failure that will not improve. statusCode is 0
// when the attempt never reached a response.
func (g *PushDeliveryGateway) MarkDead(ctx context.Context, id string, statusCode int, errorMessage string) error {
	_, err := g.client.MarkNotificationDead(ctx, connect.NewRequest(&datahubv1.MarkNotificationDeadRequest{
		Id:           id,
		StatusCode:   safeconv.Int32(statusCode),
		ErrorMessage: errorMessage,
	}))
	if err != nil {
		return fmt.Errorf("mark push delivery %s dead: %w", id, err)
	}
	return nil
}

// BacklogAge reports how stale the delivery queue is, how deep it is, and how
// many devices are registered to receive from it.
//
// The provider counts rows in `sending` as well as `pending`, which is what
// makes the number usable: a row orphaned by a dispatcher that died mid-attempt
// stays `sending` until its lease elapses, and an age blind to those would read
// 0 for a queue nothing is draining.
//
// (0, 0) is a drained queue, not a failed read — the caller publishes the gauge
// on every tick including that one, because a gauge that stops being set keeps
// reporting its last value for as long as the process is up.
func (g *PushDeliveryGateway) BacklogAge(ctx context.Context) (oldestAge time.Duration, pending, subscriptions int64, err error) {
	resp, err := g.client.GetNotificationBacklogAge(ctx,
		connect.NewRequest(&datahubv1.GetNotificationBacklogAgeRequest{}))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read push delivery backlog age: %w", err)
	}
	return time.Duration(resp.Msg.GetOldestPendingAgeSeconds() * float64(time.Second)),
		resp.Msg.GetPendingCount(),
		resp.Msg.GetActiveSubscriptionCount(),
		nil
}

// ---------------------------------------------------------------------------
// Wire conversion
// ---------------------------------------------------------------------------

func preferencesToProto(p domain.NotificationPreferences) *datahubv1.NotificationPreferences {
	return &datahubv1.NotificationPreferences{
		SummaryReady:       p.SummaryReady,
		AcolyteReportReady: p.AcolyteReportReady,
		RecapReady:         p.RecapReady,
		TodayEntranceReady: p.TodayEntranceReady,
	}
}

// preferencesFromProto reads an absent message as "everything off".
//
// protoJSON omits false, so a subscription with the daily digest turned off
// arrives with that key missing and one with every kind off arrives with no
// preferences block at all. Defaulting the nil case to "everything on" would
// turn a user who opted out of all four into one who opted into all four.
func preferencesFromProto(p *datahubv1.NotificationPreferences) domain.NotificationPreferences {
	if p == nil {
		return domain.NotificationPreferences{}
	}
	return domain.NotificationPreferences{
		SummaryReady:       p.GetSummaryReady(),
		AcolyteReportReady: p.GetAcolyteReportReady(),
		RecapReady:         p.GetRecapReady(),
		TodayEntranceReady: p.GetTodayEntranceReady(),
	}
}

func pushSubscriptionFromProto(msg *datahubv1.PushSubscription) domain.PushSubscription {
	return domain.PushSubscription{
		UserID:              msg.GetUserId(),
		Endpoint:            msg.GetEndpoint(),
		P256dh:              msg.GetP256Dh(),
		Auth:                msg.GetAuth(),
		Preferences:         preferencesFromProto(msg.GetPreferences()),
		VAPIDKeyFingerprint: msg.GetVapidKeyFingerprint(),
		CreatedAt:           timeFromProto(msg.GetCreatedAt()),
		UpdatedAt:           timeFromProto(msg.GetUpdatedAt()),
		LastSuccessAt:       timeFromProto(msg.GetLastSuccessAt()),
		LastFailureAt:       timeFromProto(msg.GetLastFailureAt()),
	}
}

func pushDeliveryFromProto(msg *datahubv1.PushDelivery) domain.PushDelivery {
	return domain.PushDelivery{
		ID:             msg.GetId(),
		DedupeKey:      msg.GetDedupeKey(),
		SubscriptionID: msg.GetSubscriptionId(),
		UserID:         msg.GetUserId(),
		Kind:           msg.GetKind(),
		Payload:        msg.GetPayload(),
		OccurredAt:     timeFromProto(msg.GetOccurredAt()),
		State:          notificationStateFromProto(msg.GetState()),
		Attempts:       int(msg.GetAttempts()),
		NextAttemptAt:  timeFromProto(msg.GetNextAttemptAt()),
		ExpiresAt:      timeFromProto(msg.GetExpiresAt()),
		Endpoint:       msg.GetEndpoint(),
		P256dh:         msg.GetP256Dh(),
		Auth:           msg.GetAuth(),
	}
}

// notificationStateFromProto maps an unspecified or unrecognised enum to the
// empty state. Defaulting to PENDING would turn "this build does not know the
// state the provider sent" into "this row is ready to send", which is the one
// answer that causes a second push.
func notificationStateFromProto(s datahubv1.NotificationState) domain.NotificationState {
	switch s {
	case datahubv1.NotificationState_NOTIFICATION_STATE_PENDING:
		return domain.NotificationPending
	case datahubv1.NotificationState_NOTIFICATION_STATE_SENDING:
		return domain.NotificationSending
	case datahubv1.NotificationState_NOTIFICATION_STATE_SENT:
		return domain.NotificationSent
	case datahubv1.NotificationState_NOTIFICATION_STATE_DEAD:
		return domain.NotificationDead
	case datahubv1.NotificationState_NOTIFICATION_STATE_EXPIRED:
		return domain.NotificationExpired
	default:
		return ""
	}
}
