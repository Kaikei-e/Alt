package usecase

// RelevanceGate evaluates retrieval quality using cross-encoder reranker scores.
// Uses top-1 score against calibrated thresholds instead of heuristic string matching.
type RelevanceGate struct {
	goodThreshold     float32
	marginalThreshold float32
}

// NewRelevanceGate creates a gate with calibrated thresholds.
// goodThreshold: top-1 score >= this → Good (proceed to generation)
// marginalThreshold: top-1 score >= this → Marginal (retry with re-planned query)
// Below marginalThreshold → Insufficient (fallback)
func NewRelevanceGate(goodThreshold, marginalThreshold float32) *RelevanceGate {
	return &RelevanceGate{
		goodThreshold:     goodThreshold,
		marginalThreshold: marginalThreshold,
	}
}

// Evaluate checks the top-1 reranker score against calibrated thresholds.
// Uses RerankScore when available (> 0), otherwise falls back to Score.
func (g *RelevanceGate) Evaluate(contexts []ContextItem) QualityVerdict {
	if len(contexts) == 0 {
		return QualityInsufficient
	}

	top := contexts[0]
	score := top.RerankScore
	if score == 0 {
		score = top.Score
	}

	if score >= g.goodThreshold {
		return QualityGood
	}
	if score >= g.marginalThreshold {
		return QualityMarginal
	}
	return QualityInsufficient
}
