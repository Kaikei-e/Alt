package bootstrap

import (
	"context"
	"time"

	"search-indexer/domain"
	"search-indexer/logger"
)

// warmupTimeout caps how long the warmup probe is allowed to block. Set to
// 30s because Ollama's first qwen3-embedding load on cold GPU can take ~10s
// in the worst case; anything past 30s indicates news-creator-backend is down
// and the probe should give up so it never holds back service start.
const warmupTimeout = 30 * time.Second

// warmupProbeQuery is intentionally a non-word so it matches nothing useful in
// the index. The point is to force Meilisearch to invoke the embedder so the
// qwen3 model becomes GPU-resident; the search result is discarded.
const warmupProbeQuery = "warmup-probe-aaa"

// warmupSearcher narrows the SearchEngine surface to the single method
// warmup needs. It exists to keep the warmup unit test independent of the
// full port.SearchEngine interface (most methods are irrelevant here).
type warmupSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error)
}

// warmupSearchEngine issues a single probe Search so Meilisearch's hybrid
// pipeline pulls the qwen3 embedding model into Ollama's resident set. Without
// this, the first user-facing search after process start pays the embedder
// cold-start tax (~1.1s in production observations).
//
// Failure modes (DNS, embedder down, ctx cancel) degrade to a Warn log; the
// caller's service start must never block on this.
func warmupSearchEngine(ctx context.Context, eng warmupSearcher) {
	wctx, cancel := context.WithTimeout(ctx, warmupTimeout)
	defer cancel()

	start := time.Now()
	if _, err := eng.Search(wctx, warmupProbeQuery, 1); err != nil {
		logger.Logger.WarnContext(ctx, "search engine warmup probe failed",
			"err", err,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
		return
	}
	logger.Logger.InfoContext(ctx, "search engine warmup probe ok",
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
}

// runWarmupLoop re-probes the search engine on an interval instead of once
// at startup. A single startup-only probe (the pre-2026-07-22 design) was
// not enough: production observation showed gemma4 (chat/RAG) and
// qwen3-embedding (hybrid search) exclusively swap GPU residency on this
// host's single GPU, so the embedding model goes cold again within minutes
// of the last chat request regardless of OLLAMA_KEEP_ALIVE. Re-probing on
// the same cadence as the LRU cache TTL means a query is either a cheap
// cache hit or the embedder is already warm.
func runWarmupLoop(ctx context.Context, eng warmupSearcher, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		warmupSearchEngine(ctx, eng)

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
