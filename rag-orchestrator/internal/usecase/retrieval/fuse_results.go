package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

// FuseResults runs parallel vector search for expanded queries and applies RRF fusion (Stage 3).
func FuseResults(
	ctx context.Context,
	sc *StageContext,
	chunkRepo domain.RagChunkRepository,
	logger *slog.Logger,
) error {
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
	type searchResult struct {
		index   int
		results []domain.SearchResult
		err     error
	}

	searchStart := time.Now()
	resultsChan := make(chan searchResult, len(allEmbeddings))
	var wg sync.WaitGroup

	// Index 0 = original (already searched), reuse results
	resultsChan <- searchResult{index: 0, results: sc.OriginalResults, err: nil}

	for i := 1; i < len(allEmbeddings); i++ {
		wg.Add(1)
		go func(idx int, qv []float32) {
			defer wg.Done()
			var results []domain.SearchResult
			var err error
			if hasCandidateArticles {
				results, err = chunkRepo.SearchWithinArticles(ctx, qv, sc.CandidateArticleIDs, sc.SearchLimit)
			} else {
				results, err = chunkRepo.Search(ctx, qv, sc.SearchLimit)
			}
			resultsChan <- searchResult{index: idx, results: results, err: err}
		}(i, allEmbeddings[i])
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	allResults := make([][]domain.SearchResult, len(allEmbeddings))
	var searchErr error
	for sr := range resultsChan {
		if sr.err != nil && searchErr == nil {
			searchErr = sr.err
		}
		allResults[sr.index] = sr.results
	}
	if searchErr != nil {
		return fmt.Errorf("failed to search chunks: %w", searchErr)
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
			fusedMap[articleID] = &fusedResult{
				vectorResult: nil,
				rrfScore:     1.0 / (rrfK + float64(br.Rank)),
			}
		}
	}

	results := make([]domain.SearchResult, 0, len(fusedMap))
	for _, fr := range fusedMap {
		if fr.vectorResult != nil {
			result := *fr.vectorResult
			result.Score = float32(fr.rrfScore)
			results = append(results, result)
		}
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
