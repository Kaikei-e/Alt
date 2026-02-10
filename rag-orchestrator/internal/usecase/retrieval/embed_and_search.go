package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"rag-orchestrator/internal/domain"

	"golang.org/x/sync/errgroup"
)

// EmbedAndSearch runs BM25 search, original vector search, and expanded embedding in parallel (Stage 2).
func EmbedAndSearch(
	ctx context.Context,
	sc *StageContext,
	encoder domain.VectorEncoder,
	bm25Searcher domain.BM25Searcher,
	chunkRepo domain.RagChunkRepository,
	hybridEnabled bool,
	bm25Limit int,
	logger *slog.Logger,
) error {
	// Build the full list of additional queries that need embedding
	sc.AdditionalQueries = buildAdditionalQueries(sc.ExpandedQueries, sc.TagQueries)

	hasCandidateArticles := len(sc.CandidateArticleIDs) > 0

	g, gctx := errgroup.WithContext(ctx)

	// goroutine D: Embed expanded + tag queries
	if len(sc.AdditionalQueries) > 0 {
		g.Go(func() error {
			embs, err := encoder.Encode(gctx, sc.AdditionalQueries)
			if err != nil {
				logger.Warn("expanded_embedding_failed",
					slog.String("retrieval_id", sc.RetrievalID),
					slog.String("error", err.Error()))
				return nil // non-fatal
			}
			sc.AdditionalEmbeddings = embs
			return nil
		})
	}

	// goroutine E: BM25 Search
	if hybridEnabled && bm25Searcher != nil {
		g.Go(func() error {
			bm25Start := time.Now()
			results, err := bm25Searcher.SearchBM25(gctx, sc.Query, bm25Limit)
			bm25Duration := time.Since(bm25Start)
			if err != nil {
				logger.Warn("hybrid_bm25_search_failed",
					slog.String("retrieval_id", sc.RetrievalID),
					slog.String("error", err.Error()),
					slog.Int64("duration_ms", bm25Duration.Milliseconds()))
				return nil // non-fatal
			}
			sc.BM25Results = results
			logger.Info("hybrid_bm25_search_completed",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.Int("bm25_hits", len(results)),
				slog.Int64("duration_ms", bm25Duration.Milliseconds()))
			return nil
		})
	}

	// goroutine F: Original Vector Search
	g.Go(func() error {
		var results []domain.SearchResult
		var err error
		if hasCandidateArticles {
			results, err = chunkRepo.SearchWithinArticles(gctx, sc.OriginalEmbedding, sc.CandidateArticleIDs, sc.SearchLimit)
		} else {
			results, err = chunkRepo.Search(gctx, sc.OriginalEmbedding, sc.SearchLimit)
		}
		if err != nil {
			return fmt.Errorf("failed to search original query: %w", err)
		}
		sc.OriginalResults = results
		return nil
	})

	return g.Wait()
}

func buildAdditionalQueries(expandedQueries, tagQueries []string) []string {
	additional := make([]string, 0, len(expandedQueries)+len(tagQueries))
	additional = append(additional, expandedQueries...)
	for _, tq := range tagQueries {
		exists := false
		for _, eq := range expandedQueries {
			if eq == tq {
				exists = true
				break
			}
		}
		if !exists {
			additional = append(additional, tq)
		}
	}
	return additional
}
