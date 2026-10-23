package datahubapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/dataplane/usecase/push_delivery_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
)

type fakePushSubscriptionPort struct {
	upsertCreated bool
	upsertErr     error
	lastUpsert    domain.PushSubscription

	getSub *domain.PushSubscription
	getErr error

	updateResult bool
	updateErr    error
	lastPrefs    domain.NotificationPreferences

	deleteResult bool
	deleteErr    error

	listSubs []domain.PushSubscription
	listErr  error
}

func (f *fakePushSubscriptionPort) Upsert(_ context.Context, sub domain.PushSubscription) (bool, error) {
	f.lastUpsert = sub
	return f.upsertCreated, f.upsertErr
}

func (f *fakePushSubscriptionPort) Get(_ context.Context, _, _ string) (*domain.PushSubscription, error) {
	return f.getSub, f.getErr
}

func (f *fakePushSubscriptionPort) UpdatePreferences(_ context.Context, _, _ string, prefs domain.NotificationPreferences) (bool, error) {
	f.lastPrefs = prefs
	return f.updateResult, f.updateErr
}

func (f *fakePushSubscriptionPort) Delete(_ context.Context, _, _ string) (bool, error) {
	return f.deleteResult, f.deleteErr
}

func (f *fakePushSubscriptionPort) ListForUser(_ context.Context, _ string) ([]domain.PushSubscription, error) {
	return f.listSubs, f.listErr
}

type fakePushDeliveryPort struct {
	oldest        time.Duration
	pending       int64
	subscriptions int64
	err           error
	calls         int

	enqueueDelivered  int
	enqueueSuperseded int
	enqueueErr        error

	claimedBatch []domain.PushDelivery
	claimErr     error

	markSentErr error
	releaseErr  error
	markDeadErr error
}

func (f *fakePushDeliveryPort) Enqueue(_ context.Context, _ domain.NotificationEnqueue) (int, int, error) {
	return f.enqueueDelivered, f.enqueueSuperseded, f.enqueueErr
}

func (f *fakePushDeliveryPort) ClaimBatch(_ context.Context, _ string, _ int, _ time.Duration) ([]domain.PushDelivery, error) {
	return f.claimedBatch, f.claimErr
}

func (f *fakePushDeliveryPort) MarkSent(_ context.Context, _ string, _ int) error {
	return f.markSentErr
}

func (f *fakePushDeliveryPort) Release(_ context.Context, _ string, _ time.Time, _ string) error {
	return f.releaseErr
}

func (f *fakePushDeliveryPort) MarkDead(_ context.Context, _ string, _ int, _ string) error {
	return f.markDeadErr
}

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

func TestWithPushCapabilities_PanicsOnNilDependency(t *testing.T) {
	uc := push_delivery_usecase.NewPushDeliveryUsecase(&fakePushDeliveryPort{})
	assert.Panics(t, func() {
		WithPushCapabilities(nil, uc)
	})
	assert.Panics(t, func() {
		WithPushCapabilities(&fakePushSubscriptionPort{}, nil)
	})
}

