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
