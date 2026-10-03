package morning_letter_connect_gateway

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"alt/domain"
	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/shared/domain/authcontext"
)

func TestGateway_StreamChat_PropagatesIdentityHeaders(t *testing.T) {
	var receivedToken, receivedUser, receivedTenant string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("X-Alt-Backend-Token")
		receivedUser = r.Header.Get("X-Alt-User-Id")
		receivedTenant = r.Header.Get("X-Alt-Tenant-Id")
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	gw := NewGateway(server.Client(), server.URL, slog.Default())

	testUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testTID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	userCtx := &domain.UserContext{
		UserID:    testUID,
		TenantID:  testTID,
		Email:     "user@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	ctx := context.Background()
	ctx = authcontext.WithJWT(ctx, "jwt-proof-token")
	ctx = domain.SetUserContext(ctx, userCtx)

	_, _ = gw.StreamChat(ctx, []*morningletterv2.ChatMessage{
		{Role: "user", Content: "hello"},
	}, 24)

	assert.Equal(t, "jwt-proof-token", receivedToken)
	assert.Equal(t, testUID.String(), receivedUser)
	assert.Equal(t, testTID.String(), receivedTenant)
}
