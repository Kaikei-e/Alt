package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- article-id based retrieval metrics ---

func TestRecallAtKByID(t *testing.T) {
	tests := []struct {
		name      string
		relevant  []string
		retrieved []string
		k         int
		want      float64
	}{
		{name: "all found", relevant: []string{"a", "b"}, retrieved: []string{"a", "b", "z"}, k: 20, want: 1.0},
		{name: "half found", relevant: []string{"a", "b"}, retrieved: []string{"z", "a"}, k: 20, want: 0.5},
		{name: "cut off by k", relevant: []string{"a", "b"}, retrieved: []string{"z", "a", "b"}, k: 1, want: 0.0},
		{name: "no relevant ids", relevant: nil, retrieved: []string{"a"}, k: 20, want: 0.0},
		{name: "duplicate retrieval counts once", relevant: []string{"a"}, retrieved: []string{"a", "a"}, k: 20, want: 1.0},
		{name: "prefix is not a match", relevant: []string{"abcd"}, retrieved: []string{"ab"}, k: 20, want: 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, RecallAtKByID(tt.relevant, tt.retrieved, tt.k), 0.0001)
		})
	}
}

func TestReciprocalRankByID(t *testing.T) {
	tests := []struct {
		name      string
		relevant  []string
		retrieved []string
		want      float64
	}{
		{name: "first hit", relevant: []string{"a"}, retrieved: []string{"a", "b"}, want: 1.0},
		{name: "third hit", relevant: []string{"c"}, retrieved: []string{"a", "b", "c"}, want: 1.0 / 3.0},
		{name: "no hit", relevant: []string{"z"}, retrieved: []string{"a", "b"}, want: 0.0},
		{name: "empty retrieval", relevant: []string{"a"}, retrieved: nil, want: 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, ReciprocalRankByID(tt.relevant, tt.retrieved), 0.0001)
		})
	}
}

func TestExpectedBehavior_RelevanceGrades(t *testing.T) {
	e := ExpectedBehavior{
		RelevantArticleIDs:         []string{"a", "b", "c"},
		ExpectedCitationArticleIDs: []string{"a"},
	}
	grades := e.RelevanceGrades()
	assert.Equal(t, 2, grades["a"], "must-cite articles outrank merely relevant ones")
	assert.Equal(t, 1, grades["b"])
	assert.Equal(t, 1, grades["c"])
	assert.Len(t, grades, 3)
}

func TestExpectedBehavior_RelevanceGradesIncludesCitationOnlyIDs(t *testing.T) {
	e := ExpectedBehavior{ExpectedCitationArticleIDs: []string{"a"}}
	grades := e.RelevanceGrades()
	assert.Equal(t, map[string]int{"a": 2}, grades)
}

func TestRerankGain(t *testing.T) {
	grades := map[string]int{"a": 2, "b": 1}

	// Reranking lifts "a" from last place to first.
	pre := []string{"z", "b", "a"}
	post := []string{"a", "b", "z"}
	gain := RerankGain(grades, pre, post, 10)
	assert.Greater(t, gain, 0.0)

	// Reranking that reverses a good order must be reported as a loss.
	assert.Less(t, RerankGain(grades, post, pre, 10), 0.0)

	// No reordering means no gain.
	assert.InDelta(t, 0.0, RerankGain(grades, post, post, 10), 0.0001)
}

func TestForbiddenHits(t *testing.T) {
	got := ForbiddenHits([]string{"a", "spam-1", "b", "spam-2"}, []string{"spam-1", "spam-2", "spam-3"})
	assert.Equal(t, []string{"spam-1", "spam-2"}, got)
	assert.Empty(t, ForbiddenHits([]string{"a"}, []string{"spam-1"}))
	assert.Empty(t, ForbiddenHits(nil, nil))
}

func TestCitationRecallByID(t *testing.T) {
	assert.InDelta(t, 1.0, CitationRecallByID([]string{"a"}, []string{"a", "b"}), 0.0001)
	assert.InDelta(t, 0.5, CitationRecallByID([]string{"a", "b"}, []string{"b"}), 0.0001)
	assert.InDelta(t, 0.0, CitationRecallByID([]string{"a"}, nil), 0.0001)
	assert.InDelta(t, 0.0, CitationRecallByID(nil, []string{"a"}), 0.0001)
}

// --- VerifyCase: article-level expectations ---

func TestVerifyCase_ForbiddenArticleCited(t *testing.T) {
	gc := GoldenCase{
		ID: "forbidden",
		Expected: ExpectedBehavior{
			RequiresCitations:   true,
			ForbiddenArticleIDs: []string{"spam-1"},
		},
	}
	v := VerifyCase(gc, EvalResult{
		CaseID:              "forbidden",
		CitationCount:       1,
		CitedArticleIDs:     []string{"spam-1"},
		RetrievedArticleIDs: []string{"spam-1"},
		Answer:              "answer",
	})
	assert.False(t, v.Passed)
	assert.NotEmpty(t, v.Failures)
}

func TestVerifyCase_ExpectedCitationMissing(t *testing.T) {
	gc := GoldenCase{
		ID: "must-cite",
		Expected: ExpectedBehavior{
			RequiresCitations:          true,
			ExpectedCitationArticleIDs: []string{"target"},
		},
	}
	v := VerifyCase(gc, EvalResult{
		CaseID:          "must-cite",
		CitationCount:   1,
		CitedArticleIDs: []string{"other"},
		Answer:          "answer",
	})
	assert.False(t, v.Passed)

	v = VerifyCase(gc, EvalResult{
		CaseID:          "must-cite",
		CitationCount:   1,
		CitedArticleIDs: []string{"target"},
		Answer:          "answer",
	})
	assert.True(t, v.Passed, "failures: %v", v.Failures)
}

