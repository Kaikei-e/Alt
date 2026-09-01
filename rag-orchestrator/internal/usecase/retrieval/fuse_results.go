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

// FuseResults runs parallel search for the expanded queries and applies RRF fusion (Stage 3).
//
// hybridSearcher, when non-nil and hybridEnabled, runs the expanded queries
// through the same in-database vector + tsvector RRF the original query took in
// Stage 2. Sending only the original query through it left the expanded
// queries — which carry the cross-language translations, the ones a lexical
// match helps most — on plain vector search, so enabling the in-DB source
// removed lexical matching from most of the pipeline instead of extending it.
// Candidate-scoped retrieval keeps the chunk-repo path: HybridSearcher has no
// article filter (same carve-out as EmbedAndSearch).
func FuseResults(
	ctx context.Context,
	sc *StageContext,
	chunkRepo domain.RagChunkRepository,
	hybridSearcher domain.HybridSearcher,
	hybridEnabled bool,
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
	useHybridSearcher := hybridEnabled && hybridSearcher != nil && !hasCandidateArticles

	// Parallel search for expanded query embeddings (skip index 0, already done)
	searchStart := time.Now()
	allResults := make([][]domain.SearchResult, len(allEmbeddings))
	// Index 0 = original (already searched), reuse results
	allResults[0] = sc.OriginalResults

	g, gctx := errgroup.WithContext(ctx)
	for i := 1; i < len(allEmbeddings); i++ {
		idx, qv := i, allEmbeddings[i]
		queryText := ""
		if idx < len(allQueries) {
			queryText = allQueries[idx]
		}
		g.Go(func() error {
			var results []domain.SearchResult
			var err error
			switch {
			case useHybridSearcher:
				results, err = hybridSearcher.HybridSearch(gctx, qv, queryText, sc.SearchLimit)
			case hasCandidateArticles:
				results, err = chunkRepo.SearchWithinArticles(gctx, qv, sc.CandidateArticleIDs, sc.SearchLimit)
			default:
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
		slog.Bool("hybrid_expanded_arm", useHybridSearcher),
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
			continue
		}
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
						ArticleID:       res.ArticleID,
					},
				}
			}
			chunksMapExpanded[res.Chunk.ID].RRFScore += 1.0 / (rrfK + float64(rank+1))
		}
	}

	// The expanded list ranks on the fused RRF value, so that value is what
	// Score carries. Leaving Score at the per-query similarity of whichever
	// query happened to find the chunk first let Allocate's own sort re-order
	// the list by a number the fusion had already superseded.
	hitsExpanded := make([]ContextItem, 0, len(chunksMapExpanded))
	for _, data := range chunksMapExpanded {
		item := data.Item
		item.Score = float32(data.RRFScore)
		item.ScoreKind = domain.ScoreKindRRF
		hitsExpanded = append(hitsExpanded, item)
	}
	sort.SliceStable(hitsExpanded, func(i, j int) bool {
		return hitsExpanded[i].Score > hitsExpanded[j].Score
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
				"rrf":   hitsExpanded[i].Score,
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
// The fused score replaces both inputs, so the result set moves into RRF space.
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
	order := make([]string, 0, len(vectorResults)+len(bm25Results))

	for i, vr := range vectorResults {
		articleID := vr.ArticleID
		if _, exists := fusedMap[articleID]; !exists {
			vrCopy := vr
			fusedMap[articleID] = &fusedResult{vectorResult: &vrCopy}
			order = append(order, articleID)
		}
		fusedMap[articleID].rrfScore += 1.0 / (rrfK + float64(i+1))
	}

	for _, br := range bm25Results {
		articleID := br.ArticleID
		if existing, exists := fusedMap[articleID]; exists {
			existing.rrfScore += 1.0 / (rrfK + float64(br.Rank))
			continue
		}
		// BM25-only hit (no vector match): resolve it into a SearchResult
		// from the BM25 payload itself instead of dropping the contribution.
		bm25AsResult := bm25ResultToSearchResult(br)
		fusedMap[articleID] = &fusedResult{
			vectorResult: &bm25AsResult,
			rrfScore:     1.0 / (rrfK + float64(br.Rank)),
		}
		order = append(order, articleID)
	}

	results := make([]domain.SearchResult, 0, len(fusedMap))
	for _, articleID := range order {
		fr := fusedMap[articleID]
		result := *fr.vectorResult
		result.Score = float32(fr.rrfScore)
		result.ScoreKind = domain.ScoreKindRRF
		results = append(results, result)
	}

	sort.SliceStable(results, func(i, j int) bool {
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
//
// The index's own ordering is the only ranking signal here: the search-indexer
// response exposes no score, so every promoted hit carries Score 0. Downstream
// ranking therefore has to sort stably, or an all-zero list gets scrambled.
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
		ScoreKind: domain.ScoreKindBM25,
		ArticleID: br.ArticleID,
		Title:     br.Title,
		URL:       br.URL,
	}
}
