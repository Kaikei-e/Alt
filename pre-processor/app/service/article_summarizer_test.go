package service

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"pre-processor/models"
	"pre-processor/repository"

	"github.com/stretchr/testify/assert"
)

func testLoggerSummarizer() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestArticleSummarizerService_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ArticleSummarizerService interface", func(t *testing.T) {
		// GREEN PHASE: Test that service implements interface
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		// Verify interface compliance at compile time
		var _ = service

		assert.NotNil(t, service)
	})
}

func TestArticleSummarizerService_SummarizeArticles(t *testing.T) {
	t.Run("should return empty result with minimal implementation", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		result, err := service.SummarizeArticles(context.Background(), 10)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ProcessedCount)
		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 0, result.ErrorCount)
		assert.False(t, result.HasMore)
		assert.Empty(t, result.Errors)
	})
}

func TestArticleSummarizerService_HasUnsummarizedArticles(t *testing.T) {
	t.Run("should return false with minimal implementation", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		hasArticles, err := service.HasUnsummarizedArticles(context.Background())

		assert.NoError(t, err)
		assert.False(t, hasArticles)
	})
}

func TestArticleSummarizerService_ResetPagination(t *testing.T) {
	t.Run("should reset pagination without error", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		err := service.ResetPagination()

		assert.NoError(t, err)
	})
}

// --- Local mocks for context cancellation tests ---

// stubArticleRepo returns a fixed set of articles from FindForSummarization.
type stubArticleRepo struct {
	repository.ArticleRepository
	articles []*models.Article
}

func (m *stubArticleRepo) FindForSummarization(_ context.Context, _ *repository.Cursor, _ int) ([]*models.Article, *repository.Cursor, error) {
	return m.articles, nil, nil
}

// trackingAPIRepo tracks calls to SummarizeArticle and optionally cancels a context.
type trackingAPIRepo struct {
	repository.ExternalAPIRepository
	callCount    int
	cancelOnCall int // cancel the context on this call number (1-indexed), 0 = never
	cancelFunc   context.CancelFunc
}

func (m *trackingAPIRepo) SummarizeArticle(_ context.Context, article *models.Article, _ string) (*models.SummarizedContent, error) {
	m.callCount++
	if m.cancelOnCall > 0 && m.callCount == m.cancelOnCall {
		m.cancelFunc()
	}
	return &models.SummarizedContent{
		ArticleID:       article.ID,
		SummaryJapanese: "テスト要約",
	}, nil
}

// noopSummaryRepo accepts all Create calls without error.
type noopSummaryRepo struct {
	repository.SummaryRepository
}

func (m *noopSummaryRepo) Create(_ context.Context, _ *models.ArticleSummary) error {
	return nil
}

func TestArticleSummarizerService_SummarizeArticles_ContextCanceled(t *testing.T) {
	t.Run("should skip remaining articles when context is canceled mid-batch", func(t *testing.T) {
		articles := []*models.Article{
			{ID: "1", Content: "content1", UserID: "user1", Title: "title1"},
			{ID: "2", Content: "content2", UserID: "user1", Title: "title2"},
			{ID: "3", Content: "content3", UserID: "user1", Title: "title3"},
		}

		ctx, cancel := context.WithCancel(context.Background())
		apiRepo := &trackingAPIRepo{
			cancelOnCall: 1, // cancel after processing the first article
			cancelFunc:   cancel,
		}

		svc := NewArticleSummarizerService(
			&stubArticleRepo{articles: articles},
			&noopSummaryRepo{},
			apiRepo,
			testLoggerSummarizer(),
		)

		result, err := svc.SummarizeArticles(ctx, 10)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		// Only the first article should have been processed via the API
		assert.Equal(t, 1, apiRepo.callCount,
			"only 1 article should be processed before context cancellation stops the loop")
		assert.Equal(t, 1, result.SuccessCount,
			"only 1 article should succeed")
	})

	t.Run("should process zero articles when context is already canceled", func(t *testing.T) {
		articles := []*models.Article{
			{ID: "1", Content: "content1", UserID: "user1", Title: "title1"},
			{ID: "2", Content: "content2", UserID: "user1", Title: "title2"},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel before processing

		apiRepo := &trackingAPIRepo{}

		svc := NewArticleSummarizerService(
			&stubArticleRepo{articles: articles},
			&noopSummaryRepo{},
			apiRepo,
			testLoggerSummarizer(),
		)

		result, err := svc.SummarizeArticles(ctx, 10)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, apiRepo.callCount,
			"no articles should be processed when context is already canceled")
		assert.Equal(t, 0, result.SuccessCount)
	})
}
