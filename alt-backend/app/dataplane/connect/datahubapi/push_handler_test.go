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
	oldest  time.Duration
	pending int64
	err     error
	calls   int
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

func (f *fakePushDeliveryPort) BacklogAge(context.Context) (time.Duration, int64, error) {
	f.calls++
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.oldest, f.pending, nil
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
		name        string
		oldest      time.Duration
		pending     int64
		wantSeconds float64
		wantPending int64
	}{
		{
			name:        "a backlog reports its age and depth",
			oldest:      90*time.Second + 500*time.Millisecond,
			pending:     4,
			wantSeconds: 90.5,
			wantPending: 4,
		},
		{
			// A drained queue is a reading, not an absence. The dispatcher
			// publishes the gauge on every tick including this one, because a
			// gauge that stops being set keeps reporting its last value for as
			// long as the target is up.
			name:        "an empty queue answers zero rather than an error",
			oldest:      0,
			pending:     0,
			wantSeconds: 0,
			wantPending: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &fakePushDeliveryPort{oldest: tt.oldest, pending: tt.pending}
			h := newPushTestHandler(t, port)

			resp, err := h.GetNotificationBacklogAge(context.Background(),
				connect.NewRequest(&datahubv1.GetNotificationBacklogAgeRequest{}))
			require.NoError(t, err)
			require.Equal(t, 1, port.calls)
			require.InDelta(t, tt.wantSeconds, resp.Msg.GetOldestPendingAgeSeconds(), 1e-9)
			require.Equal(t, tt.wantPending, resp.Msg.GetPendingCount())
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
