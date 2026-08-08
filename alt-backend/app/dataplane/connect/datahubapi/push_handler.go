package datahubapi

import (
	"context"
	"errors"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/dataplane/usecase/push_delivery_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// WithPushCapabilities wires Web Push storage: the per-device subscription
// table and the dispatcher's delivery queue.
//
// Both arguments are required and a nil one panics, like the Wave 3 options
// above and unlike the WithPhaseN ones. A nil subscription port would make
// RegisterSubscription answer Unimplemented — the same answer a retired
// procedure gives — so a user would grant notification permission, see the
// toggle fail, and no log anywhere would say the storage was never wired. A
// nil delivery port is quieter still: the dispatcher would claim nothing,
// which is indistinguishable from a drained queue (CLAUDE.md rule 8,
// ADR-000928).
func WithPushCapabilities(
	pushSubscriptions datahub_capability_port.PushSubscriptionPort,
	pushDeliveries *push_delivery_usecase.PushDeliveryUsecase,
) HandlerOption {
	switch {
	case pushSubscriptions == nil:
		panic("datahubapi: PushSubscriptionPort is required — alt.push.v1.PushService has no other route to push_subscriptions")
	case pushDeliveries == nil:
		panic("datahubapi: PushDeliveryUsecase is required — the Web Push dispatcher has no other route to push_deliveries")
	}

	return func(h *Handler) {
		h.pushSubscriptions = pushSubscriptions
		h.pushDeliveries = pushDeliveries
	}
}

// ---------------------------------------------------------------------------
// push_subscriptions
// ---------------------------------------------------------------------------

// UpsertPushSubscription stores a browser subscription for one user.
//
// user_id is required on the request rather than read from the transport: the
// peer certificate names alt-backend, so scoping this to the transport
// identity would file every device under one service account.
func (h *Handler) UpsertPushSubscription(ctx context.Context, req *connect.Request[datahubv1.UpsertPushSubscriptionRequest]) (*connect.Response[datahubv1.UpsertPushSubscriptionResponse], error) {
	msg := req.Msg.GetSubscription()
	if msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("subscription is required"))
	}
	switch {
	case msg.GetUserId() == "":
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("subscription.user_id is required"))
	case msg.GetEndpoint() == "":
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("subscription.endpoint is required"))
	case msg.GetP256Dh() == "" || msg.GetAuth() == "":
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("subscription.p256dh and subscription.auth are required: a subscription without RFC 8291 key material cannot be encrypted to"))
	case msg.GetVapidKeyFingerprint() == "":
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("subscription.vapid_key_fingerprint is required: a row that does not record which keypair it was created under cannot be invalidated by a rotation"))
	}

	created, err := h.pushSubscriptions.Upsert(ctx, domain.PushSubscription{
		UserID:              msg.GetUserId(),
		Endpoint:            msg.GetEndpoint(),
		P256dh:              msg.GetP256Dh(),
		Auth:                msg.GetAuth(),
		Preferences:         notificationPreferencesFromProto(msg.GetPreferences()),
		VAPIDKeyFingerprint: msg.GetVapidKeyFingerprint(),
	})
	switch {
	case errors.Is(err, domain.ErrPushSubscriptionOwnedByAnotherUser):
		// Neither the log line nor the response names the endpoint. It is a
		// capability URL, and this is exactly the path a caller replaying a
		// leaked one would take.
		h.logger.WarnContext(ctx, "UpsertPushSubscription rejected", "user_id", msg.GetUserId(),
			"reason", "endpoint belongs to a different user")
		return nil, connect.NewError(connect.CodeAlreadyExists, domain.ErrPushSubscriptionOwnedByAnotherUser)
	case err != nil:
		h.logger.ErrorContext(ctx, "UpsertPushSubscription failed", "error", err, "user_id", msg.GetUserId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to store push subscription"))
	}

	return connect.NewResponse(&datahubv1.UpsertPushSubscriptionResponse{Created: created}), nil
}

