package eval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- RecallAtK ---

func TestRecallAtK_AllRelevantFound(t *testing.T) {
	relevant := []string{"Example Systems outage", "Example Systems recall impact"}
	retrieved := []string{"Example Systems outage", "Example Systems recall impact", "Unrelated article"}
	got := RecallAtK(relevant, retrieved, 3)
	assert.Equal(t, 1.0, got)
}

func TestRecallAtK_PartialRelevant(t *testing.T) {
	relevant := []string{"Example Systems outage", "Example Systems recall impact"}
	retrieved := []string{"Orchard Gardening Weekly", "Example Systems outage", "Example Protocol Digest"}
	got := RecallAtK(relevant, retrieved, 3)
	assert.Equal(t, 0.5, got)
}

func TestRecallAtK_NoneRelevant(t *testing.T) {
	relevant := []string{"Example Systems outage"}
	retrieved := []string{"Orchard Gardening Weekly", "Example Protocol Digest", "Meadow Pricing Roundup"}
	got := RecallAtK(relevant, retrieved, 3)
	assert.Equal(t, 0.0, got)
}

func TestRecallAtK_KSmallerThanRetrieved(t *testing.T) {
	relevant := []string{"Example Systems outage", "Example Systems recall impact"}
	retrieved := []string{"Unrelated", "Example Systems outage", "Example Systems recall impact"}
	// At K=1, only "Unrelated" is checked
	got := RecallAtK(relevant, retrieved, 1)
	assert.Equal(t, 0.0, got)
}

func TestRecallAtK_EmptyRelevant(t *testing.T) {
	got := RecallAtK([]string{}, []string{"anything"}, 5)
	assert.Equal(t, 0.0, got)
}

// --- NDCGAtK ---

func TestNDCGAtK_PerfectRanking(t *testing.T) {
	relevance := map[string]int{
		"Example Systems outage": 2,
		"Example Systems recall": 1,
		"Unrelated":              0,
	}
	retrieved := []string{"Example Systems outage", "Example Systems recall", "Unrelated"}
	got := NDCGAtK(relevance, retrieved, 3)
	assert.InDelta(t, 1.0, got, 0.001)
}

func TestNDCGAtK_ReversedRanking(t *testing.T) {
	relevance := map[string]int{
		"Example Systems outage": 2,
		"Example Systems recall": 1,
		"Unrelated":              0,
	}
	retrieved := []string{"Unrelated", "Example Systems recall", "Example Systems outage"}
	got := NDCGAtK(relevance, retrieved, 3)
	// DCG = 0/log2(2) + 1/log2(3) + 2/log2(4) = 0 + 0.631 + 1.0 = 1.631
	// IDCG = 2/log2(2) + 1/log2(3) + 0/log2(4) = 2.0 + 0.631 + 0 = 2.631
	// nDCG = 1.631 / 2.631 ≈ 0.620
	assert.InDelta(t, 0.620, got, 0.01)
}

func TestNDCGAtK_EmptyRetrieved(t *testing.T) {
	relevance := map[string]int{"Example Systems outage": 2}
	got := NDCGAtK(relevance, []string{}, 10)
	assert.Equal(t, 0.0, got)
}

func TestNDCGAtK_NoRelevantDocs(t *testing.T) {
	relevance := map[string]int{}
	got := NDCGAtK(relevance, []string{"A", "B"}, 2)
	assert.Equal(t, 0.0, got)
}

// --- Top1Precision ---

func TestTop1Precision_Relevant(t *testing.T) {
	got := Top1Precision(
		[]string{"Example Systems outage", "Example Systems recall"},
		[]string{"Example Systems outage", "Unrelated"},
	)
	assert.Equal(t, 1.0, got)
}

func TestTop1Precision_Irrelevant(t *testing.T) {
	got := Top1Precision(
		[]string{"Example Systems outage"},
		[]string{"Orchard Gardening Weekly", "Example Systems outage"},
	)
	assert.Equal(t, 0.0, got)
}

