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
//
// hybridSearcher, when non-nil and hybridEnabled, replaces both the BM25 arm
// (bm25Searcher) and the plain vector arm for the *original* query with a
// single in-database hybrid (vector + tsvector RRF) call — the fusion that
// would otherwise happen in FuseResults' fuseHybridResults is already done
// inside the SQL query. It only applies to the unscoped (no candidate
// article IDs) case: HybridSearcher has no SearchWithinArticles-equivalent,
// so candidate-scoped retrieval (e.g. Morning Letter) falls back to the
// bm25Searcher/chunkRepo path below.
func EmbedAndSearch(
	ctx context.Context,
	sc *StageContext,
	encoder domain.VectorEncoder,
	bm25Searcher domain.BM25Searcher,
	hybridSearcher domain.HybridSearcher,
	chunkRepo domain.RagChunkRepository,
	hybridEnabled bool,
	bm25Limit int,
	logger *slog.Logger,
) error {
	// Build the full list of additional queries that need embedding
	sc.AdditionalQueries = buildAdditionalQueries(sc.ExpandedQueries, sc.TagQueries)

	hasCandidateArticles := len(sc.CandidateArticleIDs) > 0
	useHybridSearcher := hybridEnabled && hybridSearcher != nil && !hasCandidateArticles

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

	// goroutine E: BM25 Search (original + expanded queries for cross-language matching)
	// Skipped when useHybridSearcher: the in-DB hybrid search below already
	// fuses lexical + vector signals, so a separate BM25 arm would be redundant.
	if !useHybridSearcher && hybridEnabled && bm25Searcher != nil {
		g.Go(func() error {
			bm25Start := time.Now()

			// Build deduplicated query list: original + expanded (for cross-language BM25)
			queries := bm25Queries(sc.Query, sc.AdditionalQueries)

			var allResults []domain.BM25SearchResult
			seen := make(map[string]struct{})
			for _, q := range queries {
				results, err := bm25Searcher.SearchBM25(gctx, q, bm25Limit)
				if err != nil {
					logger.Warn("hybrid_bm25_search_failed",
						slog.String("retrieval_id", sc.RetrievalID),
						slog.String("query_preview", queryLogPreview(q)),
						slog.String("error", err.Error()))
					continue // non-fatal per query
				}
				for _, r := range results {
					if _, exists := seen[r.ArticleID]; !exists {
						seen[r.ArticleID] = struct{}{}
						allResults = append(allResults, r)
					}
				}
			}
			sc.BM25Results = allResults

			bm25Duration := time.Since(bm25Start)
			logger.Info("hybrid_bm25_search_completed",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.Int("bm25_queries", len(queries)),
				slog.Int("bm25_hits", len(allResults)),
				slog.Int64("duration_ms", bm25Duration.Milliseconds()))
			return nil
		})
	}

	// goroutine F: Original Vector Search (skipped when embedding unavailable)
	if sc.OriginalEmbedding != nil {
		g.Go(func() error {
			var results []domain.SearchResult
			var err error
			switch {
			case useHybridSearcher:
				hybridStart := time.Now()
				results, err = hybridSearcher.HybridSearch(gctx, sc.OriginalEmbedding, sc.Query, sc.SearchLimit)
				if err == nil {
					logger.Info("hybrid_db_search_completed",
						slog.String("retrieval_id", sc.RetrievalID),
						slog.Int("hits", len(results)),
						slog.Int64("duration_ms", time.Since(hybridStart).Milliseconds()))
				}
			case hasCandidateArticles:
				results, err = chunkRepo.SearchWithinArticles(gctx, sc.OriginalEmbedding, sc.CandidateArticleIDs, sc.SearchLimit)
			default:
				results, err = chunkRepo.Search(gctx, sc.OriginalEmbedding, sc.SearchLimit)
			}
			if err != nil {
				return fmt.Errorf("failed to search original query: %w", err)
			}
			sc.OriginalResults = results
			return nil
		})
	} else {
		logger.Warn("vector_search_skipped",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.String("reason", "original_embedding_unavailable"),
			slog.String("degraded_mode", "bm25_only"))
	}

	return g.Wait()
}

// bm25Queries builds a deduplicated list of queries for BM25 search.
// Includes the original query plus expanded/translated queries for cross-language matching.
func bm25Queries(original string, additionalQueries []string) []string {
	queries := make([]string, 0, 1+len(additionalQueries))
	queries = append(queries, original)
	seen := map[string]struct{}{original: {}}
	for _, q := range additionalQueries {
		if _, exists := seen[q]; !exists {
			seen[q] = struct{}{}
			queries = append(queries, q)
		}
	}
	return queries
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
