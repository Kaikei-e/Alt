package feeds

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/utils/logger"

	"alt/config"
	"alt/di"
	"alt/domain"
)

// Mock ports for testing
type mockFeedAmountPort struct{}

func (m *mockFeedAmountPort) Execute(ctx context.Context) (int, error) {
	return 10, nil
}

type mockSummarizedArticlesCountPort struct{}

func (m *mockSummarizedArticlesCountPort) Execute(ctx context.Context) (int, error) {
	return 7, nil
}

type mockTotalArticlesCountPort struct{}

func (m *mockTotalArticlesCountPort) Execute(ctx context.Context) (int, error) {
	return 100, nil
}

type mockUnsummarizedArticlesCountPort struct{}

func (m *mockUnsummarizedArticlesCountPort) Execute(ctx context.Context) (int, error) {
	return 3, nil
}

// Create test handler
func createTestHandler() *Handler {
	// Initialize global logger for usecases
	logger.InitLogger()

	feedAmountUsecase := fetch_feed_stats_usecase.NewFeedsCountUsecase(&mockFeedAmountPort{})
	summarizedUsecase := fetch_feed_stats_usecase.NewSummarizedArticlesCountUsecase(&mockSummarizedArticlesCountPort{})
	totalArticlesUsecase := fetch_feed_stats_usecase.NewTotalArticlesCountUsecase(&mockTotalArticlesCountPort{})
	unsummarizedUsecase := fetch_feed_stats_usecase.NewUnsummarizedArticlesCountUsecase(&mockUnsummarizedArticlesCountPort{})

	container := &di.ApplicationComponents{
		FeedAmountUsecase:                feedAmountUsecase,
		SummarizedArticlesCountUsecase:   summarizedUsecase,
		TotalArticlesCountUsecase:        totalArticlesUsecase,
		UnsummarizedArticlesCountUsecase: unsummarizedUsecase,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handlerLogger := slog.Default()

	return NewHandler(container, cfg, handlerLogger)
}

func createAuthContext() context.Context {
	userID := uuid.New()
	tenantID := uuid.New()
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  tenantID,
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// Test unary RPC methods
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
	ctx := context.Background() // No auth

	req := connect.NewRequest(&feedsv2.GetFeedStatsRequest{})
	_, err := handler.GetFeedStats(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

// Test streaming response construction (unit test of helper function)
// Note: Full streaming tests would require integration testing
func TestStreamFeedStats_DataConstruction(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	// Test that we can construct proper response messages
	// This tests the data gathering logic without actual streaming

	// Get feed stats
	feedCount, err := handler.container.FeedAmountUsecase.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10, feedCount)

	unsummarized, err := handler.container.UnsummarizedArticlesCountUsecase.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, unsummarized)

	totalArticles, err := handler.container.TotalArticlesCountUsecase.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, 100, totalArticles)

	// Construct response message (as done in sendStatsUpdate)
	resp := &feedsv2.StreamFeedStatsResponse{
		FeedAmount:              int64(feedCount),
		UnsummarizedFeedAmount:  int64(unsummarized),
		TotalArticles:           int64(totalArticles),
		Metadata: &feedsv2.ResponseMetadata{
			Timestamp:   time.Now().Unix(),
			IsHeartbeat: false,
		},
	}

	// Verify response structure
	assert.Equal(t, int64(10), resp.FeedAmount)
	assert.Equal(t, int64(3), resp.UnsummarizedFeedAmount)
	assert.Equal(t, int64(100), resp.TotalArticles)
	assert.NotNil(t, resp.Metadata)
	assert.False(t, resp.Metadata.IsHeartbeat)
	assert.Greater(t, resp.Metadata.Timestamp, int64(0))
}

func TestStreamFeedStats_HeartbeatConstruction(t *testing.T) {
	// Test heartbeat message construction
	heartbeat := &feedsv2.StreamFeedStatsResponse{
		Metadata: &feedsv2.ResponseMetadata{
			Timestamp:   time.Now().Unix(),
			IsHeartbeat: true,
		},
	}

	// Heartbeat should have zero stats
	assert.Equal(t, int64(0), heartbeat.FeedAmount)
	assert.Equal(t, int64(0), heartbeat.UnsummarizedFeedAmount)
	assert.Equal(t, int64(0), heartbeat.TotalArticles)
	assert.True(t, heartbeat.Metadata.IsHeartbeat)
}
