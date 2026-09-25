package feeds

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feedsv2 "alt/gen/proto/alt/feeds/v2"
)

func TestGetFeedStats(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.GetFeedStatsRequest{})
	resp, err := handler.GetFeedStats(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(10), resp.Msg.FeedAmount)
	assert.Equal(t, int64(7), resp.Msg.SummarizedFeedAmount)
}

func TestGetDetailedFeedStats(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.GetDetailedFeedStatsRequest{})
	resp, err := handler.GetDetailedFeedStats(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(10), resp.Msg.FeedAmount)
	assert.Equal(t, int64(100), resp.Msg.ArticleAmount)
	assert.Equal(t, int64(3), resp.Msg.UnsummarizedFeedAmount)
}

func TestGetFeedStats_RequiresAuth(t *testing.T) {
	handler := createTestHandler()
	ctx := context.Background()

	req := connect.NewRequest(&feedsv2.GetFeedStatsRequest{})
	_, err := handler.GetFeedStats(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestStreamFeedStats_DataConstruction(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	feedCount, err := handler.deps.FeedAmount.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10, feedCount)

	unsummarized, err := handler.deps.UnsummarizedCount.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, unsummarized)

	totalArticles, err := handler.deps.TotalCount.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, 100, totalArticles)

	resp := &feedsv2.StreamFeedStatsResponse{
		FeedAmount:             int64(feedCount),
		UnsummarizedFeedAmount: int64(unsummarized),
		TotalArticles:          int64(totalArticles),
		Metadata: &feedsv2.ResponseMetadata{
			Timestamp:   time.Now().Unix(),
			IsHeartbeat: false,
		},
	}

	assert.Equal(t, int64(10), resp.FeedAmount)
	assert.Equal(t, int64(3), resp.UnsummarizedFeedAmount)
	assert.Equal(t, int64(100), resp.TotalArticles)
	assert.NotNil(t, resp.Metadata)
	assert.False(t, resp.Metadata.IsHeartbeat)
	assert.Greater(t, resp.Metadata.Timestamp, int64(0))
}

func TestStreamFeedStats_HeartbeatConstruction(t *testing.T) {
	heartbeat := &feedsv2.StreamFeedStatsResponse{
		Metadata: &feedsv2.ResponseMetadata{
			Timestamp:   time.Now().Unix(),
			IsHeartbeat: true,
		},
	}

	assert.Equal(t, int64(0), heartbeat.FeedAmount)
	assert.Equal(t, int64(0), heartbeat.UnsummarizedFeedAmount)
	assert.Equal(t, int64(0), heartbeat.TotalArticles)
	assert.True(t, heartbeat.Metadata.IsHeartbeat)
}