func TestTop1Precision_EmptyRetrieved(t *testing.T) {
	got := Top1Precision([]string{"Example Systems outage"}, []string{})
	assert.Equal(t, 0.0, got)
}

// --- Faithfulness ---

func TestFaithfulness_AllEntitiesInBoth(t *testing.T) {
	answer := "エグザンプル社の供給危機は規制により発生した"
	chunks := []string{"エグザンプル社に対する新規制が供給網を停止させた"}
	entities := []string{"エグザンプル", "供給", "規制"}
	got := Faithfulness(answer, chunks, entities)
	assert.Equal(t, 1.0, got)
}

func TestFaithfulness_EntityInAnswerButNotContext(t *testing.T) {
	answer := "エグザンプル社の供給危機は規制により発生した"
	chunks := []string{"Orchard Gardening Weekly is a growing trend"}
	entities := []string{"エグザンプル", "供給", "規制"}
	got := Faithfulness(answer, chunks, entities)
	assert.Equal(t, 0.0, got)
}

func TestFaithfulness_PartialSupport(t *testing.T) {
	answer := "エグザンプル社の供給危機は規制と地政学的要因による"
	chunks := []string{"エグザンプル社に対する規制が強化された"}
	entities := []string{"エグザンプル", "規制", "地政学"}
	// "エグザンプル" and "規制" are in both, "地政学" is in answer but not context
	got := Faithfulness(answer, chunks, entities)
	assert.InDelta(t, 2.0/3.0, got, 0.01)
}

func TestFaithfulness_EmptyEntities(t *testing.T) {
	got := Faithfulness("some answer", []string{"some context"}, []string{})
	assert.Equal(t, 0.0, got)
}

// --- CitationCorrectness ---

func TestCitationCorrectness_AllCitedRelevant(t *testing.T) {
	cited := []string{"Example Systems outage", "Example Systems recall"}
	relevant := []string{"Example Systems outage", "Example Systems recall", "Example Systems earnings"}
	got := CitationCorrectness(cited, relevant)
	assert.Equal(t, 1.0, got)
}

func TestCitationCorrectness_NoneCitedRelevant(t *testing.T) {
	cited := []string{"Orchard Gardening Weekly", "Example Protocol Digest"}
	relevant := []string{"Example Systems outage"}
	got := CitationCorrectness(cited, relevant)
	assert.Equal(t, 0.0, got)
}

func TestCitationCorrectness_Partial(t *testing.T) {
	cited := []string{"Example Systems outage", "Orchard Gardening Weekly"}
	relevant := []string{"Example Systems outage", "Example Systems recall"}
	got := CitationCorrectness(cited, relevant)
	assert.Equal(t, 0.5, got)
}

func TestCitationCorrectness_EmptyCited(t *testing.T) {
	got := CitationCorrectness([]string{}, []string{"Example Systems outage"})
	assert.Equal(t, 0.0, got)
}

// --- ContainsIrrelevant ---

func TestContainsIrrelevant_NoIrrelevant(t *testing.T) {
	retrieved := []string{"Example Systems outage", "Example Systems recall"}
	irrelevant := []string{"Orchard Gardening Weekly", "Example Protocol Digest"}
	got := ContainsIrrelevant(retrieved, irrelevant)
	assert.Empty(t, got)
}

func TestContainsIrrelevant_HasIrrelevant(t *testing.T) {
	retrieved := []string{"Example Systems outage", "Orchard Gardening Weekly", "Example Protocol Digest"}
	irrelevant := []string{"Orchard Gardening Weekly", "Example Protocol Digest"}
	got := ContainsIrrelevant(retrieved, irrelevant)
	assert.ElementsMatch(t, []string{"Orchard Gardening Weekly", "Example Protocol Digest"}, got)
}

// --- VerifyCase ---

