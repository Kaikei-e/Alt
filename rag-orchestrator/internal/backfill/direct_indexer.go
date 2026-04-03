package backfill

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"rag-orchestrator/internal/usecase"
)

// DirectIndexer bypasses HTTP and indexes articles directly using
// IndexArticleUsecase instances. Supports multiple embedder replicas
// with round-robin distribution for parallel GPU utilization.
type DirectIndexer struct {
	indexers    []usecase.IndexArticleUsecase
	next        atomic.Uint64
	concurrency int
	logger      *slog.Logger
}

// NewDirectIndexer creates a direct indexer with a single usecase.
func NewDirectIndexer(
	indexUsecase usecase.IndexArticleUsecase,
	concurrency int,
	logger *slog.Logger,
) *DirectIndexer {
	return &DirectIndexer{
		indexers:    []usecase.IndexArticleUsecase{indexUsecase},
		concurrency: concurrency,
		logger:      logger,
	}
}

// NewDirectIndexerMulti creates a direct indexer with multiple usecases
// (one per embedder replica) for round-robin distribution.
func NewDirectIndexerMulti(
	indexers []usecase.IndexArticleUsecase,
	concurrency int,
	logger *slog.Logger,
) *DirectIndexer {
	return &DirectIndexer{
		indexers:    indexers,
		concurrency: concurrency,
		logger:      logger,
	}
}

// IndexArticle indexes a single article, distributing across replicas.
func (d *DirectIndexer) IndexArticle(ctx context.Context, a Article) error {
	idx := d.next.Add(1) % uint64(len(d.indexers))
	return d.indexers[idx].Upsert(ctx, a.ID, a.Title, a.URL, a.Body)
}

// IndexBatch indexes a batch of articles concurrently.
func (d *DirectIndexer) IndexBatch(ctx context.Context, articles []Article) (processed, failed int64) {
	sem := make(chan struct{}, d.concurrency)
	var wg sync.WaitGroup
	var procCount, failCount int64

	for _, a := range articles {
		select {
		case <-ctx.Done():
			return atomic.LoadInt64(&procCount), atomic.LoadInt64(&failCount)
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(article Article) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := d.IndexArticle(ctx, article); err != nil {
				d.logger.Warn("direct_index_failed",
					slog.String("id", article.ID),
					slog.String("error", err.Error()),
				)
				atomic.AddInt64(&failCount, 1)
			} else {
				atomic.AddInt64(&procCount, 1)
			}
		}(a)
	}

	wg.Wait()
	return atomic.LoadInt64(&procCount), atomic.LoadInt64(&failCount)
}
