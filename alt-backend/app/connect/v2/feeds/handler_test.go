package feeds

import (
	"context"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/driver/alt_db"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/reading_status"
	"alt/utils/logger"
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

// Mock for FeedsReadingStatusUsecase dependency
type mockUpdateFeedStatusPort struct{}

func (m *mockUpdateFeedStatusPort) UpdateFeedStatus(ctx context.Context, feedURL url.URL) error {
	return nil
}

// Mock for ArticlesReadingStatusUsecase dependency
type mockUpdateArticleStatusPort struct{}

func (m *mockUpdateArticleStatusPort) MarkArticleAsRead(ctx context.Context, articleURL url.URL) error {
	return nil
}

// Create test handler
func createTestHandler() *Handler {
	// Initialize global logger for usecases
	logger.InitLogger()

	feedAmountUsecase := fetch_feed_stats_usecase.NewFeedsCountUsecase(&mockFeedAmountPort{})
	summarizedUsecase := fetch_feed_stats_usecase.NewSummarizedArticlesCountUsecase(&mockSummarizedArticlesCountPort{})
	totalArticlesUsecase := fetch_feed_stats_usecase.NewTotalArticlesCountUsecase(&mockTotalArticlesCountPort{})
	unsummarizedUsecase := fetch_feed_stats_usecase.NewUnsummarizedArticlesCountUsecase(&mockUnsummarizedArticlesCountPort{})
	feedsReadingStatusUsecase := reading_status.NewFeedsReadingStatusUsecase(&mockUpdateFeedStatusPort{})
	articlesReadingStatusUsecase := reading_status.NewArticlesReadingStatusUsecase(&mockUpdateArticleStatusPort{})

	container := &di.ApplicationComponents{
		FeedAmountUsecase:                feedAmountUsecase,
		SummarizedArticlesCountUsecase:   summarizedUsecase,
		TotalArticlesCountUsecase:        totalArticlesUsecase,
		UnsummarizedArticlesCountUsecase: unsummarizedUsecase,
		FeedsReadingStatusUsecase:        feedsReadingStatusUsecase,
		ArticlesReadingStatusUsecase:     articlesReadingStatusUsecase,
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

// =============================================================================
// Phase 2-3: Feed List and Search Helper Tests
// =============================================================================

func createSampleFeeds() []*domain.FeedItem {
	now := time.Now()
	return []*domain.FeedItem{
		{
			Title:           "Test Feed 1",
			Description:     "<p>Test Description 1</p>",
			Link:            "https://example.com/feed1",
			Published:       now.Format(time.RFC3339),
			PublishedParsed: now,
			Author:          domain.Author{Name: "Author 1"},
		},
		{
			Title:           "Test Feed 2",
			Description:     "<p>Test Description 2</p>",
			Link:            "https://example.com/feed2",
			Published:       now.Add(-time.Hour).Format(time.RFC3339),
			PublishedParsed: now.Add(-time.Hour),
			Author:          domain.Author{Name: "Author 2"},
		},
	}
}

func TestConvertFeedsToProto(t *testing.T) {
	feeds := createSampleFeeds()
	protoFeeds := convertFeedsToProto(feeds)

	require.Len(t, protoFeeds, 2)

	// First feed
	assert.Equal(t, "https://example.com/feed1", protoFeeds[0].Id)
	assert.Equal(t, "Test Feed 1", protoFeeds[0].Title)
	assert.Equal(t, "Test Description 1", protoFeeds[0].Description) // HTML sanitized
	assert.Equal(t, "https://example.com/feed1", protoFeeds[0].Link)
	assert.Equal(t, "Author 1", protoFeeds[0].Author)
	assert.NotEmpty(t, protoFeeds[0].CreatedAt)
	assert.NotEmpty(t, protoFeeds[0].Published)

	// Second feed
	assert.Equal(t, "https://example.com/feed2", protoFeeds[1].Id)
	assert.Equal(t, "Test Feed 2", protoFeeds[1].Title)
}

func TestConvertFeedsToProto_EmptyList(t *testing.T) {
	protoFeeds := convertFeedsToProto([]*domain.FeedItem{})
	assert.Len(t, protoFeeds, 0)
	assert.NotNil(t, protoFeeds) // Should be empty slice, not nil
}

func TestDeriveNextCursor_WithHasMore(t *testing.T) {
	feeds := createSampleFeeds()
	cursor := deriveNextCursor(feeds, true)

	require.NotNil(t, cursor)
	// Should be RFC3339 format
	_, err := time.Parse(time.RFC3339, *cursor)
	assert.NoError(t, err)
}

func TestDeriveNextCursor_WithoutHasMore(t *testing.T) {
	feeds := createSampleFeeds()
	cursor := deriveNextCursor(feeds, false)

	assert.Nil(t, cursor)
}

func TestDeriveNextCursor_EmptyFeeds(t *testing.T) {
	cursor := deriveNextCursor([]*domain.FeedItem{}, true)
	assert.Nil(t, cursor)
}

func TestSanitizeDescription(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes HTML tags",
			input:    "<p>Hello <strong>World</strong></p>",
			expected: "Hello World",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "collapses whitespace",
			input:    "Hello    World",
			expected: "Hello World",
		},
		{
			name:     "removes script tags",
			input:    "<script>alert('xss')</script>Hello",
			expected: "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeDescription(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "just now",
			input:    now.Add(-30 * time.Second),
			expected: "Just now",
		},
		{
			name:     "minutes ago",
			input:    now.Add(-5 * time.Minute),
			expected: "5m ago",
		},
		{
			name:     "hours ago",
			input:    now.Add(-3 * time.Hour),
			expected: "3h ago",
		},
		{
			name:     "yesterday",
			input:    now.Add(-36 * time.Hour),
			expected: "Yesterday",
		},
		{
			name:     "zero time",
			input:    time.Time{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimeAgo(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatAuthor(t *testing.T) {
	tests := []struct {
		name     string
		author   domain.Author
		authors  []domain.Author
		expected string
	}{
		{
			name:     "uses primary author",
			author:   domain.Author{Name: "Primary Author"},
			authors:  []domain.Author{{Name: "Secondary"}},
			expected: "Primary Author",
		},
		{
			name:     "falls back to first author",
			author:   domain.Author{},
			authors:  []domain.Author{{Name: "First Author"}},
			expected: "First Author",
		},
		{
			name:     "empty when no authors",
			author:   domain.Author{},
			authors:  []domain.Author{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAuthor(tt.author, tt.authors)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test response message construction for Phase 2-3
func TestGetUnreadFeedsResponse_Construction(t *testing.T) {
	feeds := createSampleFeeds()
	protoFeeds := convertFeedsToProto(feeds)
	nextCursor := deriveNextCursor(feeds, true)

	resp := &feedsv2.GetUnreadFeedsResponse{
		Data:       protoFeeds,
		NextCursor: nextCursor,
		HasMore:    true,
	}

	assert.Len(t, resp.Data, 2)
	assert.True(t, resp.HasMore)
	assert.NotNil(t, resp.NextCursor)
}

func TestSearchFeedsResponse_Construction(t *testing.T) {
	feeds := createSampleFeeds()
	protoFeeds := convertFeedsToProto(feeds)
	offset := int32(20)

	resp := &feedsv2.SearchFeedsResponse{
		Data:       protoFeeds,
		NextCursor: &offset,
		HasMore:    true,
	}

	assert.Len(t, resp.Data, 2)
	assert.True(t, resp.HasMore)
	assert.Equal(t, int32(20), *resp.NextCursor)
}

// =============================================================================
// Phase 6: StreamSummarize Tests
// =============================================================================

func TestStreamSummarize_RequiresAuth(t *testing.T) {
	handler := createTestHandler()
	ctx := context.Background() // No auth

	feedURL := "https://example.com/article"
	req := connect.NewRequest(&feedsv2.StreamSummarizeRequest{
		FeedUrl: &feedURL,
	})

	// Create a mock stream (we just need to test auth check)
	err := handler.StreamSummarize(ctx, req, nil)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestStreamSummarize_RequiresFeedURLOrArticleID(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	// Neither feed_url nor article_id provided
	req := connect.NewRequest(&feedsv2.StreamSummarizeRequest{})

	err := handler.StreamSummarize(ctx, req, nil)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestStreamSummarizeResponse_Construction(t *testing.T) {
	articleID := "test-article-id"
	summary := "This is a test summary"

	// Test cached response construction
	cachedResp := &feedsv2.StreamSummarizeResponse{
		Chunk:       "",
		IsFinal:     true,
		ArticleId:   articleID,
		IsCached:    true,
		FullSummary: &summary,
	}

	assert.Equal(t, articleID, cachedResp.ArticleId)
	assert.True(t, cachedResp.IsCached)
	assert.True(t, cachedResp.IsFinal)
	assert.Equal(t, summary, *cachedResp.FullSummary)

	// Test streaming chunk construction
	chunkText := "This is a chunk"
	chunkResp := &feedsv2.StreamSummarizeResponse{
		Chunk:     chunkText,
		IsFinal:   false,
		ArticleId: articleID,
		IsCached:  false,
	}

	assert.Equal(t, chunkText, chunkResp.Chunk)
	assert.False(t, chunkResp.IsFinal)
	assert.False(t, chunkResp.IsCached)
}

func TestParseSSESummary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text passthrough",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "extracts data from SSE",
			input:    "data: Hello\ndata: World\n",
			expected: "HelloWorld",
		},
		{
			name:     "handles empty SSE data",
			input:    "data: \n",
			expected: "",
		},
		{
			name:     "handles mixed content",
			input:    "event: message\ndata: Test\n",
			expected: "Test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSSESummary(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSSEData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extracts plain JSON string",
			input:    "data: \"Hello World\"\n",
			expected: "Hello World",
		},
		{
			name:     "decodes escaped Unicode characters",
			input:    "data: \"2025\\u5e74\\u306e\\u30cb\\u30e5\\u30fc\\u30b9\"\n",
			expected: "2025年のニュース",
		},
		{
			name:     "handles multiple data lines with Unicode",
			input:    "data: \"\\u3053\\u3093\\u306b\\u3061\\u306f\"\ndata: \"\\u4e16\\u754c\"\n",
			expected: "こんにちは世界",
		},
		{
			name:     "handles non-JSON content as fallback",
			input:    "data: plain text\n",
			expected: "plain text",
		},
		{
			name:     "handles empty data",
			input:    "data: \n",
			expected: "",
		},
		{
			name:     "ignores non-data lines",
			input:    "event: message\ndata: \"Test\"\nid: 123\n",
			expected: "Test",
		},
		{
			name:     "handles mixed JSON and non-JSON",
			input:    "data: \"JSON string\"\ndata: plain text\n",
			expected: "JSON stringplain text",
		},
		{
			name:     "handles empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSSEData(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Phase 7: MarkAsRead Tests
// =============================================================================

func TestMarkAsRead_RequiresAuth(t *testing.T) {
	handler := createTestHandler()
	ctx := context.Background() // No auth

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/article",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestMarkAsRead_RequiresArticleURL(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "", // Empty URL
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestMarkAsRead_InvalidURL(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "://invalid-url", // Invalid URL
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestMarkAsRead_Success(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/article",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Feed read status updated", resp.Msg.Message)
}

func TestMarkAsReadResponse_Construction(t *testing.T) {
	resp := &feedsv2.MarkAsReadResponse{
		Message: "Feed read status updated",
	}

	assert.Equal(t, "Feed read status updated", resp.Message)
}

// =============================================================================
// Phase 7 (TDD): MarkAsRead Error Handling Tests
// =============================================================================

// Mock that returns ErrFeedNotFound
type mockUpdateArticleStatusPortReturnsNotFound struct{}

func (m *mockUpdateArticleStatusPortReturnsNotFound) MarkArticleAsRead(ctx context.Context, articleURL url.URL) error {
	return domain.ErrFeedNotFound
}

// Mock that returns generic error
type mockUpdateArticleStatusPortReturnsError struct{}

func (m *mockUpdateArticleStatusPortReturnsError) MarkArticleAsRead(ctx context.Context, articleURL url.URL) error {
	return assert.AnError // Generic error from testify
}

// Test that domain.ErrFeedNotFound returns HTTP 404
func TestHandler_MarkAsRead_FeedNotFound_Returns404(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create usecase with mock that returns ErrFeedNotFound
	articlesReadingStatusUsecase := reading_status.NewArticlesReadingStatusUsecase(&mockUpdateArticleStatusPortReturnsNotFound{})

	container := &di.ApplicationComponents{
		ArticlesReadingStatusUsecase: articlesReadingStatusUsecase,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(container, cfg, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/nonexistent",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	// Should return NotFound error, not Internal error
	require.Error(t, err)
	require.Nil(t, resp)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(), "Expected HTTP 404 Not Found")
	assert.Contains(t, connectErr.Message(), "feed not found", "Error message should mention 'feed not found'")
}

// Test that non-domain errors return HTTP 500
func TestHandler_MarkAsRead_DatabaseError_Returns500(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create usecase with mock that returns generic error
	articlesReadingStatusUsecase := reading_status.NewArticlesReadingStatusUsecase(&mockUpdateArticleStatusPortReturnsError{})

	container := &di.ApplicationComponents{
		ArticlesReadingStatusUsecase: articlesReadingStatusUsecase,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(container, cfg, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/article",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	// Should return Internal error for non-domain errors
	require.Error(t, err)
	require.Nil(t, resp)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInternal, connectErr.Code(), "Expected HTTP 500 Internal Server Error")
	// Should NOT leak internal error details to client - now uses safe message with error ID
	assert.Contains(t, connectErr.Message(), "An unexpected error occurred", "Error message should be generic and user-friendly")
	assert.Contains(t, connectErr.Message(), "Error ID:", "Error message should contain Error ID for traceability")
}

// =============================================================================
// resolveArticle: DB Content Priority Tests
// =============================================================================

func TestResolveArticle_DBContentPrioritizedOverRequestContent(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create pgxmock pool
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Create repository with mock pool
	repo := alt_db.NewAltDBRepository(mock)

	// Create container with mock repository
	container := &di.ApplicationComponents{
		AltDBRepository: repo,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(container, cfg, slog.Default())
	ctx := createAuthContext()

	// Setup mock: FetchArticleByID should return article with DB content
	articleID := "test-article-id-123"
	dbTitle := "DB Title"
	dbContent := "This is clean content from database"
	dbURL := "https://example.com/article"

	// Expected query for FetchArticleByID
	mock.ExpectQuery(`SELECT id, title, content, url FROM articles WHERE id = \$1`).
		WithArgs(articleID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url"}).
			AddRow(articleID, dbTitle, dbContent, dbURL))

	// Call resolveArticle with both articleID and request content (which should be ignored)
	requestContent := "<html><body>This is raw HTML content from request that should be IGNORED</body></html>"
	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(ctx, "", articleID, requestContent, "")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, dbTitle, resolvedTitle, "Title should come from DB")
	assert.Equal(t, dbContent, resolvedContent, "Content should come from DB, not request content")
	assert.NotEqual(t, requestContent, resolvedContent, "Request content should be ignored when DB has content")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveArticle_FallbackToRequestContentWhenDBEmpty(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create pgxmock pool
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Create repository with mock pool
	repo := alt_db.NewAltDBRepository(mock)

	// Create container with mock repository
	container := &di.ApplicationComponents{
		AltDBRepository: repo,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(container, cfg, slog.Default())
	ctx := createAuthContext()

	// Setup mock: FetchArticleByID returns article with empty content
	articleID := "test-article-id-456"
	dbTitle := "DB Title"
	dbContent := "" // Empty content in DB
	dbURL := "https://example.com/article"

	// Expected query for FetchArticleByID
	mock.ExpectQuery(`SELECT id, title, content, url FROM articles WHERE id = \$1`).
		WithArgs(articleID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url"}).
			AddRow(articleID, dbTitle, dbContent, dbURL))

	// Call resolveArticle with both articleID and request content
	requestContent := "Request content as fallback"
	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(ctx, "", articleID, requestContent, "Request Title")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "Request Title", resolvedTitle, "Should use provided title when DB title would be overwritten by empty string")
	assert.Equal(t, requestContent, resolvedContent, "Should fallback to request content when DB content is empty")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveArticle_ErrorWhenDBEmptyAndNoRequestContent(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create pgxmock pool
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Create repository with mock pool
	repo := alt_db.NewAltDBRepository(mock)

	// Create container with mock repository
	container := &di.ApplicationComponents{
		AltDBRepository: repo,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(container, cfg, slog.Default())
	ctx := createAuthContext()

	// Setup mock: FetchArticleByID returns article with empty content
	articleID := "test-article-id-789"
	dbTitle := "DB Title"
	dbContent := "" // Empty content in DB
	dbURL := "https://example.com/article"

	// Expected query for FetchArticleByID
	mock.ExpectQuery(`SELECT id, title, content, url FROM articles WHERE id = \$1`).
		WithArgs(articleID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url"}).
			AddRow(articleID, dbTitle, dbContent, dbURL))

	// Call resolveArticle with articleID but no request content
	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(ctx, "", articleID, "", "")

	require.Error(t, err, "Should return error when both DB and request content are empty")
	assert.Contains(t, err.Error(), "content is empty")
	assert.Empty(t, resolvedArticleID)
	assert.Empty(t, resolvedTitle)
	assert.Empty(t, resolvedContent)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveArticle_FallbackToRequestContentWhenArticleNotInDB(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Create pgxmock pool
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Create repository with mock pool
	repo := alt_db.NewAltDBRepository(mock)

	// Create container with mock repository
	container := &di.ApplicationComponents{
		AltDBRepository: repo,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(container, cfg, slog.Default())
	ctx := createAuthContext()

	// Setup mock: FetchArticleByID returns no rows (article not found)
	articleID := "non-existent-article-id"

	// Expected query for FetchArticleByID - returns empty result
	mock.ExpectQuery(`SELECT id, title, content, url FROM articles WHERE id = \$1`).
		WithArgs(articleID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url"}))

	// Call resolveArticle with articleID and request content
	requestContent := "Fallback content when article not in DB"
	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(ctx, "", articleID, requestContent, "Request Title")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "Request Title", resolvedTitle)
	assert.Equal(t, requestContent, resolvedContent, "Should use request content when article not found in DB")

	require.NoError(t, mock.ExpectationsWereMet())
}
