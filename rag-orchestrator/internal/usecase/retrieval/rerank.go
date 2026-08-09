package retrieval

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"rag-orchestrator/internal/domain"
)

// RerankConfig holds reranking stage parameters.
type RerankConfig struct {
	Enabled bool
	TopK    int
	Timeout time.Duration
}

// Rerank applies cross-encoder reranking to the candidate results (Stage 4).
func Rerank(
	ctx context.Context,
	sc *StageContext,
	reranker domain.Reranker,
	cfg RerankConfig,
	logger *slog.Logger,
) {
	if !cfg.Enabled || reranker == nil {
		return
	}

	rerankStart := time.Now()

	// Prepare candidates from all unique hits (original + expanded), keyed by
	// hitIdentity rather than chunk id: BM25 hits all carry uuid.Nil, and
	// keying on that collapses an entire BM25-only result set into one
	// candidate. A hit that can be identified by neither id is left at its
	// retrieval score instead — there would be no way to map a cross-encoder
	// score back onto it alone.
	candidateMap := make(map[string]domain.SearchResult)
	candidateOrder := make([]string, 0, len(sc.HitsOriginal)+len(sc.HitsExpanded))
	addCandidate := func(key string, res domain.SearchResult) {
		if key == "" {
			return
		}
		if _, exists := candidateMap[key]; exists {
			return
		}
		candidateMap[key] = res
		candidateOrder = append(candidateOrder, key)
	}

	for _, res := range sc.HitsOriginal {
		addCandidate(hitIdentity(res.Chunk.ID, res.ArticleID), res)
	}
	for _, item := range sc.HitsExpanded {
		addCandidate(hitIdentity(item.ChunkID, item.ArticleID), domain.SearchResult{
			Chunk: domain.RagChunk{
				ID:      item.ChunkID,
				Content: item.ChunkText,
			},
			Score:           item.Score,
			ArticleID:       item.ArticleID,
			Title:           item.Title,
			URL:             item.URL,
			DocumentVersion: item.DocumentVersion,
		})
	}

	// Convert to rerank candidates in insertion order, so which candidates
	// survive the cap below does not depend on map iteration order.
	candidates := make([]domain.RerankCandidate, 0, len(candidateOrder))
	for _, key := range candidateOrder {
		res := candidateMap[key]
		candidates = append(candidates, domain.RerankCandidate{
			ID:      key,
			Content: res.Chunk.Content,
			Score:   res.Score,
		})
	}

	// Limit candidates to prevent reranker timeout on cross-encoder inference.
	// MPS benchmark: 30 candidates = 16.7s, 15 candidates ≈ 8s (fits within 12s timeout).
	// cfg.TopK drives the actual cap; this is only the fallback when TopK is unset.
	const defaultMaxRerankCandidates = 15
	maxRerankCandidates := cfg.TopK
	if maxRerankCandidates <= 0 {
		maxRerankCandidates = defaultMaxRerankCandidates
	}
	if len(candidates) > maxRerankCandidates {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Score > candidates[j].Score
		})
		candidates = candidates[:maxRerankCandidates]
	}

	// Call reranker with timeout
	rerankCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	reranked, err := reranker.Rerank(rerankCtx, sc.Query, candidates)
	cancel()

	rerankDuration := time.Since(rerankStart)

	if err != nil {
		logger.Warn("reranking_failed_using_original_scores",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", rerankDuration.Milliseconds()))
		return
	}

	logger.Info("reranking_completed",
		slog.String("retrieval_id", sc.RetrievalID),
		slog.Int("candidate_count", len(candidates)),
		slog.Int("reranked_count", len(reranked)),
		slog.String("model", reranker.ModelName()),
		slog.Int64("duration_ms", rerankDuration.Milliseconds()))

	sc.RerankApplied = true

	// Apply reranked scores
	rerankScores := make(map[string]float32, len(reranked))
	for _, r := range reranked {
		rerankScores[r.ID] = r.Score
	}

	// Update original hits scores
	for i := range sc.HitsOriginal {
		if score, ok := rerankScores[hitIdentity(sc.HitsOriginal[i].Chunk.ID, sc.HitsOriginal[i].ArticleID)]; ok {
			sc.HitsOriginal[i].Score = score
		}
	}
	sort.Slice(sc.HitsOriginal, func(i, j int) bool {
		return sc.HitsOriginal[i].Score > sc.HitsOriginal[j].Score
	})

	// Update expanded hits scores and propagate rerank scores
	for i := range sc.HitsExpanded {
		if score, ok := rerankScores[hitIdentity(sc.HitsExpanded[i].ChunkID, sc.HitsExpanded[i].ArticleID)]; ok {
			sc.HitsExpanded[i].Score = score
			sc.HitsExpanded[i].RerankScore = score
			sc.HitsExpanded[i].RerankApplied = true
		}
	}
	sort.Slice(sc.HitsExpanded, func(i, j int) bool {
		return sc.HitsExpanded[i].Score > sc.HitsExpanded[j].Score
	})
}
