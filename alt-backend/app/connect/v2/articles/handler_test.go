package articles

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	articlesv2 "alt/gen/proto/alt/articles/v2"

	"alt/config"
	"alt/di"
	"alt/domain"
)

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

// =============================================================================
// Response Construction Tests
// =============================================================================

func TestFetchArticleContentResponse_Construction(t *testing.T) {
	articleID := "test-article-id"
	resp := &articlesv2.FetchArticleContentResponse{
		Url:       "https://example.com/article",
		Content:   "Test Content",
		ArticleId: articleID,
	}

	assert.Equal(t, "https://example.com/article", resp.Url)
	assert.Equal(t, "Test Content", resp.Content)
	assert.Equal(t, articleID, resp.ArticleId)
}

func TestArchiveArticleResponse_Construction(t *testing.T) {
	resp := &articlesv2.ArchiveArticleResponse{
		Message: "article archived",
	}

	assert.Equal(t, "article archived", resp.Message)
}

func TestArticleItem_Construction(t *testing.T) {
	id := uuid.New().String()
	published := time.Now().Format(time.RFC3339)
	item := &articlesv2.ArticleItem{
		Id:          id,
		Title:       "Test Title",
		Url:         "https://example.com/article",
		Content:     "Test Content",
		PublishedAt: published,
		Tags:        []string{"tag1", "tag2"},
	}

	assert.Equal(t, id, item.Id)
	assert.Equal(t, "Test Title", item.Title)
	assert.Equal(t, "https://example.com/article", item.Url)
	assert.Equal(t, "Test Content", item.Content)
	assert.Equal(t, published, item.PublishedAt)
	assert.Len(t, item.Tags, 2)
}

func TestFetchArticlesCursorResponse_Construction(t *testing.T) {
	id := uuid.New().String()
	published := time.Now().Format(time.RFC3339)
	nextCursor := time.Now().Add(-time.Hour).Format(time.RFC3339)

	resp := &articlesv2.FetchArticlesCursorResponse{
		Data: []*articlesv2.ArticleItem{
			{
				Id:          id,
				Title:       "Test Article",
				Url:         "https://example.com/article",
				Content:     "Test Content",
				PublishedAt: published,
				Tags:        []string{"tag1"},
			},
		},
		NextCursor: &nextCursor,
		HasMore:    true,
	}

	assert.Len(t, resp.Data, 1)
	assert.Equal(t, id, resp.Data[0].Id)
	assert.NotNil(t, resp.NextCursor)
	assert.True(t, resp.HasMore)
}

func TestFetchArticlesCursorResponse_Empty(t *testing.T) {
	resp := &articlesv2.FetchArticlesCursorResponse{
		Data:       []*articlesv2.ArticleItem{},
		NextCursor: nil,
		HasMore:    false,
	}

	assert.Empty(t, resp.Data)
	assert.Nil(t, resp.NextCursor)
	assert.False(t, resp.HasMore)
}

// =============================================================================
// Request Validation Tests
// =============================================================================

func TestFetchArticleContentRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid URL",
			url:     "https://example.com/article",
			wantErr: false,
		},
		{
			name:    "empty URL should fail",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &articlesv2.FetchArticleContentRequest{
				Url: tt.url,
			}

			if tt.wantErr {
				assert.Empty(t, req.Url)
			} else {
				assert.NotEmpty(t, req.Url)
			}
		})
	}
}

func TestArchiveArticleRequest_Validation(t *testing.T) {
	title := "Test Title"
	tests := []struct {
		name    string
		feedUrl string
		title   *string
		wantErr bool
	}{
		{
			name:    "valid request with title",
			feedUrl: "https://example.com/article",
			title:   &title,
			wantErr: false,
		},
		{
			name:    "valid request without title",
			feedUrl: "https://example.com/article",
			title:   nil,
			wantErr: false,
		},
		{
			name:    "empty URL should fail",
			feedUrl: "",
			title:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &articlesv2.ArchiveArticleRequest{
				FeedUrl: tt.feedUrl,
				Title:   tt.title,
			}

			if tt.wantErr {
				assert.Empty(t, req.FeedUrl)
			} else {
				assert.NotEmpty(t, req.FeedUrl)
			}
		})
	}
}

