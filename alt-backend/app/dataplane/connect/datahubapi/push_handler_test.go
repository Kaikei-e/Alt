package datahubapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"alt/dataplane/usecase/push_delivery_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
)

// fakePushSubscriptionPort satisfies the port WithPushCapabilities requires.
// The delivery tests never reach it; it exists because the option refuses to
// build with a nil one.
type fakePushSubscriptionPort struct{}

func (f *fakePushSubscriptionPort) Upsert(context.Context, domain.PushSubscription) (bool, error) {
	return false, nil
}

func (f *fakePushSubscriptionPort) Get(context.Context, string, string) (*domain.PushSubscription, error) {
	return nil, nil
}

func (f *fakePushSubscriptionPort) UpdatePreferences(context.Context, string, string, domain.NotificationPreferences) (bool, error) {
	return false, nil
}

func (f *fakePushSubscriptionPort) Delete(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *fakePushSubscriptionPort) ListForUser(context.Context, string) ([]domain.PushSubscription, error) {
	return nil, nil
}

type fakePushDeliveryPort struct {
	oldest        time.Duration
	pending       int64
	subscriptions int64
	err           error
	calls         int
}

func (f *fakePushDeliveryPort) Enqueue(context.Context, domain.NotificationEnqueue) (int, int, error) {
	return 0, 0, nil
}

func (f *fakePushDeliveryPort) ClaimBatch(context.Context, string, int, time.Duration) ([]domain.PushDelivery, error) {
	return nil, nil
}

func (f *fakePushDeliveryPort) MarkSent(context.Context, string, int) error { return nil }

func (f *fakePushDeliveryPort) Release(context.Context, string, time.Time, string) error { return nil }

func (f *fakePushDeliveryPort) MarkDead(context.Context, string, int, string) error { return nil }

func (f *fakePushDeliveryPort) BacklogAge(context.Context) (time.Duration, int64, int64, error) {
	f.calls++
	if f.err != nil {
		return 0, 0, 0, f.err
	}
	return f.oldest, f.pending, f.subscriptions, nil
}

func newPushTestHandler(t *testing.T, deliveries *fakePushDeliveryPort) *Handler {
	t.Helper()
	return newTestHandler(WithPushCapabilities(
		&fakePushSubscriptionPort{},
		push_delivery_usecase.NewPushDeliveryUsecase(deliveries),
	))
}

func TestGetNotificationBacklogAge(t *testing.T) {
	tests := []struct {
		name          string
		oldest        time.Duration
		pending       int64
		subscriptions int64
		wantSeconds   float64
		wantPending   int64
	}{
		{
			name:          "a backlog reports its age, depth and device population",
			oldest:        90*time.Second + 500*time.Millisecond,
			pending:       4,
			subscriptions: 3,
			wantSeconds:   90.5,
			wantPending:   4,
		},
		{
			// A drained queue is a reading, not an absence. The dispatcher
			// publishes the gauge on every tick including this one, because a
			// gauge that stops being set keeps reporting its last value for as
			// long as the target is up.
			name:          "an empty queue answers zero rather than an error",
			oldest:        0,
			pending:       0,
			subscriptions: 2,
			wantSeconds:   0,
			wantPending:   0,
		},
		{
			// The pair the alert reads together. Identical to the row above in
			// every field the queue owns, and the opposite conclusion.
			name:          "an unsubscribed deployment reports zero devices",
			oldest:        0,
			pending:       0,
			subscriptions: 0,
			wantSeconds:   0,
			wantPending:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &fakePushDeliveryPort{
				oldest:        tt.oldest,
				pending:       tt.pending,
				subscriptions: tt.subscriptions,
			}
			h := newPushTestHandler(t, port)

			resp, err := h.GetNotificationBacklogAge(context.Background(),
				connect.NewRequest(&datahubv1.GetNotificationBacklogAgeRequest{}))
			require.NoError(t, err)
			require.Equal(t, 1, port.calls)
			require.InDelta(t, tt.wantSeconds, resp.Msg.GetOldestPendingAgeSeconds(), 1e-9)
			require.Equal(t, tt.wantPending, resp.Msg.GetPendingCount())
			require.Equal(t, tt.subscriptions, resp.Msg.GetActiveSubscriptionCount())
		})
	}
}

// A failed read must not be answered with a zero age. Zero is what a healthy
// drained queue reports, so returning it here would turn a broken data plane
// into a gauge that says the queue is empty — the same silence the alert over
// this metric exists to break.
func TestGetNotificationBacklogAge_ErrorIsNotZero(t *testing.T) {
	port := &fakePushDeliveryPort{err: errors.New("pool exhausted")}
	h := newPushTestHandler(t, port)

	resp, err := h.GetNotificationBacklogAge(context.Background(),
		connect.NewRequest(&datahubv1.GetNotificationBacklogAgeRequest{}))
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	// The internal reason does not travel to the caller.
	require.NotContains(t, err.Error(), "pool exhausted")
}

func TestUpsertPushSubscription_EndpointValidation(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantCode connect.Code
	}{
		{
			name:     "valid FCM endpoint accepted",
			endpoint: "https://fcm.googleapis.com/fcm/send/dGhpcy1pcy1hLXRlc3Q",
			wantCode: 0,
		},
		{
			name:     "valid Mozilla endpoint accepted",
			endpoint: "https://updates.push.services.mozilla.com/wpush/v2/gAAAAAB",
			wantCode: 0,
		},
		{
			name:     "valid Apple endpoint accepted",
			endpoint: "https://web.push.apple.com/QAtR7DUxQ",
			wantCode: 0,
		},
		{
			name:     "valid Windows endpoint accepted",
			endpoint: "https://wns2-par02p.notify.windows.com/w/?token=abc",
			wantCode: 0,
		},
		{
			name:     "http scheme rejected",
			endpoint: "http://fcm.googleapis.com/fcm/send/abc",
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "non-443 port rejected",
			endpoint: "https://fcm.googleapis.com:8443/fcm/send/abc",
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "userinfo rejected",
			endpoint: "https://user:pass@fcm.googleapis.com/fcm/send/abc",
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "internal service SSRF rejected",
			endpoint: "http://search-indexer:9300/v1/search",
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "untrusted host rejected",
			endpoint: "https://evil.example.com/push/abc",
			wantCode: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newPushTestHandler(t, &fakePushDeliveryPort{})
			_, err := h.UpsertPushSubscription(context.Background(), connect.NewRequest(&datahubv1.UpsertPushSubscriptionRequest{
				Subscription: &datahubv1.PushSubscription{
					UserId:              "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
					Endpoint:            tt.endpoint,
					P256Dh:              "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw",
					Auth:                "tBHItJI5svbpez7KI4CCXg",
					VapidKeyFingerprint: "b0a1c2d3e4f5",
				},
			}))
			if tt.wantCode != 0 {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, connect.CodeOf(err))
				return
			}
			require.NoError(t, err)
		})
	}
}
