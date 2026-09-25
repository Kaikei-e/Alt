package articles

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
)

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
	deps := ArticleHandlerDeps{
		FetchInoreaderSummary: mockUsecase,
	}
	cfg := &config.Config{}
	logger := slog.Default()
	return NewHandler(deps, cfg, logger)
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
