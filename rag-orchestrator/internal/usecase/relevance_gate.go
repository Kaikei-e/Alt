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
//
// The thresholds only mean anything on the cross-encoder's scale, so the gate
// judges a score exclusively when the reranker actually produced one. Without
// it, ContextItem.Score holds an RRF score instead — around 0.016–0.033 for
// k=60 regardless of how relevant the hit is — and reading that as a
// cross-encoder score condemns every retrieval, which the caller turns into a
// hard stop that leaves the question unanswered. An unjudgeable retrieval is
// reported Marginal: no evidence the context is good, and none that it is bad.
func (g *RelevanceGate) Evaluate(contexts []ContextItem) QualityVerdict {
	if len(contexts) == 0 {
		return QualityInsufficient
	}

	top := contexts[0]
	if !top.RerankApplied {
		return QualityMarginal
	}

	if top.RerankScore >= g.goodThreshold {
		return QualityGood
	}
	if top.RerankScore >= g.marginalThreshold {
		return QualityMarginal
	}
	return QualityInsufficient
}
