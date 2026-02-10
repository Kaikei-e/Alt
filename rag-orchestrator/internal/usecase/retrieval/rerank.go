package retrieval

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
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

	// Prepare candidates from all unique hits (original + expanded)
	candidateMap := make(map[uuid.UUID]domain.SearchResult)
	for _, res := range sc.HitsOriginal {
		candidateMap[res.Chunk.ID] = res
	}
	for _, item := range sc.HitsExpanded {
		if _, exists := candidateMap[item.ChunkID]; !exists {
			candidateMap[item.ChunkID] = domain.SearchResult{
				Chunk: domain.RagChunk{
					ID:      item.ChunkID,
					Content: item.ChunkText,
				},
				Score:           item.Score,
				Title:           item.Title,
				URL:             item.URL,
				DocumentVersion: item.DocumentVersion,
			}
		}
	}

	// Convert to rerank candidates
	candidates := make([]domain.RerankCandidate, 0, len(candidateMap))
	for id, res := range candidateMap {
		candidates = append(candidates, domain.RerankCandidate{
			ID:      id.String(),
			Content: res.Chunk.Content,
			Score:   res.Score,
		})
	}

	// Limit candidates to prevent reranker timeout on cross-encoder inference.
	const maxRerankCandidates = 30
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

	// Apply reranked scores
	rerankScores := make(map[uuid.UUID]float32)
	for _, r := range reranked {
		id, _ := uuid.Parse(r.ID)
		rerankScores[id] = r.Score
	}

	// Update original hits scores
	for i := range sc.HitsOriginal {
		if score, ok := rerankScores[sc.HitsOriginal[i].Chunk.ID]; ok {
			sc.HitsOriginal[i].Score = score
		}
	}
	sort.Slice(sc.HitsOriginal, func(i, j int) bool {
		return sc.HitsOriginal[i].Score > sc.HitsOriginal[j].Score
	})

	// Update expanded hits scores
	for i := range sc.HitsExpanded {
		if score, ok := rerankScores[sc.HitsExpanded[i].ChunkID]; ok {
			sc.HitsExpanded[i].Score = score
		}
	}
	sort.Slice(sc.HitsExpanded, func(i, j int) bool {
		return sc.HitsExpanded[i].Score > sc.HitsExpanded[j].Score
	})
}
