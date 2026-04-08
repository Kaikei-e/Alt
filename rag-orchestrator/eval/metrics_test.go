package eval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- RecallAtK ---

func TestRecallAtK_AllRelevantFound(t *testing.T) {
	relevant := []string{"Iran oil crisis", "Iran sanctions impact"}
	retrieved := []string{"Iran oil crisis", "Iran sanctions impact", "Unrelated article"}
	got := RecallAtK(relevant, retrieved, 3)
	assert.Equal(t, 1.0, got)
}

func TestRecallAtK_PartialRelevant(t *testing.T) {
	relevant := []string{"Iran oil crisis", "Iran sanctions impact"}
	retrieved := []string{"Asset Tokenization", "Iran oil crisis", "LibreFang"}
	got := RecallAtK(relevant, retrieved, 3)
	assert.Equal(t, 0.5, got)
}

func TestRecallAtK_NoneRelevant(t *testing.T) {
	relevant := []string{"Iran oil crisis"}
	retrieved := []string{"Asset Tokenization", "LibreFang", "AI Model Pricing"}
	got := RecallAtK(relevant, retrieved, 3)
	assert.Equal(t, 0.0, got)
}

func TestRecallAtK_KSmallerThanRetrieved(t *testing.T) {
	relevant := []string{"Iran oil crisis", "Iran sanctions impact"}
	retrieved := []string{"Unrelated", "Iran oil crisis", "Iran sanctions impact"}
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
		"Iran oil crisis": 2,
		"Iran sanctions":  1,
		"Unrelated":       0,
	}
	retrieved := []string{"Iran oil crisis", "Iran sanctions", "Unrelated"}
	got := NDCGAtK(relevance, retrieved, 3)
	assert.InDelta(t, 1.0, got, 0.001)
}

func TestNDCGAtK_ReversedRanking(t *testing.T) {
	relevance := map[string]int{
		"Iran oil crisis": 2,
		"Iran sanctions":  1,
		"Unrelated":       0,
	}
	retrieved := []string{"Unrelated", "Iran sanctions", "Iran oil crisis"}
	got := NDCGAtK(relevance, retrieved, 3)
	// DCG = 0/log2(2) + 1/log2(3) + 2/log2(4) = 0 + 0.631 + 1.0 = 1.631
	// IDCG = 2/log2(2) + 1/log2(3) + 0/log2(4) = 2.0 + 0.631 + 0 = 2.631
	// nDCG = 1.631 / 2.631 ≈ 0.620
	assert.InDelta(t, 0.620, got, 0.01)
}

func TestNDCGAtK_EmptyRetrieved(t *testing.T) {
	relevance := map[string]int{"Iran oil crisis": 2}
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
		[]string{"Iran oil crisis", "Iran sanctions"},
		[]string{"Iran oil crisis", "Unrelated"},
	)
	assert.Equal(t, 1.0, got)
}

func TestTop1Precision_Irrelevant(t *testing.T) {
	got := Top1Precision(
		[]string{"Iran oil crisis"},
		[]string{"Asset Tokenization", "Iran oil crisis"},
	)
	assert.Equal(t, 0.0, got)
}

func TestTop1Precision_EmptyRetrieved(t *testing.T) {
	got := Top1Precision([]string{"Iran oil crisis"}, []string{})
	assert.Equal(t, 0.0, got)
}

// --- Faithfulness ---

func TestFaithfulness_AllEntitiesInBoth(t *testing.T) {
	answer := "イランの石油危機は制裁により発生した"
	chunks := []string{"イランに対する経済制裁が石油輸出を停止させた"}
	entities := []string{"イラン", "石油", "制裁"}
	got := Faithfulness(answer, chunks, entities)
	assert.Equal(t, 1.0, got)
}

func TestFaithfulness_EntityInAnswerButNotContext(t *testing.T) {
	answer := "イランの石油危機は制裁により発生した"
	chunks := []string{"Asset Tokenization is a growing trend"}
	entities := []string{"イラン", "石油", "制裁"}
	got := Faithfulness(answer, chunks, entities)
	assert.Equal(t, 0.0, got)
}

func TestFaithfulness_PartialSupport(t *testing.T) {
	answer := "イランの石油危機は制裁と地政学的要因による"
	chunks := []string{"イランに対する制裁が強化された"}
	entities := []string{"イラン", "制裁", "地政学"}
	// "イラン" and "制裁" are in both, "地政学" is in answer but not context
	got := Faithfulness(answer, chunks, entities)
	assert.InDelta(t, 2.0/3.0, got, 0.01)
}

func TestFaithfulness_EmptyEntities(t *testing.T) {
	got := Faithfulness("some answer", []string{"some context"}, []string{})
	assert.Equal(t, 0.0, got)
}

// --- CitationCorrectness ---

func TestCitationCorrectness_AllCitedRelevant(t *testing.T) {
	cited := []string{"Iran oil crisis", "Iran sanctions"}
	relevant := []string{"Iran oil crisis", "Iran sanctions", "Iran economy"}
	got := CitationCorrectness(cited, relevant)
	assert.Equal(t, 1.0, got)
}

func TestCitationCorrectness_NoneCitedRelevant(t *testing.T) {
	cited := []string{"Asset Tokenization", "LibreFang"}
	relevant := []string{"Iran oil crisis"}
	got := CitationCorrectness(cited, relevant)
	assert.Equal(t, 0.0, got)
}

func TestCitationCorrectness_Partial(t *testing.T) {
	cited := []string{"Iran oil crisis", "Asset Tokenization"}
	relevant := []string{"Iran oil crisis", "Iran sanctions"}
	got := CitationCorrectness(cited, relevant)
	assert.Equal(t, 0.5, got)
}