func TestVerifyCase_DriftedRetrieval_Baseline_Fails(t *testing.T) {
	gc := GoldenCase{
		ID:    "supply-crisis-causal",
		Query: "エグザンプル社の供給危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedTopicKeywords: []string{"エグザンプル", "供給"},
			RetrievalScope:        "global",
			MinRelevantContexts:   2,
			IrrelevantTitles:      []string{"Orchard Gardening Weekly", "Example Protocol Digest"},
			ShouldClarify:         false,
			ExpectedIntent:        "causal_explanation",
			MinAnswerLength:       800,
			RequiresCitations:     true,
			ExpectedEntities:      []string{"エグザンプル", "供給", "規制"},
		},
	}

	// Simulate a baseline run where retrieval drifts off-topic
	result := EvalResult{
		CaseID:           "supply-crisis-causal",
		RetrievedTitles:  []string{"Orchard Gardening Weekly", "Example Protocol Digest"},
		BM25HitCount:     0,
		IntentClassified: "causal_explanation",
		Answer:           "エグザンプル社の供給危機は発生しました。",
		AnswerLength:     18,
		CitationCount:    0,
		CitedTitles:      []string{},
		IsFallback:       false,
	}

	verdict := VerifyCase(gc, result)
	assert.False(t, verdict.Passed)
	assert.NotEmpty(t, verdict.Failures)
	// Should fail on: irrelevant titles found, too short, no citations, min relevant contexts
}

func TestVerifyCase_FollowUpDrift_Baseline_Fails(t *testing.T) {
	gc := GoldenCase{
		ID:    "supply-follow-up-reference",
		Query: "では、それに関連するエグザンプル社の動向は？",
		ConversationHistory: []HistoryMessage{
			{Role: "user", Content: "最近の供給危機の真因は？"},
			{Role: "assistant", Content: "供給危機は規制と地政学的緊張が原因..."},
		},
		Expected: ExpectedBehavior{
			ExpectedTopicKeywords: []string{"エグザンプル"},
			RetrievalScope:        "global",
			ShouldClarify:         false,
			MinAnswerLength:       300,
			RequiresCitations:     true,
			ExpectedEntities:      []string{"エグザンプル"},
		},
	}

	result := EvalResult{
		CaseID:           "supply-follow-up-reference",
		RetrievedTitles:  []string{"Vague article about logistics"},
		IntentClassified: "general", // Misclassified
		Answer:           "エグザンプル社の動向は不明です。",
		AnswerLength:     14,
		CitationCount:    0,
		IsFallback:       false,
	}

	verdict := VerifyCase(gc, result)
	assert.False(t, verdict.Passed)
}

// --- ExpectedStructure ---

func TestVerifyCase_ExpectedStructure_AllPresent(t *testing.T) {
	gc := GoldenCase{
		ID:    "causal-structure-pass",
		Query: "エグザンプル社の供給危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedIntent:    "causal_explanation",
			ExpectedStructure: []string{"直接的要因", "構造的背景", "不確実性"},
			MinAnswerLength:   10,
		},
	}

	result := EvalResult{
		CaseID:           "causal-structure-pass",
		IntentClassified: "causal_explanation",
		Answer:           "**直接的要因**\n規制が原因...\n\n**構造的背景**\n長期的な対立...\n\n**不確実性**\n一部情報が不足...",
		AnswerLength:     50,
	}

	verdict := VerifyCase(gc, result)
	assert.True(t, verdict.Passed, "all expected structures present: %v", verdict.Failures)
}

func TestVerifyCase_ExpectedStructure_Missing(t *testing.T) {
	gc := GoldenCase{
		ID:    "causal-structure-fail",
		Query: "エグザンプル社の供給危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedIntent:    "causal_explanation",
			ExpectedStructure: []string{"直接的要因", "構造的背景", "不確実性"},
		},
	}

	result := EvalResult{
		CaseID:           "causal-structure-fail",
		IntentClassified: "causal_explanation",
		Answer:           "エグザンプル社の供給危機は規制が原因です。",
		AnswerLength:     19,
	}

	verdict := VerifyCase(gc, result)
	assert.False(t, verdict.Passed)
	// Should report missing structures
	hasStructureFailure := false
	for _, f := range verdict.Failures {
		if strings.Contains(f, "expected structure") {
			hasStructureFailure = true
			break
		}
	}
	assert.True(t, hasStructureFailure, "should report missing structure, got: %v", verdict.Failures)
}

