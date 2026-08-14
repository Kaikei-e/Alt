package service

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"pre-processor/domain"

	"github.com/stretchr/testify/assert"
)

// stubSyncArticleRepo mirrors the driver query behind FetchInoreaderArticles
// (`WHERE fetched_at > $1 ORDER BY fetched_at ASC`, no LIMIT) so consecutive
// article-sync runs can be observed the way the hourly JobRunner drives them.
type stubSyncArticleRepo struct {
	SyncArticleRepository
	available     []*domain.Article
	sinceValues   []time.Time
	upsertBatches [][]*domain.Article
	upsertErr     error
}

func (s *stubSyncArticleRepo) FetchInoreaderArticles(_ context.Context, since time.Time) ([]*domain.Article, error) {
	s.sinceValues = append(s.sinceValues, since)

	var fetched []*domain.Article
	for _, article := range s.available {
		if article.CreatedAt.After(since) {
			fetched = append(fetched, article)
		}
	}
	sort.Slice(fetched, func(i, j int) bool { return fetched[i].CreatedAt.Before(fetched[j].CreatedAt) })

	return fetched, nil
}

// UpsertArticlesReportingSkipped accepts every article it is given — these
// tests cover the watermark against validation drops, not against the write
// path refusing rows (see article_sync_refused_rows_test.go for that).
func (s *stubSyncArticleRepo) UpsertArticlesReportingSkipped(_ context.Context, articles []*domain.Article) ([]*domain.Article, error) {
	if s.upsertErr != nil {
		return nil, s.upsertErr
	}

	s.upsertBatches = append(s.upsertBatches, articles)

	return nil, nil
}

func (s *stubSyncArticleRepo) upsertedCount() int {
	var total int
	for _, batch := range s.upsertBatches {
		total += len(batch)
	}

	return total
}

func TestSyncArticles_DoesNotResyncAlreadySyncedArticles(t *testing.T) {
	t.Run("should upsert each article once across consecutive hourly runs", func(t *testing.T) {
		now := time.Now()
		articleRepo := &stubSyncArticleRepo{
			available: []*domain.Article{
				{
					ID:        "inoreader-1",
					URL:       "https://example.com/article1",
					Title:     "Article 1",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedURL:   "https://example.com/feed",
					CreatedAt: now.Add(-2 * time.Hour),
				},
				{
					ID:        "inoreader-2",
					URL:       "https://example.com/article2",
					Title:     "Article 2",
					Content:   "<p>Another valid article with sufficient content length.</p>",
					FeedURL:   "https://example.com/feed",
					CreatedAt: now.Add(-30 * time.Minute),
				},
			},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		assert.NoError(t, svc.SyncArticles(context.Background()))
		assert.NoError(t, svc.SyncArticles(context.Background()))
		assert.NoError(t, svc.SyncArticles(context.Background()))

		assert.Equal(t, 2, articleRepo.upsertedCount(), "already-synced articles must not be upserted again")
		assert.Len(t, articleRepo.sinceValues, 3)
		assert.Equal(t, now.Add(-30*time.Minute), articleRepo.sinceValues[1], "second run must start strictly above the newest synced fetched_at")
		assert.Equal(t, now.Add(-30*time.Minute), articleRepo.sinceValues[2])
	})
}

func TestSyncArticles_PicksUpArticlesArrivingAfterTheWatermark(t *testing.T) {
	t.Run("should sync articles written after the previous run", func(t *testing.T) {
		now := time.Now()
		articleRepo := &stubSyncArticleRepo{
			available: []*domain.Article{
				{
					ID:        "inoreader-1",
					URL:       "https://example.com/article1",
					Title:     "Article 1",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedURL:   "https://example.com/feed",
					CreatedAt: now.Add(-2 * time.Hour),
				},
			},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		assert.NoError(t, svc.SyncArticles(context.Background()))

		articleRepo.available = append(articleRepo.available, &domain.Article{
			ID:        "inoreader-2",
			URL:       "https://example.com/article2",
			Title:     "Article 2",
			Content:   "<p>Freshly harvested content that must still be synced.</p>",
			FeedURL:   "https://example.com/feed",
			CreatedAt: now.Add(-1 * time.Minute),
		})

		assert.NoError(t, svc.SyncArticles(context.Background()))

		assert.Equal(t, 2, articleRepo.upsertedCount())
		assert.Len(t, articleRepo.upsertBatches, 2)
		assert.Equal(t, "https://example.com/article2", articleRepo.upsertBatches[1][0].URL)
	})
}

func TestSyncArticles_FirstRunLooksBack24Hours(t *testing.T) {
	t.Run("should rescan the last 24 hours on the first run of the process", func(t *testing.T) {
		articleRepo := &stubSyncArticleRepo{}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		assert.NoError(t, svc.SyncArticles(context.Background()))

		assert.Len(t, articleRepo.sinceValues, 1)
		assert.WithinDuration(t, time.Now().Add(-24*time.Hour), articleRepo.sinceValues[0], time.Minute)
	})
}

func TestSyncArticles_WatermarkPassesSkippedArticles(t *testing.T) {
	t.Run("should not rescan articles the validation dropped", func(t *testing.T) {
		now := time.Now()
		articleRepo := &stubSyncArticleRepo{
			available: []*domain.Article{
				{
					ID:        "inoreader-1",
					URL:       "https://example.com/article1",
					Title:     "Article 1",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedURL:   "https://example.com/feed",
					CreatedAt: now.Add(-2 * time.Hour),
				},
				{
					ID:        "inoreader-2",
					URL:       "https://example.com/article2",
					Title:     "Article 2",
					Content:   "<script>alert('xss')</script>", // empty after sanitization
					FeedURL:   "https://example.com/feed",
					CreatedAt: now.Add(-1 * time.Hour),
				},
			},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		assert.NoError(t, svc.SyncArticles(context.Background()))
		assert.NoError(t, svc.SyncArticles(context.Background()))

		assert.Len(t, articleRepo.sinceValues, 2)
		assert.Equal(t, now.Add(-1*time.Hour), articleRepo.sinceValues[1], "the watermark must cover every scanned row, not only the upserted ones")
	})
}

func TestSyncArticles_KeepsWatermarkWhenUpsertFails(t *testing.T) {
	t.Run("should re-fetch the batch that was never persisted", func(t *testing.T) {
		now := time.Now()
		articleRepo := &stubSyncArticleRepo{
			available: []*domain.Article{
				{
					ID:        "inoreader-1",
					URL:       "https://example.com/article1",
					Title:     "Article 1",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedURL:   "https://example.com/feed",
					CreatedAt: now.Add(-2 * time.Hour),
				},
			},
			upsertErr: fmt.Errorf("datahub unavailable"),
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		assert.Error(t, svc.SyncArticles(context.Background()))
		assert.Error(t, svc.SyncArticles(context.Background()))

		assert.Len(t, articleRepo.sinceValues, 2)
		assert.True(t, articleRepo.sinceValues[1].Before(now.Add(-2*time.Hour)), "a failed batch must stay inside the next run's window")
	})
}
