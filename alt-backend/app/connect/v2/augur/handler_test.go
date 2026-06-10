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

// authedCtx returns a context with a valid UserContext for handler tests.
func authedCtx() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		ExpiresAt: timeFar(),
	})
}

// TestGetConversation_PassesThroughCodeNotFound pins the fix for the
// "Error ID: <hash>" red banner the UI showed every time a user asked a
// just-created conversation but the consumer polled before rag-orchestrator
// had finished the row insert. The provider returns `CodeNotFound`; the
// previous handler wrapped that into `CodeInternal` with an "internal server
// error (caused by: not_found)" message, so the FE treated it as an outage.
// The handler MUST transparently re-emit CodeNotFound so the FE can render a
// graceful "conversation not yet available" state instead.
func TestGetConversation_PassesThroughCodeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		GetConversation(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeNotFound, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.GetConversation(authedCtx(), connect.NewRequest(&augurv2.GetConversationRequest{
		Id: uuid.NewString(),
	}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
		"GetConversation must transparently re-emit CodeNotFound, not wrap as Internal")
}

// TestListConversations_PassesThroughCodeNotFound — same pattern: a user
// whose history has been purged should see an empty / "no history" UI, not
// a red error banner.
func TestListConversations_PassesThroughCodeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		ListConversations(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeNotFound, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.ListConversations(authedCtx(), connect.NewRequest(&augurv2.ListConversationsRequest{}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
		"ListConversations must transparently re-emit CodeNotFound")
}

// TestDeleteConversation_PassesThroughCodeNotFound — delete on a missing
// conversation is idempotent semantically; the FE treats NotFound as a
// no-op success. Wrapping as Internal turned the action into a hard error.
func TestDeleteConversation_PassesThroughCodeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		DeleteConversation(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeNotFound, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.DeleteConversation(authedCtx(), connect.NewRequest(&augurv2.DeleteConversationRequest{
		Id: uuid.NewString(),
	}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
		"DeleteConversation must transparently re-emit CodeNotFound")
}

// TestGetConversation_NonNotFoundStillWrapsAsInternal — defense in depth.
// Provider-side network failure, mTLS denial, deadline exceeded etc. MUST
// still be sanitised to CodeInternal so internal details do not leak.
func TestGetConversation_NonNotFoundStillWrapsAsInternal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		GetConversation(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeUnavailable, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.GetConversation(authedCtx(), connect.NewRequest(&augurv2.GetConversationRequest{
		Id: uuid.NewString(),
	}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err),
		"non-NotFound upstream codes must still be sanitised to Internal")
}
