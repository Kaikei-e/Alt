package handler

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestGetKnowledgeHomeItems_ReturnsEmpty(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	resp, err := client.GetKnowledgeHomeItems(context.Background(),
		connect.NewRequest(&sovereignv1.GetKnowledgeHomeItemsRequest{
			UserId: uuid.New().String(),
			Limit:  10,
		}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Items)
	assert.False(t, resp.Msg.HasMore)
}

// TestGetKnowledgeHomeItems_InvalidUserID_ReturnsInvalidArgument pins the
// fix for the silent-fallback UUID bug: a malformed user_id must be
// rejected at the handler boundary with CodeInvalidArgument, not silently
// coerced to uuid.Nil and forwarded to the repository.
func TestGetKnowledgeHomeItems_InvalidUserID_ReturnsInvalidArgument(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	_, err := client.GetKnowledgeHomeItems(context.Background(),
		connect.NewRequest(&sovereignv1.GetKnowledgeHomeItemsRequest{
			UserId: "not-a-uuid",
			Limit:  10,
		}))

	require.Error(t, err, "malformed user_id must be rejected, not silently coerced to uuid.Nil")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestListDistinctUserIDs_ReturnsEmpty(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	resp, err := client.ListDistinctUserIDs(context.Background(),
		connect.NewRequest(&sovereignv1.ListDistinctUserIDsRequest{}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.UserIds)
}
