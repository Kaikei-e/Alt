package feeds

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"alt/config"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/utils/logger"
)

func init() {
	logger.InitLogger()
}

// countingFeedAmountPort counts how many times Execute was called.
type countingFeedAmountPort struct {
	calls int64
}

func (c *countingFeedAmountPort) Execute(ctx context.Context) (int, error) {
	atomic.AddInt64(&c.calls, 1)
	return 42, nil
}

type countingTotalArticlesCountPort struct {
	calls int64
}

func (c *countingTotalArticlesCountPort) Execute(ctx context.Context) (int, error) {
	atomic.AddInt64(&c.calls, 1)
	return 100, nil
}

type countingUnsummarizedArticlesCountPort struct {
	calls int64
}

func (c *countingUnsummarizedArticlesCountPort) Execute(ctx context.Context) (int, error) {
	atomic.AddInt64(&c.calls, 1)
	return 5, nil
}

func resetFeedStatsCache(t *testing.T) {
	t.Helper()
	feedStatsCacheMu.Lock()
	feedStatsCache = cachedFeedStats{}
	feedStatsCacheMu.Unlock()
}

// TestFetchStatsCached_CacheHitWithinTTL verifies that consecutive calls within
// statsCacheTTL only invoke the underlying usecases once. With many concurrent
// streams calling fetchStatsCached every 5s, this is the dominant load reduction.
func TestFetchStatsCached_CacheHitWithinTTL(t *testing.T) {
	resetFeedStatsCache(t)

	feedAmount := &countingFeedAmountPort{}
	total := &countingTotalArticlesCountPort{}
	unsum := &countingUnsummarizedArticlesCountPort{}

	h := &Handler{
		deps: FeedHandlerDeps{
			FeedAmount:        fetch_feed_stats_usecase.NewFeedsCountUsecase(feedAmount),
			TotalCount:        fetch_feed_stats_usecase.NewTotalArticlesCountUsecase(total),
			UnsummarizedCount: fetch_feed_stats_usecase.NewUnsummarizedArticlesCountUsecase(unsum),
		},
		cfg:    &config.Config{},
		logger: slog.Default(),
	}

	for i := range 10 {
		stats, err := h.fetchStatsCached(context.Background())
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if stats.feedCount != 42 || stats.totalArticles != 100 || stats.unsummarizedCount != 5 {
			t.Fatalf("call %d: unexpected stats %+v", i, stats)
		}
	}

	if got := atomic.LoadInt64(&feedAmount.calls); got != 1 {
		t.Errorf("FeedAmount: expected 1 underlying call, got %d", got)
	}
	if got := atomic.LoadInt64(&total.calls); got != 1 {
		t.Errorf("TotalCount: expected 1 underlying call, got %d", got)
	}
	if got := atomic.LoadInt64(&unsum.calls); got != 1 {
		t.Errorf("UnsummarizedCount: expected 1 underlying call, got %d", got)
	}
}

// TestFetchStatsCached_RefetchAfterTTL exercises the cache miss after expiry by
// rewinding the cached fetchedAt timestamp.
func TestFetchStatsCached_RefetchAfterTTL(t *testing.T) {
	resetFeedStatsCache(t)

	feedAmount := &countingFeedAmountPort{}
	total := &countingTotalArticlesCountPort{}
	unsum := &countingUnsummarizedArticlesCountPort{}

	h := &Handler{
		deps: FeedHandlerDeps{
			FeedAmount:        fetch_feed_stats_usecase.NewFeedsCountUsecase(feedAmount),
			TotalCount:        fetch_feed_stats_usecase.NewTotalArticlesCountUsecase(total),
			UnsummarizedCount: fetch_feed_stats_usecase.NewUnsummarizedArticlesCountUsecase(unsum),
		},
		cfg:    &config.Config{},
		logger: slog.Default(),
	}

	if _, err := h.fetchStatsCached(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Simulate cache expiry by rewinding fetchedAt past the TTL.
	feedStatsCacheMu.Lock()
	feedStatsCache.fetchedAt = time.Now().Add(-2 * statsCacheTTL)
	feedStatsCacheMu.Unlock()

	if _, err := h.fetchStatsCached(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt64(&feedAmount.calls); got != 2 {
		t.Errorf("FeedAmount: expected 2 underlying calls (fresh + refetch), got %d", got)
	}
}
