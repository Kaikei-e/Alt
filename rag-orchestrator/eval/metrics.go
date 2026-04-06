package eval

import (
	"math"
	"strings"
	"unicode/utf8"
)

// VerifyCase checks a single EvalResult against its GoldenCase expectations.
func VerifyCase(gc GoldenCase, result EvalResult) CaseVerdict {
	v := CaseVerdict{CaseID: gc.ID, Passed: true}

	// Clarification check
	if gc.Expected.ShouldClarify {
		if !result.ClarificationAsked {
			v.fail("expected clarification but none was asked")
		}
		// If clarification is expected, other checks are skipped
		return v
	}
	if !gc.Expected.ShouldClarify && result.ClarificationAsked {
		v.fail("unexpected clarification was asked")
	}

	// Irrelevant titles check
	if len(gc.Expected.IrrelevantTitles) > 0 {
		found := ContainsIrrelevant(result.RetrievedTitles, gc.Expected.IrrelevantTitles)
		if len(found) > 0 {
			v.fail("irrelevant titles in retrieval: " + strings.Join(found, ", "))
		}
	}

	// Minimum relevant contexts
	if gc.Expected.MinRelevantContexts > 0 {
		relevant := countRelevant(result.RetrievedTitles, gc.Expected.ExpectedTopicKeywords)
		if relevant < gc.Expected.MinRelevantContexts {
			v.failf("min relevant contexts: got %d, want >= %d", relevant, gc.Expected.MinRelevantContexts)
		}
	}

	// Intent classification
	if gc.Expected.ExpectedIntent != "" && result.IntentClassified != gc.Expected.ExpectedIntent {
		v.failf("intent: got %q, want %q", result.IntentClassified, gc.Expected.ExpectedIntent)
	}

	// Answer length
	if gc.Expected.MinAnswerLength > 0 {
		runeLen := utf8.RuneCountInString(result.Answer)
		if runeLen < gc.Expected.MinAnswerLength {
			v.failf("answer length: got %d runes, want >= %d", runeLen, gc.Expected.MinAnswerLength)
		}
	}

	// Citations required
	if gc.Expected.RequiresCitations && result.CitationCount == 0 {
		v.fail("citations required but none provided")
	}

	// Expected entities in answer
	for _, entity := range gc.Expected.ExpectedEntities {
		if !strings.Contains(result.Answer, entity) {
			v.failf("expected entity %q not found in answer", entity)
		}
	}

	// Forbidden patterns
	for _, pattern := range gc.Expected.ForbiddenPatterns {
		if strings.Contains(result.Answer, pattern) {
			v.failf("forbidden pattern %q found in answer", pattern)
		}
	}

	return v
}

func (v *CaseVerdict) fail(reason string) {
	v.Passed = false
	v.Failures = append(v.Failures, reason)
}

func (v *CaseVerdict) failf(format string, args ...interface{}) {
	v.fail(sprintf(format, args...))
}

// sprintf is a minimal formatter to avoid importing fmt in hot path.
func sprintf(format string, args ...interface{}) string {
	// Use strings.Builder for efficiency; we only need %d and %q
	var b strings.Builder
	argIdx := 0
	for i := 0; i < len(format); i++ {
		if format[i] == '%' && i+1 < len(format) && argIdx < len(args) {
			switch format[i+1] {
			case 'd':
				b.WriteString(intToStr(args[argIdx].(int)))
				argIdx++
				i++
				continue
			case 'q':
				b.WriteByte('"')
				b.WriteString(args[argIdx].(string))
				b.WriteByte('"')
				argIdx++
				i++
				continue
			case 's':
				b.WriteString(args[argIdx].(string))
				argIdx++
				i++
				continue
			}
		}
		b.WriteByte(format[i])
	}
	return b.String()
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

// countRelevant counts how many retrieved titles contain at least one expected keyword.
func countRelevant(retrievedTitles []string, keywords []string) int {
	count := 0
	for _, title := range retrievedTitles {
		lower := strings.ToLower(title)
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				count++
				break
			}
		}
	}
	return count
}

