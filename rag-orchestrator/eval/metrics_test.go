package eval

import (
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
		"Iran oil crisis":      2,
		"Iran sanctions":       1,
		"Unrelated":            0,
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
