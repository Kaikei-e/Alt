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
// topic incoherence, score variance, and query-context mismatch on "good" base verdict cause Insufficient.
// Marginal base verdict is preserved (allows retry), but if the retrieval is
// both marginal AND incoherent/mismatched, it becomes Insufficient.
// For other intents, delegates to the standard Assess() method.
// The query parameter enables query-context relevance checking (pass "" to skip).
func (a *RetrievalQualityAssessor) AssessWithIntent(contexts []ContextItem, intentType IntentType, query string) QualityVerdict {
	baseVerdict := a.Assess(contexts)

	if intentType != IntentCausalExplanation {
		return baseVerdict
	}

	topN := 3
	if len(contexts) < topN {
		topN = len(contexts)
	}

	incoherent := topN >= 2 && hasTopicIncoherence(contexts[:topN])

	scores := make([]float32, topN)
	for i := 0; i < topN; i++ {
		scores[i] = contexts[i].RerankScore
		if scores[i] == 0 {
			scores[i] = contexts[i].Score
		}
	}
	highVariance := topN >= 2 && hasHighScoreVariance(scores)

	// Query-context relevance: check if contexts are about the query topic
	// Uses Title + ChunkText, not title alone.
	mismatch := query != "" && hasQueryContextMismatch(query, contexts[:topN])

	// When context IS relevant to query (no mismatch), title incoherence is less meaningful:
	// articles about the same topic often have diverse titles (e.g., "Middle East Update",
	// "Energy Markets Weekly" can both be about Iran oil crisis).

	// Causal + Good but incoherent or high variance → Insufficient
	if baseVerdict == QualityGood && (incoherent || highVariance) {
		return QualityInsufficient
	}

	// Causal + Marginal + incoherent
	if baseVerdict == QualityMarginal && incoherent {
		// If query is provided and content is relevant, keep Marginal (diverse titles for same topic)
		if query != "" && !mismatch {
			return QualityMarginal
		}
		return QualityInsufficient
	}

	// Causal + Marginal + mismatch (content doesn't match query)
	if baseVerdict == QualityMarginal && mismatch {
		return QualityInsufficient
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

// hasQueryContextMismatch checks whether retrieved contexts are relevant to the query.
// Examines both Title and ChunkText (not title only) to avoid false positives.
// Returns true if NONE of the top contexts contain any significant query keyword.
// For CJK queries, extracts CJK substrings of 2+ runes.
// For Latin queries, extracts words of 3+ runes.
func hasQueryContextMismatch(query string, contexts []ContextItem) bool {
	if query == "" || len(contexts) == 0 {
		return false
	}

	keywords := extractQueryKeywords(query)
	if len(keywords) == 0 {
		return false
	}

	// Check if ANY context matches ANY keyword (in title or chunk text)
	for _, ctx := range contexts {
		combined := strings.ToLower(ctx.Title + " " + ctx.ChunkText)
		for _, kw := range keywords {
			if strings.Contains(combined, kw) {
				return false // found a match
			}
		}
	}
	return true // no context matched any keyword
}

// extractQueryKeywords extracts significant keywords from a query.
// For CJK: splits at hiragana boundaries, extracts katakana/kanji segments of 2+ runes.
// For Latin: extracts words of 3+ runes (excluding stop words).
func extractQueryKeywords(query string) []string {
	var keywords []string

	// Extract CJK keyword segments: accumulate katakana + kanji runs,
	// treating hiragana as word boundaries (particles like の、は、が).
	var segBuf []rune
	for _, r := range query {
		if isKeywordCJKRune(r) {
			segBuf = append(segBuf, r)
		} else {
			if len(segBuf) >= 2 {
				keywords = append(keywords, string(segBuf))
			}
			segBuf = segBuf[:0]
		}
	}
	if len(segBuf) >= 2 {
		keywords = append(keywords, string(segBuf))
	}

	// Extract Latin words (3+ runes, excluding stop words)
	for _, w := range strings.Fields(strings.ToLower(query)) {
		w = strings.Trim(w, "?？!！.,;:()（）「」")
		if len([]rune(w)) >= 3 && !isStopWord(w) && !containsCJK(w) {
			keywords = append(keywords, w)
		}
	}

	return keywords
}

// isKeywordCJKRune returns true for katakana and kanji (content characters),
// but false for hiragana (grammatical particles used as word boundaries).
func isKeywordCJKRune(r rune) bool {
	return (r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) // CJK Extension A
}

func isCJKRune(r rune) bool {
	return (r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) // CJK Extension A
}

func containsCJK(s string) bool {
	for _, r := range s {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "was": true,
	"were": true, "been": true, "has": true, "have": true, "had": true,
	"not": true, "but": true, "with": true, "this": true, "that": true,
	"from": true, "they": true, "what": true, "why": true, "how": true,
	"who": true, "which": true, "when": true, "where": true,
}

func isStopWord(w string) bool {
	return stopWords[w]
}
