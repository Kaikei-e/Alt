package usecase

// QualityVerdict classifies the quality of retrieved context.
type QualityVerdict string

const (
	QualityGood         QualityVerdict = "good"
	QualityMarginal     QualityVerdict = "marginal"
	QualityInsufficient QualityVerdict = "insufficient"
)

// RetrievalQualityAssessor evaluates the quality of retrieved contexts
// using reranker scores (or RRF scores as fallback).
type RetrievalQualityAssessor struct {
	goodThreshold     float32
	marginalThreshold float32
	minContexts       int
}

// NewRetrievalQualityAssessor creates a new assessor.
// goodThreshold: average score >= this → Good
// marginalThreshold: average score >= this → Marginal
// minContexts: minimum number of contexts required (otherwise Insufficient)
func NewRetrievalQualityAssessor(goodThreshold, marginalThreshold float32, minContexts int) *RetrievalQualityAssessor {
	return &RetrievalQualityAssessor{
		goodThreshold:     goodThreshold,
		marginalThreshold: marginalThreshold,
		minContexts:       minContexts,
	}
}

// Assess evaluates the quality of retrieved contexts.
// Uses RerankScore when available (> 0), otherwise falls back to Score.
// Computes average of top-3 scores.
func (a *RetrievalQualityAssessor) Assess(contexts []ContextItem) QualityVerdict {
	if len(contexts) < a.minContexts {
		return QualityInsufficient
	}

	topN := 3
	if len(contexts) < topN {
		topN = len(contexts)
	}

	var sum float32
	for i := 0; i < topN; i++ {
		score := contexts[i].RerankScore
		if score == 0 {
			score = contexts[i].Score
		}
		sum += score
	}
	avg := sum / float32(topN)

	if avg >= a.goodThreshold {
		return QualityGood
	}
	if avg >= a.marginalThreshold {
		return QualityMarginal
	}
	return QualityInsufficient
}
