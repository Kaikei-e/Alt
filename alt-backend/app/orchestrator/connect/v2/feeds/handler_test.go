package feeds

import (
	"context"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/orchestrator/usecase/fetch_feed_stats_usecase"
	"alt/orchestrator/usecase/reading_status"
	"alt/orchestrator/usecase/resolve_article_usecase"
	"alt/utils/logger"
)

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

type mockUpdateArticleStatusPort struct{}

func (m *mockUpdateArticleStatusPort) MarkArticleAsRead(ctx context.Context, articleURL url.URL, userID uuid.UUID) error {
	return nil
}

func createTestHandler() *Handler {
	logger.InitLogger()

	feedAmountUsecase := fetch_feed_stats_usecase.NewFeedsCountUsecase(&mockFeedAmountPort{})
	summarizedUsecase := fetch_feed_stats_usecase.NewSummarizedArticlesCountUsecase(&mockSummarizedArticlesCountPort{})
	totalArticlesUsecase := fetch_feed_stats_usecase.NewTotalArticlesCountUsecase(&mockTotalArticlesCountPort{})
	unsummarizedUsecase := fetch_feed_stats_usecase.NewUnsummarizedArticlesCountUsecase(&mockUnsummarizedArticlesCountPort{})
	articlesReadingStatusUsecase := reading_status.NewArticlesReadingStatusUsecase(&mockUpdateArticleStatusPort{})

	deps := FeedHandlerDeps{
		FeedAmount:            feedAmountUsecase,
		SummarizedCount:       summarizedUsecase,
		TotalCount:            totalArticlesUsecase,
		UnsummarizedCount:     unsummarizedUsecase,
		ArticlesReadingStatus: articlesReadingStatusUsecase,
		ResolveArticle:        resolve_article_usecase.New(nil, nil),
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handlerLogger := slog.Default()

	return NewHandler(deps, cfg, handlerLogger)
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

func createSampleFeeds() []*domain.FeedItem {
	now := time.Now()
	return []*domain.FeedItem{
		{
			Title:           "Test Feed 1",
			Description:     "Test Description 1",
			Link:            "https://example.com/feed1",
			Published:       now.Format(time.RFC3339),
			PublishedParsed: now,
			Author:          domain.Author{Name: "Author 1"},
		},
		{
			Title:           "Test Feed 2",
			Description:     "Test Description 2",
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

	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, protoFeeds[0].Id)
	assert.Equal(t, "Test Feed 1", protoFeeds[0].Title)
	assert.Equal(t, "Test Description 1", protoFeeds[0].Description)
	assert.Equal(t, "https://example.com/feed1", protoFeeds[0].Link)
	assert.Equal(t, "Author 1", protoFeeds[0].Author)
	assert.NotEmpty(t, protoFeeds[0].CreatedAt)
	assert.NotEmpty(t, protoFeeds[0].Published)

	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, protoFeeds[1].Id)
	assert.NotEqual(t, protoFeeds[0].Id, protoFeeds[1].Id)
	assert.Equal(t, "Test Feed 2", protoFeeds[1].Title)
}

func TestConvertFeedsToProto_IsReadField(t *testing.T) {
	now := time.Now()
	feeds := []*domain.FeedItem{
		{
			Title:           "Unread Feed",
			Description:     "Unread description",
			Link:            "https://example.com/unread",
			Published:       now.Format(time.RFC3339),
			PublishedParsed: now,
			IsRead:          false,
		},
		{
			Title:           "Read Feed",
			Description:     "Read description",
			Link:            "https://example.com/read",
			Published:       now.Add(-time.Hour).Format(time.RFC3339),
			PublishedParsed: now.Add(-time.Hour),
			IsRead:          true,
		},
	}

	protoFeeds := convertFeedsToProto(feeds)

	require.Len(t, protoFeeds, 2)
	assert.False(t, protoFeeds[0].IsRead, "First feed should be unread")
	assert.True(t, protoFeeds[1].IsRead, "Second feed should be read")
}

func TestConvertFeedsToProto_EmptyList(t *testing.T) {
	protoFeeds := convertFeedsToProto([]*domain.FeedItem{})
	assert.Len(t, protoFeeds, 0)
	assert.NotNil(t, protoFeeds)
}

func TestDeriveNextCursor_WithHasMore(t *testing.T) {
	feeds := createSampleFeeds()
	cursor := deriveNextCursor(feeds, true)

	require.NotNil(t, cursor)
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

func TestGetAllFeeds_RequiresAuth(t *testing.T) {
	handler := createTestHandler()
	ctx := context.Background()

	req := connect.NewRequest(&feedsv2.GetAllFeedsRequest{})
	resp, err := handler.GetAllFeeds(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestGetAllFeedsResponse_Construction(t *testing.T) {
	feeds := createSampleFeeds()
	protoFeeds := convertFeedsToProto(feeds)
	nextCursor := deriveNextCursor(feeds, true)

	resp := &feedsv2.GetAllFeedsResponse{
		Data:       protoFeeds,
		NextCursor: nextCursor,
		HasMore:    true,
	}

	assert.Len(t, resp.Data, 2)
	assert.True(t, resp.HasMore)
	assert.NotNil(t, resp.NextCursor)
}

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
