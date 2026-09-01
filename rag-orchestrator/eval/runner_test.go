package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGoldenCases_ValidFile(t *testing.T) {
	cases, err := LoadGoldenCases(syntheticGoldenPath)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(cases), 10)

	var found bool
	for _, c := range cases {
		if c.ID == "recall-miss-vendor-acquisition" {
			found = true
			assert.Equal(t, CategoryRecallMiss, c.Category)
			assert.True(t, c.Expected.RequiresCitations)
			assert.NotEmpty(t, c.Expected.RelevantArticleIDs)
			break
		}
	}
	assert.True(t, found, "recall-miss-vendor-acquisition case not found")
}

func TestLoadGoldenCases_ConversationHistory(t *testing.T) {
	cases, err := LoadGoldenCases(syntheticGoldenPath)
	require.NoError(t, err)

	for _, c := range cases {
		if c.ID == "followup-ambiguous-more-detail" {
			assert.Len(t, c.ConversationHistory, 2)
			assert.Equal(t, "user", c.ConversationHistory[0].Role)
			assert.True(t, c.Expected.ShouldClarify)
			return
		}
	}
	t.Fatal("followup-ambiguous-more-detail case not found")
}

func TestLoadGoldenCases_FileNotFound(t *testing.T) {
	_, err := LoadGoldenCases("testdata/nonexistent.json")
	assert.Error(t, err)
}

func TestRunOfflineEval_BaselineKnownFailures(t *testing.T) {
	cases, err := LoadGoldenCases(syntheticGoldenPath)
	require.NoError(t, err)

	// A baseline run where retrieval drifts off-topic and no citation is emitted.
	results := map[string]EvalResult{
		"recall-miss-vendor-acquisition": {
			CaseID:              "recall-miss-vendor-acquisition",
			RetrievedTitles:     []string{"Unrelated A", "Unrelated B"},
			RetrievedArticleIDs: []string{"00000000-0000-4000-8000-0000000000ff"},
			BM25HitCount:        0,
			Answer:              "短い回答。",
			AnswerLength:        6,
			CitationCount:       0,
		},
		"followup-ambiguous-more-detail": {
			CaseID:             "followup-ambiguous-more-detail",
			Answer:             "曖昧なまま回答してしまった。",
			AnswerLength:       14,
			ClarificationAsked: false,
		},
	}

	report := RunOfflineEval(cases, results)

	for _, v := range report.Verdicts {
		switch v.CaseID {
		case "recall-miss-vendor-acquisition":
			assert.False(t, v.Passed, "drifted retrieval should fail")
			assert.NotEmpty(t, v.Failures)
		case "followup-ambiguous-more-detail":
			assert.False(t, v.Passed, "missing clarification should fail")
		}
	}

	assert.Greater(t, report.FailCount, 0)
	assert.Greater(t, report.Metrics.BM25ZeroRate, 0.0)
}

func TestRunOfflineEval_MissingResults(t *testing.T) {
	cases := []GoldenCase{
		{ID: "test-1", Query: "test query", Category: CategoryRegression, Expected: ExpectedBehavior{ShouldClarify: false}},
	}
	results := map[string]EvalResult{} // Empty results

	report := RunOfflineEval(cases, results)
	assert.Equal(t, 1, report.FailCount)
	assert.Equal(t, "no result found for case", report.Verdicts[0].Failures[0])
	assert.Equal(t, 1, report.Categories[CategoryRegression].CaseCount)
}

func TestSaveReport_WritesValidJSON(t *testing.T) {
	report := EvalReport{
		Timestamp: "2026-09-01T00:00:00Z",
		CaseCount: 1,
		PassCount: 0,
		FailCount: 1,
		Verdicts: []CaseVerdict{
			{CaseID: "test", Passed: false, Failures: []string{"test failure"}},
		},
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "report.json")

	err := SaveReport(report, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "test failure")
}
