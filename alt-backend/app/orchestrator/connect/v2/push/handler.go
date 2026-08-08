// Package push implements alt.push.v1.PushService, the browser-facing Web
// Push surface cmd/backend serves behind the JWT auth interceptor.
//
// Two rules shape every procedure here.
//
// The caller's identity comes from middleware.GetUserContext — the
// X-Alt-Backend-Token subject the auth interceptor validated — and never from
// a header the browser could set. X-Alt-User-Id exists on the internal hops
// and is not consulted: trusting it here would let anyone who can reach this
// port register a device against any account.
//
// The endpoint is a capability URL: whoever holds it can push to that device.
// Nothing in this package logs it, puts it in an error message, or attaches it
// to a span. The log lines below carry the user id and the procedure, which is
// enough to correlate an incident without writing the capability into the log
// store.
package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"

	"connectrpc.com/connect"

	"alt/connect/v2/middleware"
	"alt/domain"
	pushv1 "alt/gen/proto/alt/push/v1"
	"alt/gen/proto/alt/push/v1/pushv1connect"
	"alt/orchestrator/port/push_port"
)

// Handler serves alt.push.v1.PushService.
type Handler struct {
	subscriptions push_port.PushSubscriptionPort
	// vapidPublicKey is what the browser passes as applicationServerKey.
	vapidPublicKey string
	// vapidFingerprint is stamped on every stored subscription so a rotation
	// can be detected. Derived once at construction rather than per request:
	// it is a pure function of the key, and hashing it on every call would
	// make a hot read do cryptography for a constant.
	vapidFingerprint string
	logger           *slog.Logger
}

var _ pushv1connect.PushServiceHandler = (*Handler)(nil)

// NewHandler builds the push handler and refuses to start without either
// dependency.
//
// The nil checks are panics rather than a stored nil for the reason
// .claude/rules/di-wiring.md gives: a nil port would make RegisterSubscription
// fail on the first user who granted notification permission, weeks after
// deploy, with nothing at startup saying the storage was never wired. An empty
// VAPID key is worse than a missing one — GetPushConfig would answer 200 with
// an empty string, the browser would reject it inside pushManager.subscribe,
// and the only trace would be in that user's console. config.ValidateBackendConfig
// already rejects it before this is reached, so arriving here means the config
// guard was bypassed.
func NewHandler(subscriptions push_port.PushSubscriptionPort, vapidPublicKey string, logger *slog.Logger) *Handler {
	switch {
	case subscriptions == nil:
		panic("push: PushSubscriptionPort is required — alt.push.v1.PushService has no other route to a subscription")
	case vapidPublicKey == "":
		panic("push: VAPID_PUBLIC_KEY is required — GetPushConfig would otherwise answer an empty " +
			"applicationServerKey, which every browser rejects inside pushManager.subscribe")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		subscriptions:    subscriptions,
		vapidPublicKey:   vapidPublicKey,
		vapidFingerprint: VAPIDFingerprint(vapidPublicKey),
		logger:           logger,
	}
}

// VAPIDFingerprint is the short, stable identifier of a VAPID public key that
// every subscription row records.
//
// It is a truncated SHA-256 rather than the key itself so the value is short
// enough to index and to read in a log, and it is a hash rather than a
// configured version number so that "the key changed" cannot be forgotten:
// there is no second thing to bump.
func VAPIDFingerprint(publicKey string) string {
	sum := sha256.Sum256([]byte(publicKey))
	return hex.EncodeToString(sum[:6])
}

// GetPushConfig returns the VAPID public key, plus this device's current
// settings when the browser already holds a subscription.
//
// One round trip answers both questions because that is what the settings page
// needs on load: which key to subscribe with, and whether this device is
// already registered.
func (h *Handler) GetPushConfig(ctx context.Context, req *connect.Request[pushv1.GetPushConfigRequest]) (*connect.Response[pushv1.GetPushConfigResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	resp := &pushv1.GetPushConfigResponse{VapidPublicKey: h.vapidPublicKey}

	// No endpoint means "I have no subscription yet, just give me the key".
	// That is not an error and must not become one: it is the state every
	// first-time visitor is in.
	if req.Msg.GetEndpoint() == "" {
		return connect.NewResponse(resp), nil
	}

	sub, err := h.subscriptions.Get(ctx, user.UserID.String(), req.Msg.GetEndpoint())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetPushConfig failed", "error", err, "user_id", user.UserID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to read push subscription"))
	}
	if sub != nil {
		resp.HasSubscription = true
		resp.Preferences = preferencesToProto(sub.Preferences)
	}
	return connect.NewResponse(resp), nil
}