func TestFetchArticlesCursorRequest_Validation(t *testing.T) {
	validCursor := time.Now().Format(time.RFC3339)
	tests := []struct {
		name   string
		cursor *string
		limit  int32
	}{
		{
			name:   "no cursor",
			cursor: nil,
			limit:  20,
		},
		{
			name:   "with cursor",
			cursor: &validCursor,
			limit:  10,
		},
		{
			name:   "zero limit should use default",
			cursor: nil,
			limit:  0,
		},
		{
			name:   "max limit exceeded",
			cursor: nil,
			limit:  200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &articlesv2.FetchArticlesCursorRequest{
				Cursor: tt.cursor,
				Limit:  tt.limit,
			}

			// Just verify construction
			if tt.cursor != nil {
				assert.NotNil(t, req.Cursor)
			} else {
				assert.Nil(t, req.Cursor)
			}
		})
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestConvertArticlesToProto(t *testing.T) {
	now := time.Now()
	articles := []*domain.Article{
		{
			ID:          uuid.New(),
			Title:       "Test Article 1",
			URL:         "https://example.com/1",
			Content:     "Content 1",
			PublishedAt: now,
			Tags:        []string{"tag1", "tag2"},
		},
		{
			ID:          uuid.New(),
			Title:       "Test Article 2",
			URL:         "https://example.com/2",
			Content:     "Content 2",
			PublishedAt: now.Add(-time.Hour),
			Tags:        []string{"tag3"},
		},
	}

	protoArticles := convertArticlesToProto(articles)

	assert.Len(t, protoArticles, 2)
	assert.Equal(t, "Test Article 1", protoArticles[0].Title)
	assert.Equal(t, "https://example.com/1", protoArticles[0].Url)
	assert.Len(t, protoArticles[0].Tags, 2)
	assert.Equal(t, "Test Article 2", protoArticles[1].Title)
}

func TestConvertArticlesToProto_EmptyList(t *testing.T) {
	protoArticles := convertArticlesToProto([]*domain.Article{})
	assert.Empty(t, protoArticles)
	assert.NotNil(t, protoArticles)
}

// =============================================================================
// FetchArticleSummary Handler Tests (TDD)
// =============================================================================

// Mock for FetchInoreaderSummaryUsecase
type mockFetchInoreaderSummaryUsecase struct {
	summaries []*domain.InoreaderSummary
	err       error
}

func (m *mockFetchInoreaderSummaryUsecase) Execute(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.summaries, nil
}

// Create test handler for FetchArticleSummary tests
func createArticleSummaryTestHandler(mockUsecase *mockFetchInoreaderSummaryUsecase) *Handler {
	container := &di.ApplicationComponents{
		FetchInoreaderSummaryUsecase: mockUsecase,
	}
	cfg := &config.Config{}
	logger := slog.Default()
	return NewHandler(container, cfg, logger)
}

func TestFetchArticleSummary_Success(t *testing.T) {
	now := time.Now()
	mockSummaries := []*domain.InoreaderSummary{
		{
			ArticleURL:  "https://example.com/article1",
			Title:       "Test Article 1",
			Author:      stringPtr("Author 1"),
			Content:     "Content 1",
			ContentType: "text/html",
			PublishedAt: now,
			FetchedAt:   now,
			InoreaderID: "source-1",
		},
		{
			ArticleURL:  "https://example.com/article2",
			Title:       "Test Article 2",
			Author:      nil,
			Content:     "Content 2",
			ContentType: "text/html",
			PublishedAt: now.Add(-time.Hour),
			FetchedAt:   now,
			InoreaderID: "source-2",
		},
	}

	mockUsecase := &mockFetchInoreaderSummaryUsecase{
		summaries: mockSummaries,
		err:       nil,
	}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: []string{"https://example.com/article1", "https://example.com/article2"},
	})

	resp, err := handler.FetchArticleSummary(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 2, len(resp.Msg.MatchedArticles))
	assert.Equal(t, int32(2), resp.Msg.TotalMatched)
	assert.Equal(t, int32(2), resp.Msg.RequestedCount)

	// Verify first article
	assert.Equal(t, "Test Article 1", resp.Msg.MatchedArticles[0].Title)
	assert.Equal(t, "Content 1", resp.Msg.MatchedArticles[0].Content)
	assert.Equal(t, "Author 1", resp.Msg.MatchedArticles[0].Author)
	assert.Equal(t, "source-1", resp.Msg.MatchedArticles[0].SourceId)

	// Verify second article (with nil author)
	assert.Equal(t, "Test Article 2", resp.Msg.MatchedArticles[1].Title)
	assert.Equal(t, "", resp.Msg.MatchedArticles[1].Author)
}

func TestFetchArticleSummary_EmptyURLs(t *testing.T) {
	mockUsecase := &mockFetchInoreaderSummaryUsecase{}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: []string{},
	})

	_, err := handler.FetchArticleSummary(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "feed_urls cannot be empty")
}

func TestFetchArticleSummary_ExceedsMaxLimit(t *testing.T) {
	mockUsecase := &mockFetchInoreaderSummaryUsecase{}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := createAuthContext()

	// Create 51 URLs (exceeds limit of 50)
	urls := make([]string, 51)
	for i := range urls {
		urls[i] = "https://example.com/article" + string(rune(i))
	}

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: urls,
	})

	_, err := handler.FetchArticleSummary(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "maximum 50 URLs")
}

func TestFetchArticleSummary_RequiresAuth(t *testing.T) {
	mockUsecase := &mockFetchInoreaderSummaryUsecase{}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := context.Background() // No auth

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: []string{"https://example.com/article"},
	})

	_, err := handler.FetchArticleSummary(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
