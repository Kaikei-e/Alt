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
	upserted []domain.PushSubscription
}

func (f *fakeSubscriptionPort) Upsert(_ context.Context, sub domain.PushSubscription) (bool, error) {
	f.upserted = append(f.upserted, sub)
	return true, nil
}

func (f *fakeSubscriptionPort) Get(_ context.Context, _, _ string) (*domain.PushSubscription, error) {
	return nil, nil
}

func (f *fakeSubscriptionPort) UpdatePreferences(_ context.Context, _, _ string, _ domain.NotificationPreferences) (bool, error) {
	return true, nil
}

func (f *fakeSubscriptionPort) Delete(_ context.Context, _, _ string) (bool, error) {
	return true, nil
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