func TestUpsertPushSubscription_RequiredFieldsAndOwnershipConflict(t *testing.T) {
	const validEndpoint = "https://fcm.googleapis.com/fcm/send/dGhpcy1pcy1hLXRlc3Q"
	const validUser = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"

	t.Run("missing fields return CodeInvalidArgument", func(t *testing.T) {
		h := newPushTestHandler(t, &fakePushDeliveryPort{})

		cases := []*datahubv1.PushSubscription{
			nil,
			{Endpoint: validEndpoint, P256Dh: "k", Auth: "a", VapidKeyFingerprint: "fp"},
			{UserId: validUser, P256Dh: "k", Auth: "a", VapidKeyFingerprint: "fp"},
			{UserId: validUser, Endpoint: validEndpoint, Auth: "a", VapidKeyFingerprint: "fp"},
			{UserId: validUser, Endpoint: validEndpoint, P256Dh: "k", VapidKeyFingerprint: "fp"},
			{UserId: validUser, Endpoint: validEndpoint, P256Dh: "k", Auth: "a"},
		}
		for _, sub := range cases {
			_, err := h.UpsertPushSubscription(context.Background(), connect.NewRequest(&datahubv1.UpsertPushSubscriptionRequest{
				Subscription: sub,
			}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		}
	})

	t.Run("ownership conflict returns CodeAlreadyExists without leaking endpoint", func(t *testing.T) {
		subPort := &fakePushSubscriptionPort{
			upsertErr: domain.ErrPushSubscriptionOwnedByAnotherUser,
		}
		h := newTestHandler(WithPushCapabilities(subPort, push_delivery_usecase.NewPushDeliveryUsecase(&fakePushDeliveryPort{})))

		_, err := h.UpsertPushSubscription(context.Background(), connect.NewRequest(&datahubv1.UpsertPushSubscriptionRequest{
			Subscription: &datahubv1.PushSubscription{
				UserId:              validUser,
				Endpoint:            validEndpoint,
				P256Dh:              "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw",
				Auth:                "tBHItJI5svbpez7KI4CCXg",
				VapidKeyFingerprint: "b0a1c2d3",
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
		assert.NotContains(t, err.Error(), validEndpoint)
	})
}

func TestPushSubscriptionCRUDHandlers(t *testing.T) {
	const validEndpoint = "https://fcm.googleapis.com/fcm/send/dGhpcy1pcy1hLXRlc3Q"
	const validUser = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"

	subPort := &fakePushSubscriptionPort{
		getSub: &domain.PushSubscription{
			UserID:              validUser,
			Endpoint:            validEndpoint,
			P256dh:              "p256dh-val",
			Auth:                "auth-val",
			VAPIDKeyFingerprint: "fp123456",
			Preferences:         domain.NotificationPreferences{SummaryReady: true, RecapReady: true},
		},
		updateResult: true,
		deleteResult: true,
		listSubs: []domain.PushSubscription{
			{UserID: validUser, Endpoint: validEndpoint, VAPIDKeyFingerprint: "fp123456"},
		},
	}
	h := newTestHandler(WithPushCapabilities(subPort, push_delivery_usecase.NewPushDeliveryUsecase(&fakePushDeliveryPort{})))

	t.Run("GetPushSubscription validates args and maps subscription", func(t *testing.T) {
		_, err := h.GetPushSubscription(context.Background(), connect.NewRequest(&datahubv1.GetPushSubscriptionRequest{}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		resp, err := h.GetPushSubscription(context.Background(), connect.NewRequest(&datahubv1.GetPushSubscriptionRequest{
			UserId:   validUser,
			Endpoint: validEndpoint,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.GetSubscription())
		assert.Equal(t, validEndpoint, resp.Msg.GetSubscription().GetEndpoint())
		assert.True(t, resp.Msg.GetSubscription().GetPreferences().GetSummaryReady())
	})

	t.Run("UpdatePushSubscriptionPreferences treats nil preferences as all-off", func(t *testing.T) {
		resp, err := h.UpdatePushSubscriptionPreferences(context.Background(), connect.NewRequest(&datahubv1.UpdatePushSubscriptionPreferencesRequest{
			UserId:      validUser,
			Endpoint:    validEndpoint,
			Preferences: nil,
		}))
		require.NoError(t, err)
		assert.True(t, resp.Msg.GetUpdated())
		assert.Equal(t, domain.NotificationPreferences{}, subPort.lastPrefs)
	})

	t.Run("DeletePushSubscription is idempotent", func(t *testing.T) {
		resp, err := h.DeletePushSubscription(context.Background(), connect.NewRequest(&datahubv1.DeletePushSubscriptionRequest{
			UserId:   validUser,
			Endpoint: validEndpoint,
		}))
		require.NoError(t, err)
		assert.True(t, resp.Msg.GetDeleted())
	})

	t.Run("ListPushSubscriptionsForUser requires user_id and returns subscriptions", func(t *testing.T) {
		_, err := h.ListPushSubscriptionsForUser(context.Background(), connect.NewRequest(&datahubv1.ListPushSubscriptionsForUserRequest{}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		resp, err := h.ListPushSubscriptionsForUser(context.Background(), connect.NewRequest(&datahubv1.ListPushSubscriptionsForUserRequest{
			UserId: validUser,
		}))
		require.NoError(t, err)
		require.Len(t, resp.Msg.GetSubscriptions(), 1)
		assert.Equal(t, validEndpoint, resp.Msg.GetSubscriptions()[0].GetEndpoint())
	})
}

func TestPushDeliveryQueueHandlers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	deliveryPort := &fakePushDeliveryPort{
		enqueueDelivered:  2,
		enqueueSuperseded: 1,
		claimedBatch: []domain.PushDelivery{
			{
				ID:             "deliv-1",
				DedupeKey:      "summary_ready:art-1",
				SubscriptionID: "sub-1",
				UserID:         "user-1",
				Kind:           domain.NotificationKindSummaryReady,
				State:          domain.NotificationSending,
				Attempts:       1,
				Endpoint:       "https://fcm.googleapis.com/fcm/send/abc",
				OccurredAt:     now,
			},
		},
	}
	h := newPushTestHandler(t, deliveryPort)

	t.Run("EnqueueNotification validates kind and returns counts", func(t *testing.T) {
		_, err := h.EnqueueNotification(context.Background(), connect.NewRequest(&datahubv1.EnqueueNotificationRequest{
			DedupeKey: "k1",
			UserId:    "user-1",
			Kind:      "unknown_kind",
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		resp, err := h.EnqueueNotification(context.Background(), connect.NewRequest(&datahubv1.EnqueueNotificationRequest{
			DedupeKey:  "summary_ready:art-1",
			UserId:     "user-1",
			Kind:       domain.NotificationKindSummaryReady,
			Payload:    []byte(`{"title":"Ready"}`),
			OccurredAt: timestamppb.New(now),
			ExpiresAt:  timestamppb.New(now.Add(24 * time.Hour)),
		}))
		require.NoError(t, err)
		assert.Equal(t, int32(2), resp.Msg.GetDeliveryCount())
		assert.Equal(t, int32(1), resp.Msg.GetSupersededCount())
	})

	t.Run("ClaimNotificationBatch requires locked_by and maps sending rows", func(t *testing.T) {
		_, err := h.ClaimNotificationBatch(context.Background(), connect.NewRequest(&datahubv1.ClaimNotificationBatchRequest{}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		resp, err := h.ClaimNotificationBatch(context.Background(), connect.NewRequest(&datahubv1.ClaimNotificationBatchRequest{
			LockedBy:     "alt-notifier-1",
			Limit:        10,
			LeaseSeconds: 30,
		}))
		require.NoError(t, err)
		require.Len(t, resp.Msg.GetDeliveries(), 1)
		assert.Equal(t, datahubv1.NotificationState_NOTIFICATION_STATE_SENDING, resp.Msg.GetDeliveries()[0].GetState())
	})

	t.Run("MarkNotificationSent ReleaseNotification and MarkNotificationDead validate IDs", func(t *testing.T) {
		_, err := h.MarkNotificationSent(context.Background(), connect.NewRequest(&datahubv1.MarkNotificationSentRequest{}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		_, err = h.MarkNotificationSent(context.Background(), connect.NewRequest(&datahubv1.MarkNotificationSentRequest{
			Id:         "deliv-1",
			StatusCode: 201,
		}))
		require.NoError(t, err)

		_, err = h.ReleaseNotification(context.Background(), connect.NewRequest(&datahubv1.ReleaseNotificationRequest{Id: "deliv-1"}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		_, err = h.ReleaseNotification(context.Background(), connect.NewRequest(&datahubv1.ReleaseNotificationRequest{
			Id:            "deliv-1",
			NextAttemptAt: timestamppb.New(now.Add(10 * time.Second)),
			ErrorMessage:  "429 retryable",
		}))
		require.NoError(t, err)

		_, err = h.MarkNotificationDead(context.Background(), connect.NewRequest(&datahubv1.MarkNotificationDeadRequest{}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		_, err = h.MarkNotificationDead(context.Background(), connect.NewRequest(&datahubv1.MarkNotificationDeadRequest{
			Id:           "deliv-1",
			StatusCode:   401,
			ErrorMessage: "push service rejected: status=401",
		}))
		require.NoError(t, err)
	})
}
