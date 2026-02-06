package summarize

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// stubArticleRepo implements repository.ArticleRepository for testing
type stubArticleRepo struct {
	findByIDResult *domain.Article
	findByIDErr    error
}

func (s *stubArticleRepo) FindByID(_ context.Context, _ string) (*domain.Article, error) {
	return s.findByIDResult, s.findByIDErr
}
func (s *stubArticleRepo) Create(_ context.Context, _ *domain.Article) error { return nil }
func (s *stubArticleRepo) CheckExists(_ context.Context, _ []string) (bool, error) {
	return false, nil
}
func (s *stubArticleRepo) FindForSummarization(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
	return nil, nil, nil
}
func (s *stubArticleRepo) HasUnsummarizedArticles(_ context.Context) (bool, error) {
	return false, nil
}
func (s *stubArticleRepo) FetchInoreaderArticles(_ context.Context, _ time.Time) ([]*domain.Article, error) {
	return nil, nil
}
func (s *stubArticleRepo) UpsertArticles(_ context.Context, _ []*domain.Article) error { return nil }

// stubSummaryRepo implements repository.SummaryRepository for testing
type stubSummaryRepo struct {
	createErr    error
	createCalled bool
}

func (s *stubSummaryRepo) Create(_ context.Context, _ *domain.ArticleSummary) error {
	s.createCalled = true
	return s.createErr
}
func (s *stubSummaryRepo) FindArticlesWithSummaries(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.ArticleWithSummary, *domain.Cursor, error) {
	return nil, nil, nil
}
func (s *stubSummaryRepo) Delete(_ context.Context, _ string) error { return nil }
func (s *stubSummaryRepo) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// stubAPIRepo implements repository.ExternalAPIRepository for testing
type stubAPIRepo struct {
	summarizeResult *domain.SummarizedContent
	summarizeErr    error
	summarizeCalled bool
}

func (s *stubAPIRepo) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	s.summarizeCalled = true
	return s.summarizeResult, s.summarizeErr
}
func (s *stubAPIRepo) StreamSummarizeArticle(_ context.Context, _ *domain.Article, _ string) (io.ReadCloser, error) {
	return nil, nil
}
func (s *stubAPIRepo) CheckHealth(_ context.Context, _ string) error { return nil }
func (s *stubAPIRepo) GetSystemUserID(_ context.Context) (string, error) {
	return "system", nil
}

func TestOnDemandService_ResolveArticle(t *testing.T) {
	t.Run("should resolve article from DB when content is empty", func(t *testing.T) {
		articleRepo := &stubArticleRepo{
			findByIDResult: &domain.Article{
				ID:      "art-1",
				Content: "This is article content for testing purposes.",
				Title:   "Test Article",
				UserID:  "user-1",
			},
		}
		svc := NewOnDemandService(articleRepo, nil, nil, testLogger())

		resolved, err := svc.ResolveArticle(context.Background(), SummarizeRequest{
			ArticleID: "art-1",
		})

		require.NoError(t, err)
		assert.Equal(t, "art-1", resolved.ArticleID)
		assert.Equal(t, "Test Article", resolved.Title)
		assert.Equal(t, "user-1", resolved.UserID)
		assert.NotEmpty(t, resolved.Content)
	})

	t.Run("should use provided content when not empty", func(t *testing.T) {
		articleRepo := &stubArticleRepo{
			findByIDResult: &domain.Article{
				ID:     "art-1",
				UserID: "user-1",
			},
		}
		svc := NewOnDemandService(articleRepo, nil, nil, testLogger())

		resolved, err := svc.ResolveArticle(context.Background(), SummarizeRequest{
			ArticleID: "art-1",
			Content:   "Provided content",
			Title:     "Provided Title",
		})

		require.NoError(t, err)
		assert.Equal(t, "Provided Title", resolved.Title)
		assert.Contains(t, resolved.Content, "Provided content")
	})

	t.Run("should return error when article not found", func(t *testing.T) {
		articleRepo := &stubArticleRepo{findByIDResult: nil}
		svc := NewOnDemandService(articleRepo, nil, nil, testLogger())

		_, err := svc.ResolveArticle(context.Background(), SummarizeRequest{
			ArticleID: "nonexistent",
		})

		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrArticleNotFound))
	})

	t.Run("should return error when DB content is empty", func(t *testing.T) {
		articleRepo := &stubArticleRepo{
			findByIDResult: &domain.Article{
				ID:      "art-1",
				Content: "",
			},
		}
		svc := NewOnDemandService(articleRepo, nil, nil, testLogger())

		_, err := svc.ResolveArticle(context.Background(), SummarizeRequest{
			ArticleID: "art-1",
		})

		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrArticleContentEmpty))
	})
}

