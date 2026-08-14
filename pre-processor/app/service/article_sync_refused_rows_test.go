package service

import (
	"context"
	"sort"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"

	"github.com/stretchr/testify/assert"
)

// stubRefusingArticleRepo mirrors the backend_api write path: an article whose
// feed alt-db does not know yet is refused and UpsertArticles still returns
// nil. The refusal is external to the row — inoreader_articles.fetched_at is
// never bumped when the feed is registered hours later — so a watermark that
// stepped over the row would drop the article forever.
type stubRefusingArticleRepo struct {
	repository.ArticleRepository
	available   []*domain.Article
	knownFeeds  map[string]bool
	sinceValues []time.Time
	written     []string
}

func (s *stubRefusingArticleRepo) FetchInoreaderArticles(_ context.Context, since time.Time) ([]*domain.Article, error) {
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

func (s *stubRefusingArticleRepo) UpsertArticlesReportingSkipped(_ context.Context, articles []*domain.Article) ([]*domain.Article, error) {
	var skipped []*domain.Article
	for _, article := range articles {
		if !s.knownFeeds[article.FeedURL] {
			skipped = append(skipped, article)
			continue
		}
		s.written = append(s.written, article.URL)
	}

	return skipped, nil
}

func (s *stubRefusingArticleRepo) UpsertArticles(ctx context.Context, articles []*domain.Article) error {
	_, err := s.UpsertArticlesReportingSkipped(ctx, articles)
	return err
}

func TestSyncArticles_RedeliversArticlesTheWritePathRefused(t *testing.T) {
	t.Run("should sync an article whose feed alt-db only learned about after the first run", func(t *testing.T) {
		now := time.Now()
		articleRepo := &stubRefusingArticleRepo{
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
			knownFeeds: map[string]bool{},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		// Run 1: the feed is not registered in alt-db yet, so the write path
		// refuses the article while reporting success to the caller.
		assert.NoError(t, svc.SyncArticles(context.Background()))
		assert.Empty(t, articleRepo.written)

		// The feed gets registered between the two hourly runs.
		articleRepo.knownFeeds["https://example.com/feed"] = true

		assert.NoError(t, svc.SyncArticles(context.Background()))

		assert.Equal(t, []string{"https://example.com/article1"}, articleRepo.written,
			"an article the write path refused must stay inside the next run's window")
		assert.Len(t, articleRepo.sinceValues, 2)
		assert.True(t, articleRepo.sinceValues[1].Before(now.Add(-2*time.Hour)),
			"the watermark must not advance past a row the write path refused")
	})
}

func TestSyncArticles_WatermarkStopsBelowTheOldestRefusedRow(t *testing.T) {
	t.Run("should keep re-offering only from the oldest refusal onwards", func(t *testing.T) {
		now := time.Now()
		articleRepo := &stubRefusingArticleRepo{
			available: []*domain.Article{
				{
					ID:        "inoreader-1",
					URL:       "https://example.com/known1",
					Title:     "Known 1",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedURL:   "https://example.com/known-feed",
					CreatedAt: now.Add(-3 * time.Hour),
				},
				{
					ID:        "inoreader-2",
					URL:       "https://example.com/unknown",
					Title:     "Unknown feed",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedURL:   "https://example.com/unregistered-feed",
					CreatedAt: now.Add(-2 * time.Hour),
				},
				{
					ID:        "inoreader-3",
					URL:       "https://example.com/known2",
					Title:     "Known 2",
					Content:   "<p>Valid content long enough to survive sanitization.</p>",
					FeedURL:   "https://example.com/known-feed",
					CreatedAt: now.Add(-1 * time.Hour),
				},
			},
			knownFeeds: map[string]bool{"https://example.com/known-feed": true},
		}
		externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

		svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

		assert.NoError(t, svc.SyncArticles(context.Background()))

		assert.Len(t, articleRepo.sinceValues, 1)

		articleRepo.knownFeeds["https://example.com/unregistered-feed"] = true
		assert.NoError(t, svc.SyncArticles(context.Background()))

		assert.Equal(t, now.Add(-3*time.Hour), articleRepo.sinceValues[1],
			"the watermark must stop below the oldest refused row, not at the newest scanned one")
		assert.Equal(t, []string{
			"https://example.com/known1",
			"https://example.com/known2",
			"https://example.com/unknown",
			"https://example.com/known2",
		}, articleRepo.written, "run 2 must re-offer the refused row and everything scanned after it")
	})
}

func TestHoldWatermarkBelow_AbandonsARowRefusedFarBehindTheScanFront(t *testing.T) {
	// A feed subscribed in Inoreader but never registered in alt-db is a stable
	// state, not a transient one: nothing in pre-processor calls
	// RegisterFeedLink. Holding the cursor below such a row forever pins the
	// scan window open, so every already-synced article inside it is re-upserted
	// on every hourly run — the amplification this watermark exists to stop.
	// Past maxRefusalHold behind the scan front the row is abandoned loudly
	// instead, which is what the fixed 24h window did anyway, only silently.
	svc := &articleSyncService{logger: testLoggerBackfill()}
	base := time.Now()

	scanned := []*domain.Article{
		{URL: "https://example.com/older-accepted", CreatedAt: base.Add(-3 * time.Hour)},
		{URL: "https://example.com/stuck", FeedURL: "https://example.com/never", CreatedAt: base.Add(-2 * time.Hour)},
	}
	refused := []*domain.Article{scanned[1]}

	t.Run("holds while the refusal is still within the hold window", func(t *testing.T) {
		scannedThrough := base
		held := svc.holdWatermarkBelow(context.Background(), scanned, refused, scannedThrough)

		assert.Equal(t, base.Add(-3*time.Hour), held,
			"a recently refused row must still pin the watermark below itself")
	})

	t.Run("abandons the refusal once the scan front has moved past the hold window", func(t *testing.T) {
		scannedThrough := base.Add(maxRefusalHold + time.Hour)
		held := svc.holdWatermarkBelow(context.Background(), scanned, refused, scannedThrough)

		assert.Equal(t, scannedThrough, held,
			"a row refused for longer than the hold window must not pin the cursor forever")
	})
}
