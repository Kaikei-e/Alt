package push

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"alt/domain"
	pushv1 "alt/gen/proto/alt/push/v1"
)

type fakeSubscriptionPort struct {
	upserted        []domain.PushSubscription
	stored          *domain.PushSubscription
	getErr          error
	updateResult    bool
	updateErr       error
	updatedPrefs    []domain.NotificationPreferences
	updatedUserID   string
	updatedEndpoint string
	deleteResult    bool
	deleteErr       error
	deletedUserID   string
	deletedEndpoint string
}

func (f *fakeSubscriptionPort) Upsert(_ context.Context, sub domain.PushSubscription) (bool, error) {
	f.upserted = append(f.upserted, sub)
	return true, nil
}

func (f *fakeSubscriptionPort) Get(_ context.Context, _, _ string) (*domain.PushSubscription, error) {
	return f.stored, f.getErr
}

func (f *fakeSubscriptionPort) UpdatePreferences(_ context.Context, userID, endpoint string, prefs domain.NotificationPreferences) (bool, error) {
	f.updatedUserID = userID
	f.updatedEndpoint = endpoint
	f.updatedPrefs = append(f.updatedPrefs, prefs)
	return f.updateResult, f.updateErr
}

func (f *fakeSubscriptionPort) Delete(_ context.Context, userID, endpoint string) (bool, error) {
	f.deletedUserID = userID
	f.deletedEndpoint = endpoint
	return f.deleteResult, f.deleteErr
}

func authenticatedContext(t *testing.T) context.Context {
	t.Helper()
	uid := uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uid,
		Email:     "user@example.com",
		TenantID:  uid,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestRegisterSubscription_EndpointValidation(t *testing.T) {
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
			port := &fakeSubscriptionPort{}
			h := NewHandler(port, "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw", nil)

			req := connect.NewRequest(&pushv1.RegisterSubscriptionRequest{
				Endpoint: tt.endpoint,
				Keys: &pushv1.PushSubscriptionKeys{
					P256Dh: "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw",
					Auth:   "tBHItJI5svbpez7KI4CCXg",
				},
			})

			_, err := h.RegisterSubscription(authenticatedContext(t), req)
			if tt.wantCode != 0 {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, connect.CodeOf(err))
				require.Empty(t, port.upserted)
				return
			}
			require.NoError(t, err)
			require.Len(t, port.upserted, 1)
		})
	}
}

const (
	testVAPIDKey      = "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw"
	testValidEndpoint = "https://fcm.googleapis.com/fcm/send/dGhpcy1pcy1hLXRlc3Q"
)

func TestNewHandler_FailFastValidation(t *testing.T) {
	t.Run("nil port panics", func(t *testing.T) {
		require.Panics(t, func() {
			NewHandler(nil, testVAPIDKey, nil)
		})
	})

	t.Run("empty vapidPublicKey panics", func(t *testing.T) {
		require.Panics(t, func() {
			NewHandler(&fakeSubscriptionPort{}, "", nil)
		})
	})
}