// GetPushSubscription answers with an empty envelope when this user has no
// subscription at that endpoint. The caller reads the absence as "not
// subscribed on this device", which is a different screen from an error.
func (h *Handler) GetPushSubscription(ctx context.Context, req *connect.Request[datahubv1.GetPushSubscriptionRequest]) (*connect.Response[datahubv1.GetPushSubscriptionResponse], error) {
	if err := requireUserAndEndpoint(req.Msg.GetUserId(), req.Msg.GetEndpoint()); err != nil {
		return nil, err
	}

	sub, err := h.pushSubscriptions.Get(ctx, req.Msg.GetUserId(), req.Msg.GetEndpoint())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetPushSubscription failed", "error", err, "user_id", req.Msg.GetUserId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get push subscription"))
	}

	resp := &datahubv1.GetPushSubscriptionResponse{}
	if sub != nil {
		resp.Subscription = pushSubscriptionToProto(*sub)
	}
	return connect.NewResponse(resp), nil
}

// UpdatePushSubscriptionPreferences writes all four booleans.
//
// It is a whole-set write rather than a patch on purpose: protoJSON omits
// false, so "turn the digest off" and "do not mention the digest" are the same
// bytes, and a per-field update would have to guess which one the caller meant.
func (h *Handler) UpdatePushSubscriptionPreferences(ctx context.Context, req *connect.Request[datahubv1.UpdatePushSubscriptionPreferencesRequest]) (*connect.Response[datahubv1.UpdatePushSubscriptionPreferencesResponse], error) {
	if err := requireUserAndEndpoint(req.Msg.GetUserId(), req.Msg.GetEndpoint()); err != nil {
		return nil, err
	}

	updated, err := h.pushSubscriptions.UpdatePreferences(ctx,
		req.Msg.GetUserId(), req.Msg.GetEndpoint(),
		notificationPreferencesFromProto(req.Msg.GetPreferences()))
	if err != nil {
		h.logger.ErrorContext(ctx, "UpdatePushSubscriptionPreferences failed", "error", err, "user_id", req.Msg.GetUserId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update push subscription preferences"))
	}

	return connect.NewResponse(&datahubv1.UpdatePushSubscriptionPreferencesResponse{Updated: updated}), nil
}

// DeletePushSubscription is idempotent: deleted=false means there was nothing
// to remove, which the dispatcher reaches whenever a 410 races the user's own
// removal.
func (h *Handler) DeletePushSubscription(ctx context.Context, req *connect.Request[datahubv1.DeletePushSubscriptionRequest]) (*connect.Response[datahubv1.DeletePushSubscriptionResponse], error) {
	if err := requireUserAndEndpoint(req.Msg.GetUserId(), req.Msg.GetEndpoint()); err != nil {
		return nil, err
	}

	deleted, err := h.pushSubscriptions.Delete(ctx, req.Msg.GetUserId(), req.Msg.GetEndpoint())
	if err != nil {
		h.logger.ErrorContext(ctx, "DeletePushSubscription failed", "error", err, "user_id", req.Msg.GetUserId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete push subscription"))
	}

	return connect.NewResponse(&datahubv1.DeletePushSubscriptionResponse{Deleted: deleted}), nil
}

func (h *Handler) ListPushSubscriptionsForUser(ctx context.Context, req *connect.Request[datahubv1.ListPushSubscriptionsForUserRequest]) (*connect.Response[datahubv1.ListPushSubscriptionsForUserResponse], error) {
	if req.Msg.GetUserId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}

	subs, err := h.pushSubscriptions.ListForUser(ctx, req.Msg.GetUserId())
	if err != nil {
		h.logger.ErrorContext(ctx, "ListPushSubscriptionsForUser failed", "error", err, "user_id", req.Msg.GetUserId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list push subscriptions"))
	}

	out := make([]*datahubv1.PushSubscription, 0, len(subs))
	for _, sub := range subs {
		out = append(out, pushSubscriptionToProto(sub))
	}
	return connect.NewResponse(&datahubv1.ListPushSubscriptionsForUserResponse{Subscriptions: out}), nil
}

func requireUserAndEndpoint(userID, endpoint string) error {
	if userID == "" || endpoint == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("user_id and endpoint are required"))
	}
	return nil
}

