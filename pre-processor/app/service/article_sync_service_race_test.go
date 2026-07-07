package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"
)

// raceArticleRepo implements the subset of repository.ArticleRepository used
// by both SyncArticles and BackfillEmptyFeeds so the two code paths can be
// driven concurrently in the same test.
type raceArticleRepo struct {
	repository.ArticleRepository
}

func (r *raceArticleRepo) FetchInoreaderArticles(_ context.Context, _ time.Time) ([]*domain.Article, error) {
	return []*domain.Article{
		{ID: "sync-1", URL: "https://example.com/sync-1", Title: "sync title", Content: "sync content", FeedURL: "https://example.com/feed"},
	}, nil
}

func (r *raceArticleRepo) UpsertArticles(_ context.Context, _ []*domain.Article) error {
	return nil
}

func (r *raceArticleRepo) FetchInoreaderArticlesForEmptyFeeds(_ context.Context, _ time.Time, _ int) ([]*domain.Article, error) {
	return []*domain.Article{
		{ID: "backfill-1", URL: "https://example.com/backfill-1", Title: "backfill title", Content: "backfill content", FeedID: "feed-1", CreatedAt: time.Now()},
	}, nil
}

func (r *raceArticleRepo) UpsertArticlesWithFeedID(_ context.Context, _ []*domain.Article) error {
	return nil
}

// TestArticleSyncService_ConcurrentSyncAndBackfill_NoRace reproduces the HIGH
// finding: userID and lastBackfillFetchedAt are read/written by SyncArticles
// (article-sync JobRunner) and BackfillEmptyFeeds (article-backfill
// JobRunner) without synchronization, even though both goroutines share the
// same *articleSyncService instance in production. Run with -race.
func TestArticleSyncService_ConcurrentSyncAndBackfill_NoRace(t *testing.T) {
	articleRepo := &raceArticleRepo{}
	externalAPI := &stubBackfillExternalAPI{userID: "system-user-id"}

	svc := NewArticleSyncService(articleRepo, externalAPI, testLoggerBackfill())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = svc.SyncArticles(context.Background())
		}()
		go func() {
			defer wg.Done()
			_ = svc.BackfillEmptyFeeds(context.Background())
		}()
	}
	wg.Wait()
}