func TestVerifyCase_ExpectedStructure_Partial(t *testing.T) {
	gc := GoldenCase{
		ID:    "causal-structure-partial",
		Query: "エグザンプル社の供給危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedStructure: []string{"直接的要因", "構造的背景", "不確実性"},
		},
	}

	result := EvalResult{
		CaseID:       "causal-structure-partial",
		Answer:       "**直接的要因**\n規制が原因...",
		AnswerLength: 15,
	}

	verdict := VerifyCase(gc, result)
	assert.False(t, verdict.Passed)
	// Should fail for missing "構造的背景" and "不確実性"
	structureFailures := 0
	for _, f := range verdict.Failures {
		if strings.Contains(f, "expected structure") {
			structureFailures++
		}
	}
	assert.Equal(t, 2, structureFailures, "should report 2 missing structures")
}

// --- InstructionAdherenceRate and MeanPromptTokens in AggregateMetrics ---

func TestRunOfflineEval_InstructionAdherence(t *testing.T) {
	cases := []GoldenCase{
		{
			ID:    "case-1",
			Query: "Q1",
			Expected: ExpectedBehavior{
				ExpectedStructure: []string{"概要", "詳細"},
			},
		},
		{
			ID:    "case-2",
			Query: "Q2",
			Expected: ExpectedBehavior{
				ExpectedStructure: []string{"概要", "詳細"},
			},
		},
	}
	results := map[string]EvalResult{
		"case-1": {
			CaseID:           "case-1",
			Answer:           "## 概要\ntest\n## 詳細\ntest",
			AnswerLength:     20,
			PromptTokenCount: 800,
		},
		"case-2": {
			CaseID:           "case-2",
			Answer:           "simple answer without structure",
			AnswerLength:     30,
			PromptTokenCount: 1200,
		},
	}

	report := RunOfflineEval(cases, results)
	// case-1 adheres (has both 概要 and 詳細), case-2 does not
	assert.InDelta(t, 0.5, report.Metrics.InstructionAdherenceRate, 0.01)
	// Mean prompt tokens = (800 + 1200) / 2 = 1000
	assert.InDelta(t, 1000.0, report.Metrics.MeanPromptTokens, 0.01)
}

func TestRunOfflineEval_NoStructureExpected(t *testing.T) {
	cases := []GoldenCase{
		{
			ID:       "no-structure",
			Query:    "Q1",
			Expected: ExpectedBehavior{
				// No ExpectedStructure
			},
		},
	}
	results := map[string]EvalResult{
		"no-structure": {
			CaseID:           "no-structure",
			Answer:           "simple answer",
			AnswerLength:     10,
			PromptTokenCount: 500,
		},
	}

	report := RunOfflineEval(cases, results)
	// No structure expectations → adherence rate should be 0 (no denominator)
	assert.Equal(t, 0.0, report.Metrics.InstructionAdherenceRate)
	assert.InDelta(t, 500.0, report.Metrics.MeanPromptTokens, 0.01)
}

func TestVerifyCase_ClarificationExpected_Passes(t *testing.T) {
	gc := GoldenCase{
		ID:    "ambiguous-follow-up",
		Query: "もっと詳しく",
		ConversationHistory: []HistoryMessage{
			{Role: "user", Content: "エグザンプル社の供給危機は？"},
			{Role: "assistant", Content: "エグザンプル社の供給危機は規制が原因です。"},
		},
		Expected: ExpectedBehavior{
			ShouldClarify: true,
		},
	}

	result := EvalResult{
		CaseID:             "ambiguous-follow-up",
		ClarificationAsked: true,
	}

	verdict := VerifyCase(gc, result)
	assert.True(t, verdict.Passed)
}
