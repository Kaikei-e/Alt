package retrieval

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"rag-orchestrator/internal/domain"
)

// Fallbacks for a RerankConfig that leaves a field unset. The real values come
// from config (RERANK_TOP_K / RERANK_MAX_CANDIDATES).
const (
	defaultRerankTopK          = 10
	defaultRerankMaxCandidates = 40
)

// RerankConfig holds reranking stage parameters.
type RerankConfig struct {
	Enabled bool
	// TopK is how many hits survive the stage — it shapes the output.
	TopK int
	// MaxCandidates is how many hits are sent to the cross-encoder — it shapes
	// the input. The two are different numbers: capping the input at TopK
	// meant a hit ranked below TopK by retrieval could never be promoted, which
	// is the entire purpose of a reranking stage.
	MaxCandidates int
	Timeout       time.Duration
}

// Rerank applies cross-encoder reranking to the candidate results (Stage 4).
//
// Reranking is an enhancement tier: when it is unavailable the pipeline keeps
// the retrieval order (ADR-000943's Degraded contract for RAG rerank). That
// degradation is always logged at error level — an answer built on unranked
// context is a quality regression the operator has to be able to see.
func Rerank(
	ctx context.Context,
	sc *StageContext,
	reranker domain.Reranker,
	cfg RerankConfig,
	logger *slog.Logger,
) {
	if !cfg.Enabled {
		return
	}
	if reranker == nil {
		// Reranking is switched on but nothing was wired to it. Distinguish
		// that from an operator disabling the stage, which returns above.
		logger.Error("rerank_enabled_but_unwired",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.String("degraded_mode", "rerank_skipped"),
			slog.String("hint", "RERANK_ENABLED=true but no reranker reached the retrieval graph"))
		return
	}

	rerankStart := time.Now()

	// Prepare candidates from all unique hits (original + expanded), keyed by
	// hitIdentity rather than chunk id: BM25 hits all carry uuid.Nil, and
	// keying on that collapses an entire BM25-only result set into one
	// candidate. A hit that can be identified by neither id cannot be scored
	// at all — there would be no way to map a cross-encoder score back onto
	// it alone — so it does not enter the window.
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
			ScoreKind:       item.ScoreKind,
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

	topK := cfg.TopK
	if topK <= 0 {
		topK = defaultRerankTopK
	}
	maxCandidates := cfg.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = defaultRerankMaxCandidates
	}
	if len(candidates) > maxCandidates {
		// Stable so a degraded result set whose scores are all equal (BM25-only
		// retrieval carries no score at all) keeps its retrieval order.
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].Score > candidates[j].Score
		})
		candidates = candidates[:maxCandidates]
	}

	rerankCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	reranked, err := reranker.Rerank(rerankCtx, sc.Query, candidates)
	cancel()

	rerankDuration := time.Since(rerankStart)

	fallback := func(reason string, errText string) {
		logger.Error("rerank_fallback_original_scores",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.String("degraded_mode", "rerank_skipped"),
			slog.String("reason", reason),
			slog.String("error", errText),
			slog.String("model", reranker.ModelName()),
			slog.Int("candidate_count", len(candidates)),
			slog.Int64("duration_ms", rerankDuration.Milliseconds()),
			slog.Int64("timeout_ms", cfg.Timeout.Milliseconds()))
	}

	if err != nil {
		fallback("rerank_call_failed", err.Error())
		return
	}
	if len(reranked) == 0 {
		// A 200 with no scores is indistinguishable from "everything scored 0"
		// once the scores are applied, and would empty the pipeline through the
		// TopK truncation below.
		fallback("rerank_returned_no_scores", "")
		return
	}

	rerankScores := make(map[string]float32, len(reranked))
	for _, r := range reranked {
		rerankScores[r.ID] = r.Score
	}

	keptOriginal := rerankedHits(sc.HitsOriginal, rerankScores, topK)
	keptExpanded := rerankedItems(sc.HitsExpanded, rerankScores, topK)

	if len(keptOriginal) == 0 && len(keptExpanded) == 0 {
		// The reranker answered about candidates we never sent.
		fallback("rerank_ids_unmatched", "")
		return
	}

	dropped := (len(sc.HitsOriginal) - len(keptOriginal)) + (len(sc.HitsExpanded) - len(keptExpanded))
	sc.HitsOriginal = keptOriginal
	sc.HitsExpanded = keptExpanded
	sc.RerankApplied = true

	logger.Info("reranking_completed",
		slog.String("retrieval_id", sc.RetrievalID),
		slog.Int("candidate_count", len(candidates)),
		slog.Int("reranked_count", len(reranked)),
		slog.Int("top_k", topK),
		slog.Int("max_candidates", maxCandidates),
		slog.Int("dropped_below_top_k", dropped),
		slog.String("model", reranker.ModelName()),
		slog.Int64("duration_ms", rerankDuration.Milliseconds()))
}

// rerankedHits keeps the hits the cross-encoder actually scored, moves them
// into the cross-encoder's score space and cuts the list to topK. Hits outside
// the window are dropped rather than kept at their retrieval score: mixing the
// two spaces in one sorted list is the defect this stage exists to avoid.
func rerankedHits(hits []domain.SearchResult, scores map[string]float32, topK int) []domain.SearchResult {
	kept := make([]domain.SearchResult, 0, len(hits))
	for _, hit := range hits {
		score, ok := scores[hitIdentity(hit.Chunk.ID, hit.ArticleID)]
		if !ok {
			continue
		}
		hit.Score = score
		hit.ScoreKind = domain.ScoreKindRerank
		kept = append(kept, hit)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Score > kept[j].Score })
	if len(kept) > topK {
		kept = kept[:topK]
	}
	return kept
}

func rerankedItems(items []ContextItem, scores map[string]float32, topK int) []ContextItem {
	kept := make([]ContextItem, 0, len(items))
	for _, item := range items {
		score, ok := scores[hitIdentity(item.ChunkID, item.ArticleID)]
		if !ok {
			continue
		}
		item.Score = score
		item.ScoreKind = domain.ScoreKindRerank
		item.RerankScore = score
		item.RerankApplied = true
		kept = append(kept, item)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Score > kept[j].Score })
	if len(kept) > topK {
		kept = kept[:topK]
	}
	return kept
}