func TestCitationCorrectness_EmptyCited(t *testing.T) {
	got := CitationCorrectness([]string{}, []string{"Iran oil crisis"})
	assert.Equal(t, 0.0, got)
}

// --- ContainsIrrelevant ---

func TestContainsIrrelevant_NoIrrelevant(t *testing.T) {
	retrieved := []string{"Iran oil crisis", "Iran sanctions"}
	irrelevant := []string{"Asset Tokenization", "LibreFang"}
	got := ContainsIrrelevant(retrieved, irrelevant)
	assert.Empty(t, got)
}

func TestContainsIrrelevant_HasIrrelevant(t *testing.T) {
	retrieved := []string{"Iran oil crisis", "Asset Tokenization", "LibreFang"}
	irrelevant := []string{"Asset Tokenization", "LibreFang"}
	got := ContainsIrrelevant(retrieved, irrelevant)
	assert.ElementsMatch(t, []string{"Asset Tokenization", "LibreFang"}, got)
}

// --- VerifyCase ---

func TestVerifyCase_IranOilCrisis_Baseline_Fails(t *testing.T) {
	gc := GoldenCase{
		ID:    "iran-oil-crisis-causal",
		Query: "イランの石油危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedTopicKeywords: []string{"イラン", "石油"},
			RetrievalScope:        "global",
			MinRelevantContexts:   2,
			IrrelevantTitles:      []string{"Asset Tokenization", "LibreFang"},
			ShouldClarify:         false,
			ExpectedIntent:        "causal_explanation",
			MinAnswerLength:       800,
			RequiresCitations:     true,
			ExpectedEntities:      []string{"イラン", "石油", "制裁"},
		},
	}

	// Simulate the known baseline failure (2026-04-03)
	result := EvalResult{
		CaseID:           "iran-oil-crisis-causal",
		RetrievedTitles:  []string{"Asset Tokenization", "LibreFang"},
		BM25HitCount:     0,
		IntentClassified: "causal_explanation",
		Answer:           "イランの石油危機は発生しました。",
		AnswerLength:     14,
		CitationCount:    0,
		CitedTitles:      []string{},
		IsFallback:       false,
	}

	verdict := VerifyCase(gc, result)
	assert.False(t, verdict.Passed)
	assert.NotEmpty(t, verdict.Failures)
	// Should fail on: irrelevant titles found, too short, no citations, min relevant contexts
}

func TestVerifyCase_IranFollowUp_Baseline_Fails(t *testing.T) {
	gc := GoldenCase{
		ID:    "iran-follow-up-reference",
		Query: "では、それに関連するイランの動向は？",
		ConversationHistory: []HistoryMessage{
			{Role: "user", Content: "最近の石油危機の真因は？"},
			{Role: "assistant", Content: "石油危機は制裁と地政学的緊張が原因..."},
		},
		Expected: ExpectedBehavior{
			ExpectedTopicKeywords: []string{"イラン"},
			RetrievalScope:        "global",
			ShouldClarify:         false,
			MinAnswerLength:       300,
			RequiresCitations:     true,
			ExpectedEntities:      []string{"イラン"},
		},
	}

	result := EvalResult{
		CaseID:           "iran-follow-up-reference",
		RetrievedTitles:  []string{"Vague article about Middle East"},
		IntentClassified: "general", // Misclassified
		Answer:           "イランの動向は不明です。",
		AnswerLength:     11,
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
		Query: "イランの石油危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedIntent:    "causal_explanation",
			ExpectedStructure: []string{"直接的要因", "構造的背景", "不確実性"},
			MinAnswerLength:   10,
		},
	}

	result := EvalResult{
		CaseID:           "causal-structure-pass",
		IntentClassified: "causal_explanation",
		Answer:           "**直接的要因**\n制裁が原因...\n\n**構造的背景**\n長期的な対立...\n\n**不確実性**\n一部情報が不足...",
		AnswerLength:     50,
	}

	verdict := VerifyCase(gc, result)
	assert.True(t, verdict.Passed, "all expected structures present: %v", verdict.Failures)
}

func TestVerifyCase_ExpectedStructure_Missing(t *testing.T) {
	gc := GoldenCase{
		ID:    "causal-structure-fail",
		Query: "イランの石油危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedIntent:    "causal_explanation",
			ExpectedStructure: []string{"直接的要因", "構造的背景", "不確実性"},
		},
	}

	result := EvalResult{
		CaseID:           "causal-structure-fail",
		IntentClassified: "causal_explanation",
		Answer:           "イランの石油危機は制裁が原因です。",
		AnswerLength:     17,
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
		Query: "イランの石油危機はなぜ起きた？",
		Expected: ExpectedBehavior{
			ExpectedStructure: []string{"直接的要因", "構造的背景", "不確実性"},
		},
	}

	result := EvalResult{
		CaseID:       "causal-structure-partial",
		Answer:       "**直接的要因**\n制裁が原因...",
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
			ID:    "no-structure",
			Query: "Q1",
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
		ID:    "ambiguous-more-detail",
		Query: "もっと詳しく",
		ConversationHistory: []HistoryMessage{
			{Role: "user", Content: "イランの石油危機は？"},
			{Role: "assistant", Content: "イランの石油危機は制裁が原因です。"},
		},
		Expected: ExpectedBehavior{
			ShouldClarify: true,
		},
	}

	result := EvalResult{
		CaseID:             "ambiguous-more-detail",
		ClarificationAsked: true,
	}

	verdict := VerifyCase(gc, result)
	assert.True(t, verdict.Passed)
}
