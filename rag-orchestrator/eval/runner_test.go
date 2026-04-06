package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGoldenCases_ValidFile(t *testing.T) {
	cases, err := LoadGoldenCases("testdata/golden_cases.json")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(cases), 10)

	// Verify known failure case is present
	var found bool
	for _, c := range cases {
		if c.ID == "iran-oil-crisis-causal" {
			found = true
			assert.Equal(t, "イランの石油危機はなぜ起きた？", c.Query)
			assert.Equal(t, "causal_explanation", c.Expected.ExpectedIntent)
			assert.Equal(t, 800, c.Expected.MinAnswerLength)
			assert.True(t, c.Expected.RequiresCitations)
			break
		}
	}
	assert.True(t, found, "iran-oil-crisis-causal case not found")
}

func TestLoadGoldenCases_ConversationHistory(t *testing.T) {
	cases, err := LoadGoldenCases("testdata/golden_cases.json")
	require.NoError(t, err)

	for _, c := range cases {
		if c.ID == "iran-follow-up-developments" {
			assert.Len(t, c.ConversationHistory, 2)
			assert.Equal(t, "user", c.ConversationHistory[0].Role)
			return
		}
	}
	t.Fatal("iran-follow-up-developments case not found")
}

func TestLoadGoldenCases_FileNotFound(t *testing.T) {
	_, err := LoadGoldenCases("testdata/nonexistent.json")
	assert.Error(t, err)
}

func TestRunOfflineEval_BaselineKnownFailures(t *testing.T) {
	cases, err := LoadGoldenCases("testdata/golden_cases.json")
	require.NoError(t, err)

	// Simulate baseline results where known failures reproduce
	results := map[string]EvalResult{
		"iran-oil-crisis-causal": {
			CaseID:           "iran-oil-crisis-causal",
			RetrievedTitles:  []string{"Asset Tokenization", "LibreFang"},
			BM25HitCount:     0,
			IntentClassified: "causal_explanation",
			Answer:           "イランの石油危機は発生しました。",
			AnswerLength:     14,
			CitationCount:    0,
			IsFallback:       false,
		},
		"iran-follow-up-developments": {
			CaseID:           "iran-follow-up-developments",
			RetrievedTitles:  []string{"Vague article"},
			IntentClassified: "general",
			Answer:           "不明です。",
			AnswerLength:     5,
			CitationCount:    0,
			IsFallback:       false,
		},
	}

	report := RunOfflineEval(cases, results)

	// Known failures should fail
	for _, v := range report.Verdicts {
		if v.CaseID == "iran-oil-crisis-causal" {
			assert.False(t, v.Passed, "iran-oil-crisis should fail in baseline")
			assert.NotEmpty(t, v.Failures)
		}
		if v.CaseID == "iran-follow-up-developments" {
			assert.False(t, v.Passed, "iran-follow-up should fail in baseline")
		}
	}

	// Report should have non-zero fail count
	assert.Greater(t, report.FailCount, 0)

	// BM25 zero rate should reflect the 0-hit case
	assert.Greater(t, report.Metrics.BM25ZeroRate, 0.0)
}

func TestRunOfflineEval_MissingResults(t *testing.T) {
	cases := []GoldenCase{
		{ID: "test-1", Query: "test query", Expected: ExpectedBehavior{ShouldClarify: false}},
	}
	results := map[string]EvalResult{} // Empty results

	report := RunOfflineEval(cases, results)
	assert.Equal(t, 1, report.FailCount)
	assert.Equal(t, "no result found for case", report.Verdicts[0].Failures[0])
}

func TestSaveReport_WritesValidJSON(t *testing.T) {
	report := EvalReport{
		Timestamp: "2026-04-06T00:00:00Z",
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