func notificationPreferencesFromProto(p *datahubv1.NotificationPreferences) domain.NotificationPreferences {
	// An absent message means every kind is off. protoJSON omits false, so a
	// subscription with all four turned off arrives with no preferences block
	// at all; defaulting that to "everything on" would opt a user back into
	// exactly what they opted out of.
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

func pushSubscriptionToProto(sub domain.PushSubscription) *datahubv1.PushSubscription {
	return &datahubv1.PushSubscription{
		UserId:   sub.UserID,
		Endpoint: sub.Endpoint,
		P256Dh:   sub.P256dh,
		Auth:     sub.Auth,
		Preferences: &datahubv1.NotificationPreferences{
			SummaryReady:       sub.Preferences.SummaryReady,
			AcolyteReportReady: sub.Preferences.AcolyteReportReady,
			RecapReady:         sub.Preferences.RecapReady,
			TodayEntranceReady: sub.Preferences.TodayEntranceReady,
		},
		VapidKeyFingerprint: sub.VAPIDKeyFingerprint,
		CreatedAt:           timestampOrNil(sub.CreatedAt),
		UpdatedAt:           timestampOrNil(sub.UpdatedAt),
		LastSuccessAt:       timestampOrNil(sub.LastSuccessAt),
		LastFailureAt:       timestampOrNil(sub.LastFailureAt),
	}
}

// ---------------------------------------------------------------------------
// push_deliveries
// ---------------------------------------------------------------------------

// EnqueueNotification fans one notification out to every device of one user
// that still wants that kind.
func (h *Handler) EnqueueNotification(ctx context.Context, req *connect.Request[datahubv1.EnqueueNotificationRequest]) (*connect.Response[datahubv1.EnqueueNotificationResponse], error) {
	delivered, superseded, err := h.pushDeliveries.Enqueue(ctx, domain.NotificationEnqueue{
		DedupeKey:  req.Msg.GetDedupeKey(),
		UserID:     req.Msg.GetUserId(),
		Kind:       req.Msg.GetKind(),
		Payload:    req.Msg.GetPayload(),
		OccurredAt: timeOrZero(req.Msg.GetOccurredAt()),
		ExpiresAt:  timeOrZero(req.Msg.GetExpiresAt()),
	})
	switch {
	case errors.Is(err, push_delivery_usecase.ErrInvalidEnqueue), errors.Is(err, push_delivery_usecase.ErrUnknownKind):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "EnqueueNotification failed", "error", err,
			"dedupe_key", req.Msg.GetDedupeKey(), "kind", req.Msg.GetKind())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to enqueue notification"))
	}

	return connect.NewResponse(&datahubv1.EnqueueNotificationResponse{
		DeliveryCount:   safeconv.Int32(delivered),
		SupersededCount: safeconv.Int32(superseded),
	}), nil
}

// ClaimNotificationBatch takes the delivery lease. The rows come back already
// SENDING — the lease and the state change are one statement — so the caller
// never observes a claimed row as pending.
func (h *Handler) ClaimNotificationBatch(ctx context.Context, req *connect.Request[datahubv1.ClaimNotificationBatchRequest]) (*connect.Response[datahubv1.ClaimNotificationBatchResponse], error) {
	lease := durationFromSeconds(req.Msg.GetLeaseSeconds())

	deliveries, err := h.pushDeliveries.ClaimBatch(ctx, req.Msg.GetLockedBy(), int(req.Msg.GetLimit()), lease)
	switch {
	case errors.Is(err, push_delivery_usecase.ErrMissingLockedBy):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "ClaimNotificationBatch failed", "error", err, "locked_by", req.Msg.GetLockedBy())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to claim notification batch"))
	}

	out := make([]*datahubv1.PushDelivery, 0, len(deliveries))
	for _, d := range deliveries {
		out = append(out, &datahubv1.PushDelivery{
			Id:             d.ID,
			DedupeKey:      d.DedupeKey,
			SubscriptionId: d.SubscriptionID,
			UserId:         d.UserID,
			Kind:           d.Kind,
			Payload:        d.Payload,
			OccurredAt:     timestampOrNil(d.OccurredAt),
			State:          notificationStateToProto(d.State),
			Attempts:       safeconv.Int32(d.Attempts),
			NextAttemptAt:  timestampOrNil(d.NextAttemptAt),
			ExpiresAt:      timestampOrNil(d.ExpiresAt),
			Endpoint:       d.Endpoint,
			P256Dh:         d.P256dh,
			Auth:           d.Auth,
		})
	}
	return connect.NewResponse(&datahubv1.ClaimNotificationBatchResponse{Deliveries: out}), nil
}