func TestGetPushConfig(t *testing.T) {
	t.Run("unauthenticated returns CodeUnauthenticated", func(t *testing.T) {
		h := NewHandler(&fakeSubscriptionPort{}, testVAPIDKey, nil)
		_, err := h.GetPushConfig(context.Background(), connect.NewRequest(&pushv1.GetPushConfigRequest{}))
		require.Error(t, err)
		require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("empty endpoint returns VAPID public key only with HasSubscription false", func(t *testing.T) {
		h := NewHandler(&fakeSubscriptionPort{}, testVAPIDKey, nil)
		resp, err := h.GetPushConfig(authenticatedContext(t), connect.NewRequest(&pushv1.GetPushConfigRequest{
			Endpoint: "",
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testVAPIDKey, resp.Msg.GetVapidPublicKey())
		require.False(t, resp.Msg.GetHasSubscription())
		require.Nil(t, resp.Msg.GetPreferences())
	})

	t.Run("registered endpoint returns HasSubscription true and preferences", func(t *testing.T) {
		port := &fakeSubscriptionPort{
			stored: &domain.PushSubscription{
				Endpoint: testValidEndpoint,
				Preferences: domain.NotificationPreferences{
					SummaryReady:       true,
					AcolyteReportReady: false,
					RecapReady:         true,
					TodayEntranceReady: false,
				},
			},
		}
		h := NewHandler(port, testVAPIDKey, nil)
		resp, err := h.GetPushConfig(authenticatedContext(t), connect.NewRequest(&pushv1.GetPushConfigRequest{
			Endpoint: testValidEndpoint,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testVAPIDKey, resp.Msg.GetVapidPublicKey())
		require.True(t, resp.Msg.GetHasSubscription())
		require.NotNil(t, resp.Msg.GetPreferences())
		require.True(t, resp.Msg.GetPreferences().GetSummaryReady())
		require.False(t, resp.Msg.GetPreferences().GetAcolyteReportReady())
		require.True(t, resp.Msg.GetPreferences().GetRecapReady())
		require.False(t, resp.Msg.GetPreferences().GetTodayEntranceReady())
	})

	t.Run("unregistered endpoint returns HasSubscription false", func(t *testing.T) {
		port := &fakeSubscriptionPort{stored: nil}
		h := NewHandler(port, testVAPIDKey, nil)
		resp, err := h.GetPushConfig(authenticatedContext(t), connect.NewRequest(&pushv1.GetPushConfigRequest{
			Endpoint: testValidEndpoint,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testVAPIDKey, resp.Msg.GetVapidPublicKey())
		require.False(t, resp.Msg.GetHasSubscription())
		require.Nil(t, resp.Msg.GetPreferences())
	})
}

func TestUpdatePreferences(t *testing.T) {
	t.Run("empty endpoint returns CodeInvalidArgument", func(t *testing.T) {
		h := NewHandler(&fakeSubscriptionPort{updateResult: true}, testVAPIDKey, nil)
		_, err := h.UpdatePreferences(authenticatedContext(t), connect.NewRequest(&pushv1.UpdatePreferencesRequest{
			Endpoint: "",
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("unregistered subscription returns CodeNotFound", func(t *testing.T) {
		port := &fakeSubscriptionPort{updateResult: false}
		h := NewHandler(port, testVAPIDKey, nil)
		_, err := h.UpdatePreferences(authenticatedContext(t), connect.NewRequest(&pushv1.UpdatePreferencesRequest{
			Endpoint: testValidEndpoint,
			Preferences: &pushv1.NotificationPreferences{
				SummaryReady: true,
			},
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("nil preferences message is treated as all-off NotificationPreferences", func(t *testing.T) {
		port := &fakeSubscriptionPort{updateResult: true}
		h := NewHandler(port, testVAPIDKey, nil)
		resp, err := h.UpdatePreferences(authenticatedContext(t), connect.NewRequest(&pushv1.UpdatePreferencesRequest{
			Endpoint:    testValidEndpoint,
			Preferences: nil,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, port.updatedPrefs, 1)
		require.Equal(t, domain.NotificationPreferences{
			SummaryReady:       false,
			AcolyteReportReady: false,
			RecapReady:         false,
			TodayEntranceReady: false,
		}, port.updatedPrefs[0])
	})
}

func TestDeleteSubscription(t *testing.T) {
	t.Run("empty endpoint returns CodeInvalidArgument", func(t *testing.T) {
		h := NewHandler(&fakeSubscriptionPort{deleteResult: true}, testVAPIDKey, nil)
		_, err := h.DeleteSubscription(authenticatedContext(t), connect.NewRequest(&pushv1.DeleteSubscriptionRequest{
			Endpoint: "",
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("unregistered subscription succeeds idempotently without error", func(t *testing.T) {
		port := &fakeSubscriptionPort{deleteResult: false}
		h := NewHandler(port, testVAPIDKey, nil)
		resp, err := h.DeleteSubscription(authenticatedContext(t), connect.NewRequest(&pushv1.DeleteSubscriptionRequest{
			Endpoint: testValidEndpoint,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testValidEndpoint, port.deletedEndpoint)
	})

	t.Run("registered subscription deletion succeeds", func(t *testing.T) {
		port := &fakeSubscriptionPort{deleteResult: true}
		h := NewHandler(port, testVAPIDKey, nil)
		resp, err := h.DeleteSubscription(authenticatedContext(t), connect.NewRequest(&pushv1.DeleteSubscriptionRequest{
			Endpoint: testValidEndpoint,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testValidEndpoint, port.deletedEndpoint)
	})
}
