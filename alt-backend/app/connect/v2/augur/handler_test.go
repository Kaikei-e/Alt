package augur

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"alt/domain"
	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/mocks"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// timeFar returns a far-future timestamp for UserContext.ExpiresAt so
// IsValid() never trips during tests.
func timeFar() time.Time { return time.Now().Add(24 * time.Hour) }

// TestCreateAugurSessionFromLoopEntry_ForwardsTenantHeader pins Wave 4-A's
// trust-boundary contract: alt-backend MUST set both X-Alt-User-Id AND
// X-Alt-Tenant-Id when proxying CreateAugurSessionFromLoopEntry to
// rag-orchestrator. Forgetting the tenant header would cause the
// downstream augur emit to fail Unauthenticated and silently disable
// the Knowledge Loop Surface Planner v2 augur_link signal.
func TestCreateAugurSessionFromLoopEntry_ForwardsTenantHeader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := uuid.New()
	tenantID := uuid.New()

	user := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  tenantID,
		ExpiresAt: timeFar(),
	}
	ctx := domain.SetUserContext(context.Background(), user)

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	var capturedHeaders map[string]string
	mockPort.EXPECT().
		CreateAugurSessionFromLoopEntry(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *connect.Request[augurv2.CreateAugurSessionFromLoopEntryRequest]) (*connect.Response[augurv2.CreateAugurSessionFromLoopEntryResponse], error) {
			capturedHeaders = map[string]string{
				userIDHeader:   req.Header().Get(userIDHeader),
				tenantIDHeader: req.Header().Get(tenantIDHeader),
			}
			return connect.NewResponse(&augurv2.CreateAugurSessionFromLoopEntryResponse{
				ConversationId: uuid.NewString(),
			}), nil
		})

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.CreateAugurSessionFromLoopEntry(ctx, connect.NewRequest(&augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: "01938e82-7c00-7a7b-9b10-0123456789ab",
		EntryKey:          "entry-1",
		WhyText:           "a why",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, userID.String(), capturedHeaders[userIDHeader],
		"X-Alt-User-Id must be forwarded")
	assert.Equal(t, tenantID.String(), capturedHeaders[tenantIDHeader],
		"X-Alt-Tenant-Id must be forwarded (Wave 4-A trust-boundary contract)")
}

// sanitizeMetaEvent must preserve conversation_id end-to-end so the
// frontend can keep sending the same id on subsequent turns. Stripping it
// (the original behaviour) caused every turn to spawn a brand-new
// conversation row, fragmenting the user's history.
func TestSanitizeMetaEvent_PreservesConversationID(t *testing.T) {
	h := NewHandler(nil, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	in := &augurv2.StreamChatResponse{
		Kind: "meta",
		Payload: &augurv2.StreamChatResponse_Meta{
			Meta: &augurv2.MetaPayload{
				ConversationId: "11111111-2222-3333-4444-555555555555",
				Citations: []*augurv2.Citation{
					{Url: "https://example.com", Title: "Example"},
				},
			},
		},
	}

	out := h.sanitizeMetaEvent(in)
	meta := out.GetMeta()
	if assert.NotNil(t, meta, "sanitized event must still carry MetaPayload") {
		assert.Equal(t, "11111111-2222-3333-4444-555555555555", meta.ConversationId,
			"conversation_id must round-trip through sanitization")
		assert.Len(t, meta.Citations, 1, "citations array must be preserved")
		assert.Equal(t, "https://example.com", meta.Citations[0].Url)
	}
}
