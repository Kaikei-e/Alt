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
// reads the context's declared score space and judges nothing else. Every
// degrade mode has a defined answer:
//
//   - reranked (ScoreKindRerank): judged against the thresholds.
//   - rerank skipped or failed: Score is an RRF value, around 0.016-0.033 for
//     k=60 however relevant the hit. Reading that as a cross-encoder score
//     condemns every retrieval, and the caller turns Insufficient into a hard
//     stop that leaves the question unanswered.
//   - BM25-only (embedder down): Score is a lexical score of unbounded range,
//     or 0 when the index exposes none. It cannot fail or pass a [0,1] test.
//
// In all three uncalibrated cases the verdict is Marginal: no evidence the
// context is good, and none that it is bad.
func (g *RelevanceGate) Evaluate(contexts []ContextItem) QualityVerdict {
	if len(contexts) == 0 {
		return QualityInsufficient
	}

	top := contexts[0]
	if !top.ScoreKind.Calibrated() {
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