func (h *Handler) MarkNotificationSent(ctx context.Context, req *connect.Request[datahubv1.MarkNotificationSentRequest]) (*connect.Response[datahubv1.MarkNotificationSentResponse], error) {
	err := h.pushDeliveries.MarkSent(ctx, req.Msg.GetId(), int(req.Msg.GetStatusCode()))
	switch {
	case errors.Is(err, push_delivery_usecase.ErrMissingID):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "MarkNotificationSent failed", "error", err, "delivery_id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to mark notification sent"))
	}
	return connect.NewResponse(&datahubv1.MarkNotificationSentResponse{}), nil
}

func (h *Handler) ReleaseNotification(ctx context.Context, req *connect.Request[datahubv1.ReleaseNotificationRequest]) (*connect.Response[datahubv1.ReleaseNotificationResponse], error) {
	err := h.pushDeliveries.Release(ctx, req.Msg.GetId(),
		timeOrZero(req.Msg.GetNextAttemptAt()), req.Msg.GetErrorMessage())
	switch {
	case errors.Is(err, push_delivery_usecase.ErrMissingID), errors.Is(err, push_delivery_usecase.ErrMissingNextAttempt):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "ReleaseNotification failed", "error", err, "delivery_id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to release notification"))
	}
	return connect.NewResponse(&datahubv1.ReleaseNotificationResponse{}), nil
}

func (h *Handler) MarkNotificationDead(ctx context.Context, req *connect.Request[datahubv1.MarkNotificationDeadRequest]) (*connect.Response[datahubv1.MarkNotificationDeadResponse], error) {
	err := h.pushDeliveries.MarkDead(ctx, req.Msg.GetId(),
		int(req.Msg.GetStatusCode()), req.Msg.GetErrorMessage())
	switch {
	case errors.Is(err, push_delivery_usecase.ErrMissingID):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "MarkNotificationDead failed", "error", err, "delivery_id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to mark notification dead"))
	}
	return connect.NewResponse(&datahubv1.MarkNotificationDeadResponse{}), nil
}

// GetNotificationBacklogAge answers the dispatcher's backlog gauge: how stale
// the oldest undelivered notification is, and how many are waiting.
//
// A failed read is an error rather than a zero age. Zero is what a healthy
// drained queue reports, so answering it here would turn a broken data plane
// into a gauge saying there is nothing to deliver.
func (h *Handler) GetNotificationBacklogAge(ctx context.Context, _ *connect.Request[datahubv1.GetNotificationBacklogAgeRequest]) (*connect.Response[datahubv1.GetNotificationBacklogAgeResponse], error) {
	oldest, pending, err := h.pushDeliveries.BacklogAge(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetNotificationBacklogAge failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to read notification backlog age"))
	}

	return connect.NewResponse(&datahubv1.GetNotificationBacklogAgeResponse{
		OldestPendingAgeSeconds: oldest.Seconds(),
		PendingCount:            pending,
	}), nil
}

// durationFromSeconds converts a lease window. Zero reaches the usecase, which
// substitutes its default — unlike retentionFromSeconds above, where zero has
// to be rejected because it would delete every row the query matches. Here the
// worst a zero does is take the default lease.
func durationFromSeconds(seconds int64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func notificationStateToProto(s domain.NotificationState) datahubv1.NotificationState {
	switch s {
	case domain.NotificationPending:
		return datahubv1.NotificationState_NOTIFICATION_STATE_PENDING
	case domain.NotificationSending:
		return datahubv1.NotificationState_NOTIFICATION_STATE_SENDING
	case domain.NotificationSent:
		return datahubv1.NotificationState_NOTIFICATION_STATE_SENT
	case domain.NotificationDead:
		return datahubv1.NotificationState_NOTIFICATION_STATE_DEAD
	case domain.NotificationExpired:
		return datahubv1.NotificationState_NOTIFICATION_STATE_EXPIRED
	default:
		return datahubv1.NotificationState_NOTIFICATION_STATE_UNSPECIFIED
	}
}