func TestOnDemandService_Summarize(t *testing.T) {
	t.Run("should summarize article and save to DB", func(t *testing.T) {
		articleRepo := &stubArticleRepo{
			findByIDResult: &domain.Article{
				ID:      "art-1",
				Content: "This is a long enough article content for testing.",
				Title:   "Test Article",
				UserID:  "user-1",
			},
		}
		summaryRepo := &stubSummaryRepo{}
		apiRepo := &stubAPIRepo{
			summarizeResult: &domain.SummarizedContent{
				ArticleID:       "art-1",
				SummaryJapanese: "テスト要約",
			},
		}
		svc := NewOnDemandService(articleRepo, summaryRepo, apiRepo, testLogger())

		result, err := svc.Summarize(context.Background(), SummarizeRequest{
			ArticleID: "art-1",
			Priority:  "high",
		})

		require.NoError(t, err)
		assert.Equal(t, "テスト要約", result.Summary)
		assert.Equal(t, "art-1", result.ArticleID)
		assert.True(t, summaryRepo.createCalled)
		assert.True(t, apiRepo.summarizeCalled)
	})

	t.Run("should return summary even when DB save fails", func(t *testing.T) {
		articleRepo := &stubArticleRepo{
			findByIDResult: &domain.Article{
				ID:      "art-1",
				Content: "Content for testing DB failure scenario.",
				Title:   "Test",
				UserID:  "user-1",
			},
		}
		summaryRepo := &stubSummaryRepo{createErr: errors.New("db error")}
		apiRepo := &stubAPIRepo{
			summarizeResult: &domain.SummarizedContent{
				ArticleID:       "art-1",
				SummaryJapanese: "要約テスト",
			},
		}
		svc := NewOnDemandService(articleRepo, summaryRepo, apiRepo, testLogger())

		result, err := svc.Summarize(context.Background(), SummarizeRequest{
			ArticleID: "art-1",
			Priority:  "high",
		})

		require.NoError(t, err)
		assert.Equal(t, "要約テスト", result.Summary)
	})

	t.Run("should return error when API fails", func(t *testing.T) {
		articleRepo := &stubArticleRepo{
			findByIDResult: &domain.Article{
				ID:      "art-1",
				Content: "Test content.",
				UserID:  "user-1",
			},
		}
		apiRepo := &stubAPIRepo{summarizeErr: errors.New("api error")}
		svc := NewOnDemandService(articleRepo, nil, apiRepo, testLogger())

		_, err := svc.Summarize(context.Background(), SummarizeRequest{
			ArticleID: "art-1",
			Priority:  "low",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate summary")
	})
}

func TestExtractText(t *testing.T) {
	t.Run("should extract text from HTML content", func(t *testing.T) {
		content := "<html><body><p>Hello World</p></body></html>"
		result := extractText(content, testLogger(), context.Background(), "test-id")
		assert.NotContains(t, result, "<html>")
		assert.NotContains(t, result, "<body>")
	})

	t.Run("should return plain text unchanged", func(t *testing.T) {
		content := "This is plain text without HTML."
		result := extractText(content, testLogger(), context.Background(), "test-id")
		assert.Equal(t, content, result)
	})
}
