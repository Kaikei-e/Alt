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

func TestClampReadLimit(t *testing.T) {
	assert.Equal(t, 100, clampReadLimit(0, 100))
	assert.Equal(t, 100, clampReadLimit(-10, 100))
	assert.Equal(t, 5, clampReadLimit(0, 5))
	assert.Equal(t, 50, clampReadLimit(50, 100))
	assert.Equal(t, 1000, clampReadLimit(500000, 100))
}

func TestGetTodayDigestAndRecallCandidates_DefaultsAndMapping(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New().String()

	digestResp, err := client.GetTodayDigest(ctx, connect.NewRequest(&sovereignv1.GetTodayDigestRequest{
		UserId: userID,
		Date:   "2026-10-24",
	}))
	require.NoError(t, err)
	assert.Nil(t, digestResp.Msg.Digest)

	recallResp, err := client.GetRecallCandidates(ctx, connect.NewRequest(&sovereignv1.GetRecallCandidatesRequest{
		UserId: userID,
		Limit:  5,
	}))
	require.NoError(t, err)
	assert.Empty(t, recallResp.Msg.Candidates)

	countResp, err := client.CountNeedToKnowItems(ctx, connect.NewRequest(&sovereignv1.CountNeedToKnowItemsRequest{
		UserId: userID,
		Date:   "2026-10-24",
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(0), countResp.Msg.Count)
}