func TestVerifyCase_MinExpectedRecall(t *testing.T) {
	gc := GoldenCase{
		ID: "recall",
		Expected: ExpectedBehavior{
			RelevantArticleIDs: []string{"a", "b", "c", "d"},
			MinExpectedRecall:  0.5,
		},
	}
	v := VerifyCase(gc, EvalResult{CaseID: "recall", RetrievedArticleIDs: []string{"a", "z"}})
	assert.False(t, v.Passed)

	v = VerifyCase(gc, EvalResult{CaseID: "recall", RetrievedArticleIDs: []string{"a", "b", "z"}})
	assert.True(t, v.Passed, "failures: %v", v.Failures)
}

func TestVerifyCase_NoAnswerExpectsSilence(t *testing.T) {
	gc := GoldenCase{
		ID:       "no-answer",
		Expected: ExpectedBehavior{ExpectNoAnswer: true},
	}
	v := VerifyCase(gc, EvalResult{CaseID: "no-answer", CitationCount: 3, CitedArticleIDs: []string{"a", "b", "c"}})
	assert.False(t, v.Passed, "fabricated citations must fail an expected no-answer case")

	v = VerifyCase(gc, EvalResult{CaseID: "no-answer", Answer: "該当する記事は見つかりませんでした。"})
	assert.True(t, v.Passed, "failures: %v", v.Failures)
}

// --- stage aggregation ---

func TestRunOfflineEval_StageMetrics(t *testing.T) {
	cases := []GoldenCase{
		{
			ID:       "retrieval-hit",
			Category: CategoryCrossLingual,
			Expected: ExpectedBehavior{
				RelevantArticleIDs:         []string{"a", "b"},
				ExpectedCitationArticleIDs: []string{"a"},
				RequiresCitations:          true,
			},
		},
		{
			ID:       "retrieval-miss",
			Category: CategoryRecallMiss,
			Expected: ExpectedBehavior{
				RelevantArticleIDs:         []string{"x"},
				ExpectedCitationArticleIDs: []string{"x"},
				RequiresCitations:          true,
			},
		},
	}

	results := map[string]EvalResult{
		"retrieval-hit": {
			CaseID:              "retrieval-hit",
			RetrievedArticleIDs: []string{"a", "b", "z"},
			PreRerankArticleIDs: []string{"z", "b", "a"},
			RerankApplied:       true,
			CitedArticleIDs:     []string{"a"},
			CitationCount:       1,
			Answer:              "grounded answer",
			BM25HitCount:        4,
		},
		"retrieval-miss": {
			CaseID:              "retrieval-miss",
			RetrievedArticleIDs: []string{"q", "r"},
			PreRerankArticleIDs: []string{"q", "r"},
			CitedArticleIDs:     []string{"q"},
			CitationCount:       1,
			Answer:              "drifted answer",
			BM25HitCount:        0,
		},
	}

	report := RunOfflineEval(cases, results)

	assert.Equal(t, 2, report.Stages.Retrieval.CaseCount)
	assert.InDelta(t, 0.5, report.Stages.Retrieval.MeanRecallAt20, 0.0001)
	assert.InDelta(t, 0.5, report.Stages.Retrieval.MeanMRR, 0.0001)
	assert.Greater(t, report.Stages.Retrieval.MeanNDCGAt10, 0.0)
	assert.InDelta(t, 0.5, report.Stages.Retrieval.BM25ZeroRate, 0.0001)

	assert.Equal(t, 1, report.Stages.Rerank.CaseCount, "only cases with rerank applied count")
	assert.Greater(t, report.Stages.Rerank.MeanNDCGAt10Delta, 0.0)

	assert.InDelta(t, 0.5, report.Stages.Generation.CitationRecall, 0.0001)
	assert.Equal(t, 2, report.Stages.Generation.CaseCount)

	// Legacy aggregate stays populated so old baseline reports remain comparable.
	assert.Equal(t, 2, report.CaseCount)
	assert.InDelta(t, 0.5, report.Metrics.BM25ZeroRate, 0.0001)
}

func TestRunOfflineEval_CategoryBreakdown(t *testing.T) {
	cases := []GoldenCase{
		{ID: "c1", Category: CategoryCrossLingual, Expected: ExpectedBehavior{}},
		{ID: "c2", Category: CategoryCrossLingual, Expected: ExpectedBehavior{RequiresCitations: true}},
		{ID: "c3", Category: CategoryNoAnswer, Expected: ExpectedBehavior{ExpectNoAnswer: true}},
	}
	results := map[string]EvalResult{
		"c1": {CaseID: "c1", Answer: "ok"},
		"c2": {CaseID: "c2", Answer: "ok"}, // no citations -> fails
		"c3": {CaseID: "c3", Answer: "見つかりませんでした"},
	}

	report := RunOfflineEval(cases, results)
	require.Contains(t, report.Categories, CategoryCrossLingual)
	assert.Equal(t, 2, report.Categories[CategoryCrossLingual].CaseCount)
	assert.Equal(t, 1, report.Categories[CategoryCrossLingual].PassCount)
	assert.Equal(t, 1, report.Categories[CategoryNoAnswer].PassCount)
}

func TestGoldenCase_IsCrossLingual(t *testing.T) {
	assert.True(t, GoldenCase{Language: LanguagePair{Query: "ja", Corpus: "en"}}.IsCrossLingual())
	assert.False(t, GoldenCase{Language: LanguagePair{Query: "ja", Corpus: "ja"}}.IsCrossLingual())
	assert.False(t, GoldenCase{}.IsCrossLingual())
}
