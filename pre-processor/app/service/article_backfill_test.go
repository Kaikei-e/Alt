package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"

	"github.com/stretchr/testify/assert"
)

func testLoggerBackfill() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// --- Stubs ---

type stubBackfillArticleRepo struct {
	repository.ArticleRepository
	emptyFeedArticles []*domain.Article
	upsertedArticles  []*domain.Article
	fetchErr          error
	upsertErr         error
}

func (s *stubBackfillArticleRepo) FetchInoreaderArticlesForEmptyFeeds(_ context.Context, _ time.Time, _ int) ([]*domain.Article, error) {
	return s.emptyFeedArticles, s.fetchErr
}

func (s *stubBackfillArticleRepo) UpsertArticlesWithFeedID(_ context.Context, articles []*domain.Article) error {
	s.upsertedArticles = articles
	return s.upsertErr
}

type stubBackfillExternalAPI struct {
	repository.ExternalAPIRepository
	userID string
	err    error
}

func (s *stubBackfillExternalAPI) GetSystemUserID(_ context.Context) (string, error) {
	return s.userID, s.err
}

// --- Tests ---

func TestBackfillEmptyFeeds_Success(t *testing.T) {
	t.Run("should backfill articles for empty feeds", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: []*domain.Article{
				{
					ID:          "inoreader-1",
					URL:         "https://example.com/article1",
					Title:       "Article 1",
					Content:     "<p>This is a valid article content that is long enough to pass validation checks.</p>",
					FeedID:      "feed-uuid-1",
					PublishedAt: time.Now(),
				},
				{
					ID:          "inoreader-2",
					URL:         "https://example.com/article2",
					Title:       "Article 2",
					Content:     "<p>Another valid article with sufficient content length for the system.</p>",
					FeedID:      "feed-uuid-1",
					PublishedAt: time.Now(),
				},
			},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.NoError(t, err)
		assert.Len(t, articleRepo.upsertedArticles, 2)
		assert.Equal(t, "system-user-id", articleRepo.upsertedArticles[0].UserID)
		assert.Equal(t, "system-user-id", articleRepo.upsertedArticles[1].UserID)
	})
}

func TestBackfillEmptyFeeds_NoArticles(t *testing.T) {
	t.Run("should handle no articles gracefully", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: nil,
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.NoError(t, err)
		assert.Nil(t, articleRepo.upsertedArticles)
	})
}

func TestBackfillEmptyFeeds_SkipsEmptyContent(t *testing.T) {
	t.Run("should skip articles with empty content after sanitization", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: []*domain.Article{
				{
					ID:      "inoreader-1",
					URL:     "https://example.com/article1",
					Title:   "Good Article",
					Content: "<p>Valid content that should pass sanitization checks.</p>",
					FeedID:  "feed-uuid-1",
				},
				{
					ID:      "inoreader-2",
					URL:     "https://example.com/article2",
					Title:   "Bad Article",
					Content: "<script>alert('xss')</script>", // Will be empty after sanitization
					FeedID:  "feed-uuid-1",
				},
			},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.NoError(t, err)
		assert.Len(t, articleRepo.upsertedArticles, 1)
		assert.Equal(t, "https://example.com/article1", articleRepo.upsertedArticles[0].URL)
	})
}

func TestBackfillEmptyFeeds_SkipsEmptyTitle(t *testing.T) {
	t.Run("should skip articles with empty title", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: []*domain.Article{
				{
					ID:      "inoreader-1",
					URL:     "https://example.com/article1",
					Title:   "",
					Content: "<p>Valid content but missing title field.</p>",
					FeedID:  "feed-uuid-1",
				},
			},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.NoError(t, err)
		assert.Nil(t, articleRepo.upsertedArticles)
	})
}

func TestBackfillEmptyFeeds_FetchError(t *testing.T) {
	t.Run("should return error when fetch fails", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{
			fetchErr: fmt.Errorf("database connection error"),
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch inoreader articles for empty feeds")
	})
}

func TestBackfillEmptyFeeds_SystemUserError(t *testing.T) {
	t.Run("should return error when system user ID unavailable", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{}
		externalAPI := &stubBackfillExternalAPI{err: fmt.Errorf("auth service down")}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get system user id")
	})
}

func TestBackfillEmptyFeeds_PreservesFeedID(t *testing.T) {
	t.Run("should preserve FeedID from query without re-resolution", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: []*domain.Article{
				{
					ID:      "inoreader-1",
					URL:     "https://example.com/article1",
					Title:   "Article with pre-resolved FeedID",
					Content: "<p>Content with pre-resolved feed identifier from SQL join.</p>",
					FeedID:  "pre-resolved-feed-uuid",
				},
			},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.NoError(t, err)
		assert.Len(t, articleRepo.upsertedArticles, 1)
		assert.Equal(t, "pre-resolved-feed-uuid", articleRepo.upsertedArticles[0].FeedID)
	})
}