// RegisterSubscription stores this browser's subscription for the calling user.
//
// The fingerprint is stamped here rather than taken from the request: it is a
// property of the key this process served, and a client-supplied one would let
// a stale bundle claim a subscription was created under the current key.
func (h *Handler) RegisterSubscription(ctx context.Context, req *connect.Request[pushv1.RegisterSubscriptionRequest]) (*connect.Response[pushv1.RegisterSubscriptionResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	keys := req.Msg.GetKeys()
	switch {
	case req.Msg.GetEndpoint() == "":
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint is required"))
	case keys == nil || keys.GetP256Dh() == "" || keys.GetAuth() == "":
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("keys.p256dh and keys.auth are required: a subscription without them cannot be encrypted to"))
	}

	created, err := h.subscriptions.Upsert(ctx, domain.PushSubscription{
		UserID:              user.UserID.String(),
		Endpoint:            req.Msg.GetEndpoint(),
		P256dh:              keys.GetP256Dh(),
		Auth:                keys.GetAuth(),
		Preferences:         preferencesFromProto(req.Msg.GetPreferences()),
		VAPIDKeyFingerprint: h.vapidFingerprint,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "RegisterSubscription failed", "error", err, "user_id", user.UserID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to register push subscription"))
	}

	h.logger.InfoContext(ctx, "push_subscription_registered",
		"user_id", user.UserID, "created", created, "vapid_key_fingerprint", h.vapidFingerprint)
	return connect.NewResponse(&pushv1.RegisterSubscriptionResponse{}), nil
}

// UpdatePreferences changes which kinds this device receives.
//
// A pair that matches no row is NotFound rather than a silent success: the
// browser holds a subscription this server does not know about, and telling it
// the settings were saved would leave the two permanently disagreeing.
func (h *Handler) UpdatePreferences(ctx context.Context, req *connect.Request[pushv1.UpdatePreferencesRequest]) (*connect.Response[pushv1.UpdatePreferencesResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if req.Msg.GetEndpoint() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint is required"))
	}

	updated, err := h.subscriptions.UpdatePreferences(ctx,
		user.UserID.String(), req.Msg.GetEndpoint(), preferencesFromProto(req.Msg.GetPreferences()))
	if err != nil {
		h.logger.ErrorContext(ctx, "UpdatePreferences failed", "error", err, "user_id", user.UserID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update notification preferences"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound,
			errors.New("no push subscription registered for this device"))
	}

	return connect.NewResponse(&pushv1.UpdatePreferencesResponse{}), nil
}

// DeleteSubscription removes this device.
//
// Deleting something that is not there succeeds. The browser calls this when
// the user turns every kind off, and it may also be reached after the
// dispatcher has already removed the row on a 410 — reporting NotFound would
// turn a completed intention into an error the user has to act on.
func (h *Handler) DeleteSubscription(ctx context.Context, req *connect.Request[pushv1.DeleteSubscriptionRequest]) (*connect.Response[pushv1.DeleteSubscriptionResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if req.Msg.GetEndpoint() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint is required"))
	}

	deleted, err := h.subscriptions.Delete(ctx, user.UserID.String(), req.Msg.GetEndpoint())
	if err != nil {
		h.logger.ErrorContext(ctx, "DeleteSubscription failed", "error", err, "user_id", user.UserID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete push subscription"))
	}

	h.logger.InfoContext(ctx, "push_subscription_deleted", "user_id", user.UserID, "existed", deleted)
	return connect.NewResponse(&pushv1.DeleteSubscriptionResponse{}), nil
}

// preferencesFromProto reads an absent message as "everything off".
//
// protoJSON omits false, so a user turning all four kinds off sends no
// preferences block at all. Defaulting that to "everything on" would opt them
// back into exactly what they opted out of — the one mistake a notification
// settings screen must not make.
func preferencesFromProto(p *pushv1.NotificationPreferences) domain.NotificationPreferences {
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

func preferencesToProto(p domain.NotificationPreferences) *pushv1.NotificationPreferences {
	return &pushv1.NotificationPreferences{
		SummaryReady:       p.SummaryReady,
		AcolyteReportReady: p.AcolyteReportReady,
		RecapReady:         p.RecapReady,
		TodayEntranceReady: p.TodayEntranceReady,
	}
}