// RecallAtK computes recall@K: fraction of expected relevant items found in top-K retrieved.
func RecallAtK(relevantTitles []string, retrievedTitles []string, k int) float64 {
	if len(relevantTitles) == 0 {
		return 0.0
	}
	topK := retrievedTitles
	if k < len(topK) {
		topK = topK[:k]
	}
	relevantSet := toSet(relevantTitles)
	found := 0
	for _, t := range topK {
		if relevantSet[t] {
			found++
		}
	}
	return float64(found) / float64(len(relevantTitles))
}

// NDCGAtK computes nDCG@K (Normalized Discounted Cumulative Gain).
func NDCGAtK(relevanceScores map[string]int, retrievedTitles []string, k int) float64 {
	if len(relevanceScores) == 0 || len(retrievedTitles) == 0 {
		return 0.0
	}

	topK := retrievedTitles
	if k < len(topK) {
		topK = topK[:k]
	}

	// DCG
	dcg := 0.0
	for i, title := range topK {
		rel := relevanceScores[title] // 0 if not found
		dcg += float64(rel) / math.Log2(float64(i+2))
	}

	// IDCG: sort relevance scores descending
	sorted := sortedValues(relevanceScores)
	idealK := k
	if idealK > len(sorted) {
		idealK = len(sorted)
	}
	idcg := 0.0
	for i := 0; i < idealK; i++ {
		idcg += float64(sorted[i]) / math.Log2(float64(i+2))
	}

	if idcg == 0 {
		return 0.0
	}
	return dcg / idcg
}

// sortedValues returns values from the map sorted descending.
func sortedValues(m map[string]int) []int {
	vals := make([]int, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	// Simple insertion sort (small maps)
	for i := 1; i < len(vals); i++ {
		for j := i; j > 0 && vals[j] > vals[j-1]; j-- {
			vals[j], vals[j-1] = vals[j-1], vals[j]
		}
	}
	return vals
}

// Top1Precision returns 1.0 if the top-1 retrieved title is relevant, 0.0 otherwise.
func Top1Precision(relevantTitles []string, retrievedTitles []string) float64 {
	if len(retrievedTitles) == 0 {
		return 0.0
	}
	relevantSet := toSet(relevantTitles)
	if relevantSet[retrievedTitles[0]] {
		return 1.0
	}
	return 0.0
}

// Faithfulness estimates what fraction of answer claims are supported by context.
// Simplified heuristic: for each expected entity, checks if it appears in both
// the answer AND at least one context chunk.
func Faithfulness(answer string, contextChunks []string, expectedEntities []string) float64 {
	if len(expectedEntities) == 0 {
		return 0.0
	}
	joinedContext := strings.Join(contextChunks, " ")
	supported := 0
	for _, entity := range expectedEntities {
		inAnswer := strings.Contains(answer, entity)
		inContext := strings.Contains(joinedContext, entity)
		if inAnswer && inContext {
			supported++
		}
	}
	return float64(supported) / float64(len(expectedEntities))
}

// CitationCorrectness checks what fraction of cited titles are in the relevant set.
func CitationCorrectness(citedTitles []string, relevantTitles []string) float64 {
	if len(citedTitles) == 0 {
		return 0.0
	}
	relevantSet := toSet(relevantTitles)
	correct := 0
	for _, t := range citedTitles {
		if relevantSet[t] {
			correct++
		}
	}
	return float64(correct) / float64(len(citedTitles))
}

// ContainsIrrelevant checks if any irrelevant titles appear in retrieved results.
func ContainsIrrelevant(retrievedTitles []string, irrelevantTitles []string) []string {
	irrelevantSet := toSet(irrelevantTitles)
	var found []string
	for _, t := range retrievedTitles {
		if irrelevantSet[t] {
			found = append(found, t)
		}
	}
	return found
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
