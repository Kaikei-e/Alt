package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/domain"

	"github.com/stretchr/testify/assert"
)

const testSyncOwnerUserID = "11111111-2222-4333-8444-555555555555"

func testLoggerBackfill() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// --- Stubs ---

type stubBackfillArticleRepo struct {
	SyncArticleRepository
	emptyFeedArticles []*domain.Article
	scannedThrough    time.Time
	fetchCursors      []time.Time
	upsertedArticles  []*domain.Article
	fetchErr          error
	upsertErr         error
}

func (s *stubBackfillArticleRepo) FetchInoreaderArticlesForEmptyFeeds(_ context.Context, fetchedAfter time.Time, _ int) ([]*domain.Article, time.Time, error) {
	s.fetchCursors = append(s.fetchCursors, fetchedAfter)
	return s.emptyFeedArticles, s.scannedThrough, s.fetchErr
}

func (s *stubBackfillArticleRepo) UpsertArticlesWithFeedID(_ context.Context, articles []*domain.Article) error {
	s.upsertedArticles = articles
	return s.upsertErr
}

// --- Tests ---

func TestBackfillEmptyFeeds_Success(t *testing.T) {
	t.Run("should backfill articles for empty feeds using configured owner user ID", func(t *testing.T) {
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

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.NoError(t, err)
		assert.Len(t, articleRepo.upsertedArticles, 2)
		assert.Equal(t, testSyncOwnerUserID, articleRepo.upsertedArticles[0].UserID)
		assert.Equal(t, testSyncOwnerUserID, articleRepo.upsertedArticles[1].UserID)
	})
}

func TestBackfillEmptyFeeds_NoArticles(t *testing.T) {
	t.Run("should handle no articles gracefully", func(t *testing.T) {
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: nil,
		}

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

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

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

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

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

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

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch inoreader articles for empty feeds")
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

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

		err := svc.BackfillEmptyFeeds(context.Background())

		assert.NoError(t, err)
		assert.Len(t, articleRepo.upsertedArticles, 1)
		assert.Equal(t, "pre-resolved-feed-uuid", articleRepo.upsertedArticles[0].FeedID)
	})
}

func TestBackfillEmptyFeeds_AdvancesCursorPastFilteredRows(t *testing.T) {
	t.Run("should advance the cursor when the empty-feed filter drops every scanned row", func(t *testing.T) {
		scannedThrough := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: nil,
			scannedThrough:    scannedThrough,
		}

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

		assert.NoError(t, svc.BackfillEmptyFeeds(context.Background()))
		assert.NoError(t, svc.BackfillEmptyFeeds(context.Background()))

		assert.Len(t, articleRepo.fetchCursors, 2)
		assert.True(t, articleRepo.fetchCursors[0].IsZero())
		assert.Equal(t, scannedThrough, articleRepo.fetchCursors[1])
	})
}

func TestBackfillEmptyFeeds_CursorUsesScanWatermark(t *testing.T) {
	t.Run("should advance the cursor to the last row scanned, not the last row returned", func(t *testing.T) {
		scannedThrough := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: []*domain.Article{
				{
					ID:        "inoreader-1",
					URL:       "https://example.com/article1",
					Title:     "Article 1",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedID:    "feed-uuid-1",
					CreatedAt: scannedThrough.Add(-6 * time.Hour),
				},
			},
			scannedThrough: scannedThrough,
		}

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

		assert.NoError(t, svc.BackfillEmptyFeeds(context.Background()))
		assert.NoError(t, svc.BackfillEmptyFeeds(context.Background()))

		assert.Len(t, articleRepo.fetchCursors, 2)
		assert.Equal(t, scannedThrough, articleRepo.fetchCursors[1])
	})
}

func TestBackfillEmptyFeeds_KeepsCursorWhenUpsertFails(t *testing.T) {
	t.Run("should not advance the cursor when the batch was never persisted", func(t *testing.T) {
		scannedThrough := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		articleRepo := &stubBackfillArticleRepo{
			emptyFeedArticles: []*domain.Article{
				{
					ID:        "inoreader-1",
					URL:       "https://example.com/article1",
					Title:     "Article 1",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedID:    "feed-uuid-1",
					CreatedAt: scannedThrough,
				},
			},
			scannedThrough: scannedThrough,
			upsertErr:      fmt.Errorf("datahub unavailable"),
		}

		svc := NewArticleSyncService(articleRepo, testSyncOwnerUserID, testLoggerBackfill())

		assert.Error(t, svc.BackfillEmptyFeeds(context.Background()))
		assert.Error(t, svc.BackfillEmptyFeeds(context.Background()))

		assert.Len(t, articleRepo.fetchCursors, 2)
		assert.True(t, articleRepo.fetchCursors[1].IsZero())
	})
}
