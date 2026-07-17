package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// FuseResults runs parallel vector search for expanded queries and applies RRF fusion (Stage 3).
func FuseResults(
	ctx context.Context,
	sc *StageContext,
	chunkRepo domain.RagChunkRepository,
	logger *slog.Logger,
) error {
	// Degraded mode: no embeddings available, promote BM25 results directly
	if sc.OriginalEmbedding == nil && len(sc.AdditionalEmbeddings) == 0 {
		logger.Info("fuse_results_degraded_mode",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.Int("bm25_results", len(sc.BM25Results)),
			slog.String("degraded_mode", "bm25_only"))
		sc.HitsOriginal = promoteBM25ToSearchResults(sc.BM25Results)
		sc.HitsExpanded = nil
		return nil
	}

	// Build all embeddings list: [original, ...additional]
	allEmbeddings := make([][]float32, 0, 1+len(sc.AdditionalEmbeddings))
	allEmbeddings = append(allEmbeddings, sc.OriginalEmbedding)
	allEmbeddings = append(allEmbeddings, sc.AdditionalEmbeddings...)

	allQueries := make([]string, 0, 1+len(sc.AdditionalQueries))
	allQueries = append(allQueries, sc.Query)
	allQueries = append(allQueries, sc.AdditionalQueries...)

	logger.Info("queries_encoded",
		slog.String("retrieval_id", sc.RetrievalID),
		slog.Int("query_count", len(allQueries)),
		slog.Any("queries", allQueries))

	hasCandidateArticles := len(sc.CandidateArticleIDs) > 0

	// Parallel vector search for expanded query embeddings (skip index 0, already done)
	searchStart := time.Now()
	allResults := make([][]domain.SearchResult, len(allEmbeddings))
	// Index 0 = original (already searched), reuse results
	allResults[0] = sc.OriginalResults

	g, gctx := errgroup.WithContext(ctx)
	for i := 1; i < len(allEmbeddings); i++ {
		idx, qv := i, allEmbeddings[i]
		g.Go(func() error {
			var results []domain.SearchResult
			var err error
			if hasCandidateArticles {
				results, err = chunkRepo.SearchWithinArticles(gctx, qv, sc.CandidateArticleIDs, sc.SearchLimit)
			} else {
				results, err = chunkRepo.Search(gctx, qv, sc.SearchLimit)
			}
			if err != nil {
				return err
			}
			allResults[idx] = results
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to search chunks: %w", err)
	}

	searchDuration := time.Since(searchStart)
	logger.Info("parallel_vector_search_completed",
		slog.String("retrieval_id", sc.RetrievalID),
		slog.Int("query_count", len(allEmbeddings)),
		slog.Int64("duration_ms", searchDuration.Milliseconds()))

	// Apply BM25 RRF fusion to original query results (index 0)
	if len(sc.BM25Results) > 0 && len(allResults) > 0 {
		allResults[0] = fuseHybridResults(allResults[0], sc.BM25Results, sc.RRFK, sc.RetrievalID, logger)
	}

	// Process collected results
	rrfK := sc.RRFK
	type chunkData struct {
		Item     ContextItem
		RRFScore float64
	}
	chunksMapExpanded := make(map[uuid.UUID]*chunkData)

	for i, results := range allResults {
		if i == 0 {
			sc.HitsOriginal = results
		} else {
			for rank, res := range results {
				if _, exists := chunksMapExpanded[res.Chunk.ID]; !exists {
					chunksMapExpanded[res.Chunk.ID] = &chunkData{
						Item: ContextItem{
							ChunkText:       res.Chunk.Content,
							URL:             res.URL,
							Title:           res.Title,
							PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339),
							DocumentVersion: res.DocumentVersion,
							ChunkID:         res.Chunk.ID,
							Score:           res.Score,
							ArticleID:       res.ArticleID,
						},
						RRFScore: 0,
					}
				}
				chunksMapExpanded[res.Chunk.ID].RRFScore += 1.0 / (rrfK + float64(rank+1))
			}
		}
	}

	// Prepare Expanded list sorted by RRF
	hitsExpanded := make([]ContextItem, 0, len(chunksMapExpanded))
	for _, data := range chunksMapExpanded {
		hitsExpanded = append(hitsExpanded, data.Item)
	}
	sort.Slice(hitsExpanded, func(i, j int) bool {
		return chunksMapExpanded[hitsExpanded[i].ChunkID].RRFScore > chunksMapExpanded[hitsExpanded[j].ChunkID].RRFScore
	})

	sc.HitsExpanded = hitsExpanded

	// Log top expanded hits for debugging
	debugLimit := 5
	if len(hitsExpanded) < debugLimit {
		debugLimit = len(hitsExpanded)
	}
	if debugLimit > 0 {
		var debugLog []map[string]interface{}
		for i := 0; i < debugLimit; i++ {
			debugLog = append(debugLog, map[string]interface{}{
				"title": hitsExpanded[i].Title,
				"url":   hitsExpanded[i].URL,
				"score": hitsExpanded[i].Score,
				"rrf":   chunksMapExpanded[hitsExpanded[i].ChunkID].RRFScore,
			})
		}
		logger.Info("expanded_query_hits_debug",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.Any("top_hits", debugLog))
	} else {
		logger.Info("expanded_query_hits_debug",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.String("msg", "no hits for expanded queries"))
	}

	return nil
}

// fuseHybridResults merges vector search results with BM25 results using RRF.
func fuseHybridResults(
	vectorResults []domain.SearchResult,
	bm25Results []domain.BM25SearchResult,
	rrfK float64,
	retrievalID string,
	logger *slog.Logger,
) []domain.SearchResult {
	type fusedResult struct {
		vectorResult *domain.SearchResult
		rrfScore     float64
	}
	fusedMap := make(map[string]*fusedResult)

	for i, vr := range vectorResults {
		articleID := vr.ArticleID
		if _, exists := fusedMap[articleID]; !exists {
			vrCopy := vr
			fusedMap[articleID] = &fusedResult{
				vectorResult: &vrCopy,
				rrfScore:     0,
			}
		}
		fusedMap[articleID].rrfScore += 1.0 / (rrfK + float64(i+1))
	}

	for _, br := range bm25Results {
		articleID := br.ArticleID
		if existing, exists := fusedMap[articleID]; exists {
			existing.rrfScore += 1.0 / (rrfK + float64(br.Rank))
		} else {
			// BM25-only hit (no vector match): resolve it into a SearchResult
			// from the BM25 payload itself instead of dropping the contribution.
			bm25AsResult := bm25ResultToSearchResult(br)
			fusedMap[articleID] = &fusedResult{
				vectorResult: &bm25AsResult,
				rrfScore:     1.0 / (rrfK + float64(br.Rank)),
			}
		}
	}

	results := make([]domain.SearchResult, 0, len(fusedMap))
	for _, fr := range fusedMap {
		result := *fr.vectorResult
		result.Score = float32(fr.rrfScore)
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	logger.Info("hybrid_rrf_fusion_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("vector_count", len(vectorResults)),
		slog.Int("bm25_count", len(bm25Results)),
		slog.Int("fused_count", len(results)))

	return results
}

// promoteBM25ToSearchResults converts BM25 results to domain.SearchResult format
// for use in degraded mode (embedder unavailable). BM25 results provide article-level
// data which is sufficient for answer generation even without vector-based chunk retrieval.
func promoteBM25ToSearchResults(bm25Results []domain.BM25SearchResult) []domain.SearchResult {
	if len(bm25Results) == 0 {
		return nil
	}
	results := make([]domain.SearchResult, len(bm25Results))
	for i, br := range bm25Results {
		results[i] = bm25ResultToSearchResult(br)
	}
	return results
}

// bm25ResultToSearchResult converts a single BM25 hit into a domain.SearchResult.
// ChunkID is left as uuid.Nil when the BM25 payload doesn't carry one — callers
// must treat uuid.Nil as "no chunk id" rather than a real identifier; fabricating
// a random UUID here would let downstream citations reference a chunk that
// never existed.
func bm25ResultToSearchResult(br domain.BM25SearchResult) domain.SearchResult {
	var chunkID uuid.UUID
	if br.ChunkID != "" {
		if parsed, err := uuid.Parse(br.ChunkID); err == nil {
			chunkID = parsed
		}
	}
	return domain.SearchResult{
		Chunk: domain.RagChunk{
			ID:      chunkID,
			Content: br.Content,
		},
		Score:     br.Score,
		ArticleID: br.ArticleID,
		Title:     br.Title,
		URL:       br.URL,
	}
}
