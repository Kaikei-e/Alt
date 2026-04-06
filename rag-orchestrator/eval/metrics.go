package eval

// VerifyCase checks a single EvalResult against its GoldenCase expectations.
// Returns a CaseVerdict with pass/fail and specific failure reasons.
func VerifyCase(gc GoldenCase, result EvalResult) CaseVerdict {
	panic("not implemented")
}

// RecallAtK computes recall@K: fraction of expected relevant items found in top-K retrieved.
// relevantTitles: ground-truth relevant titles.
// retrievedTitles: ordered list of retrieved titles (position 0 = rank 1).
func RecallAtK(relevantTitles []string, retrievedTitles []string, k int) float64 {
	panic("not implemented")
}

// NDCGAtK computes nDCG@K (Normalized Discounted Cumulative Gain).
// relevanceScores maps title -> relevance grade (e.g. 0, 1, 2).
// retrievedTitles: ordered list of retrieved titles.
func NDCGAtK(relevanceScores map[string]int, retrievedTitles []string, k int) float64 {
	panic("not implemented")
}

// Top1Precision returns 1.0 if the top-1 retrieved title is relevant, 0.0 otherwise.
func Top1Precision(relevantTitles []string, retrievedTitles []string) float64 {
	panic("not implemented")
}

// Faithfulness estimates what fraction of answer claims are supported by context.
// This is a simplified heuristic: checks if expected entities appear in both
// the answer and the retrieved context chunks.
func Faithfulness(answer string, contextChunks []string, expectedEntities []string) float64 {
	panic("not implemented")
}

// CitationCorrectness checks what fraction of cited titles are in the relevant set.
func CitationCorrectness(citedTitles []string, relevantTitles []string) float64 {
	panic("not implemented")
}

// ContainsIrrelevant checks if any irrelevant titles appear in retrieved results.
func ContainsIrrelevant(retrievedTitles []string, irrelevantTitles []string) []string {
	panic("not implemented")
}
