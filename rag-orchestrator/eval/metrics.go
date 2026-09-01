package eval

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	// defaultRecallK is the retrieval depth every recall floor is judged at.
	defaultRecallK = 20
	// recallEpsilon absorbs float division noise so a case asking for 1/3
	// recall is not failed by 0.33333332.
	recallEpsilon = 1e-9
)

// VerifyCase checks a single EvalResult against its GoldenCase expectations.
func VerifyCase(gc GoldenCase, result EvalResult) CaseVerdict {
	v := CaseVerdict{CaseID: gc.ID, Category: gc.Category, Passed: true}

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

	// Forbidden articles: spam, near-duplicates and index pollution must not be
	// retrieved, and must certainly not be cited.
	if len(gc.Expected.ForbiddenArticleIDs) > 0 {
		if hits := ForbiddenHits(result.RetrievedArticleIDs, gc.Expected.ForbiddenArticleIDs); len(hits) > 0 {
			v.fail("forbidden articles in retrieval: " + strings.Join(hits, ", "))
		}
		if hits := ForbiddenHits(result.CitedArticleIDs, gc.Expected.ForbiddenArticleIDs); len(hits) > 0 {
			v.fail("forbidden articles cited: " + strings.Join(hits, ", "))
		}
	}

	// Expected no-answer: the corpus has nothing, so silence beats invention.
	if gc.Expected.ExpectNoAnswer {
		if result.CitationCount > 0 {
			v.failf("expected no citations for an unanswerable query, got %d", result.CitationCount)
		}
		return v
	}

	// Retrieval recall floor over the verified article set.
	if gc.Expected.MinExpectedRecall > 0 && len(gc.Expected.RelevantArticleIDs) > 0 {
		recall := RecallAtKByID(gc.Expected.RelevantArticleIDs, result.RetrievedArticleIDs, defaultRecallK)
		if recall+recallEpsilon < gc.Expected.MinExpectedRecall {
			v.failf("recall@%d: got %.2f, want >= %.2f", defaultRecallK, recall, gc.Expected.MinExpectedRecall)
		}
	}

	// Articles the answer is required to cite.
	for _, id := range gc.Expected.ExpectedCitationArticleIDs {
		if !containsID(result.CitedArticleIDs, id) {
			v.failf("expected citation of article %s not found", id)
		}
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

	// Expected structure (instruction adherence)
	for _, structure := range gc.Expected.ExpectedStructure {
		if !strings.Contains(result.Answer, structure) {
			v.failf("expected structure %q not found in answer", structure)
		}
	}

	return v
}

func (v *CaseVerdict) fail(reason string) {
	v.Passed = false
	v.Failures = append(v.Failures, reason)
}

func (v *CaseVerdict) failf(format string, args ...interface{}) {
	v.fail(fmt.Sprintf(format, args...))
}

// countRelevant counts how many retrieved titles contain at least one expected keyword.
func countRelevant(retrievedTitles []string, keywords []string) int {
	count := 0
	for _, title := range retrievedTitles {
		if titleMatchesAnyKeyword(title, keywords) {
			count++
		}
	}
	return count
}

// titleMatchesAnyKeyword reports whether title contains (case-insensitively)
// any of the given keywords. Callers pass topic keywords as the "relevant"
// set to RecallAtK/Top1Precision/CitationCorrectness, not full titles, so an
// exact-match set lookup would almost never fire — this is the same
// substring rule countRelevant already uses.
func titleMatchesAnyKeyword(title string, keywords []string) bool {
	lower := strings.ToLower(title)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
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
	found := 0
	for _, t := range topK {
		if titleMatchesAnyKeyword(t, relevantTitles) {
			found++
		}
	}
	return float64(found) / float64(len(relevantTitles))
}

// RecallAtKByID computes recall@K over article ids. Unlike RecallAtK it matches
// ids exactly: a golden case names the articles a correct retrieval must reach,
// so a substring rule would turn a near-miss into a pass.
func RecallAtKByID(relevantIDs []string, retrievedIDs []string, k int) float64 {
	if len(relevantIDs) == 0 {
		return 0.0
	}
	topK := truncate(retrievedIDs, k)
	retrieved := toSet(topK)
	found := 0
	for _, id := range uniqueIDs(relevantIDs) {
		if retrieved[id] {
			found++
		}
	}
	return float64(found) / float64(len(uniqueIDs(relevantIDs)))
}

// ReciprocalRankByID returns 1/rank of the first relevant article, 0 when none
// of them was retrieved. Averaged over cases this is MRR.
func ReciprocalRankByID(relevantIDs []string, retrievedIDs []string) float64 {
	if len(relevantIDs) == 0 || len(retrievedIDs) == 0 {
		return 0.0
	}
	relevant := toSet(relevantIDs)
	for i, id := range retrievedIDs {
		if relevant[id] {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

// RerankGain is the nDCG@K the reranker added on top of the order it was given.
// A negative value means the cross-encoder demoted articles the fusion stage had
// already ranked correctly.
func RerankGain(grades map[string]int, preRerankIDs, postRerankIDs []string, k int) float64 {
	return NDCGAtK(grades, postRerankIDs, k) - NDCGAtK(grades, preRerankIDs, k)
}

// ForbiddenHits returns the forbidden ids that appear in the given list, in the
// order the list presents them.
func ForbiddenHits(ids []string, forbiddenIDs []string) []string {
	if len(ids) == 0 || len(forbiddenIDs) == 0 {
		return nil
	}
	forbidden := toSet(forbiddenIDs)
	var hits []string
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if forbidden[id] && !seen[id] {
			hits = append(hits, id)
			seen[id] = true
		}
	}
	return hits
}

// CitationRecallByID reports the fraction of must-cite articles the answer
// actually cited.
func CitationRecallByID(expectedIDs []string, citedIDs []string) float64 {
	expected := uniqueIDs(expectedIDs)
	if len(expected) == 0 {
		return 0.0
	}
	cited := toSet(citedIDs)
	found := 0
	for _, id := range expected {
		if cited[id] {
			found++
		}
	}
	return float64(found) / float64(len(expected))
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
	if titleMatchesAnyKeyword(retrievedTitles[0], relevantTitles) {
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
	correct := 0
	for _, t := range citedTitles {
		if titleMatchesAnyKeyword(t, relevantTitles) {
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

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func truncate(ids []string, k int) []string {
	if k >= 0 && k < len(ids) {
		return ids[:k]
	}
	return ids
}
