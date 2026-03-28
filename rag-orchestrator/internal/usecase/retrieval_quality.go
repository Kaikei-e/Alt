package usecase

import "strings"

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
// Checks: (1) average of top-3 scores, (2) topic coherence across titles,
// (3) score variance to detect "one hit + noise" pattern.
func (a *RetrievalQualityAssessor) Assess(contexts []ContextItem) QualityVerdict {
	if len(contexts) < a.minContexts {
		return QualityInsufficient
	}

	topN := 3
	if len(contexts) < topN {
		topN = len(contexts)
	}

	scores := make([]float32, topN)
	for i := 0; i < topN; i++ {
		scores[i] = contexts[i].RerankScore
		if scores[i] == 0 {
			scores[i] = contexts[i].Score
		}
	}

	var sum float32
	for _, s := range scores {
		sum += s
	}
	avg := sum / float32(topN)

	verdict := QualityInsufficient
	if avg >= a.goodThreshold {
		verdict = QualityGood
	} else if avg >= a.marginalThreshold {
		verdict = QualityMarginal
	}

	// Downgrade: topic incoherence — if top contexts have many unrelated titles
	if verdict == QualityGood && topN >= 2 && hasTopicIncoherence(contexts[:topN]) {
		verdict = QualityMarginal
	}

	// Downgrade: high score variance ("one hit + noise" pattern)
	if verdict == QualityGood && topN >= 2 && hasHighScoreVariance(scores) {
		verdict = QualityMarginal
	}

	return verdict
}

// AssessWithIntent evaluates retrieval quality with intent-specific strictness.
// For causal/explanatory queries, applies stricter coherence requirements:
// topic incoherence and score variance cause Insufficient (not just Marginal),
// and marginal average scores are treated as Insufficient (no retry).
// For other intents, delegates to the standard Assess() method.
func (a *RetrievalQualityAssessor) AssessWithIntent(contexts []ContextItem, intentType IntentType) QualityVerdict {
	baseVerdict := a.Assess(contexts)

	if intentType != IntentCausalExplanation {
		return baseVerdict
	}

	// Causal intent: stricter requirements
	// Marginal is not acceptable — causal synthesis from weak evidence produces overconfident nonsense
	if baseVerdict == QualityMarginal {
		return QualityInsufficient
	}

	// Even if base verdict is Good, re-check coherence signals with stricter thresholds
	if baseVerdict == QualityGood {
		topN := 3
		if len(contexts) < topN {
			topN = len(contexts)
		}
		if topN >= 2 && hasTopicIncoherence(contexts[:topN]) {
			return QualityInsufficient
		}

		scores := make([]float32, topN)
		for i := 0; i < topN; i++ {
			scores[i] = contexts[i].RerankScore
			if scores[i] == 0 {
				scores[i] = contexts[i].Score
			}
		}
		if hasHighScoreVariance(scores) {
			return QualityInsufficient
		}
	}

	return baseVerdict
}

// hasTopicIncoherence checks if the top contexts are from many unrelated sources.
// If every title is unique (no pair shares any significant word), this signals
// that retrieval scattered across unrelated topics.
// Skips the check when titles are empty (reranking-only flow without metadata).
func hasTopicIncoherence(contexts []ContextItem) bool {
	if len(contexts) < 2 {
		return false
	}
	// Only check when titles are populated
	titledCount := 0
	for _, c := range contexts {
		if strings.TrimSpace(c.Title) != "" {
			titledCount++
		}
	}
	if titledCount < 2 {
		return false
	}

	// Count how many pairs share at least one significant word in titles
	sharedPairs := 0
	totalPairs := 0
	for i := 0; i < len(contexts); i++ {
		if strings.TrimSpace(contexts[i].Title) == "" {
			continue
		}
		for j := i + 1; j < len(contexts); j++ {
			if strings.TrimSpace(contexts[j].Title) == "" {
				continue
			}
			totalPairs++
			if titlesShareWord(contexts[i].Title, contexts[j].Title) {
				sharedPairs++
			}
		}
	}
	// If no pair shares a word, titles are incoherent
	return totalPairs > 0 && sharedPairs == 0
}

// titlesShareWord checks if two titles share at least one significant word (>= 3 chars).
func titlesShareWord(a, b string) bool {
	wordsA := extractSignificantWords(a)
	wordsB := extractSignificantWords(b)
	for w := range wordsA {
		if wordsB[w] {
			return true
		}
	}
	return false
}

// extractSignificantWords returns lowercase words with 3+ runes from a title.
func extractSignificantWords(title string) map[string]bool {
	words := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(title)) {
		if len([]rune(w)) >= 3 {
			words[w] = true
		}
	}
	return words
}

// hasHighScoreVariance detects the "one strong hit + noise" pattern.
// If the top score is more than 5x the second score, the retrieval likely
// found one relevant chunk amid irrelevant noise.
func hasHighScoreVariance(scores []float32) bool {
	if len(scores) < 2 || scores[1] == 0 {
		return scores[0] > 0
	}
	ratio := scores[0] / scores[1]
	return ratio > 5.0
}
