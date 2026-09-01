package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleReport(profile string, recall, rerankDelta float64, verdicts []CaseVerdict) EvalReport {
	report := EvalReport{
		Timestamp: "2026-09-01T00:00:00Z",
		Profile:   ProfileSummary{Name: profile, EmbedderModel: "bge-m3", RerankEnabled: true},
		CaseCount: len(verdicts),
		Verdicts:  verdicts,
	}
	for _, v := range verdicts {
		if v.Passed {
			report.PassCount++
		} else {
			report.FailCount++
		}
	}
	report.Stages.Retrieval.MeanRecallAt20 = recall
	report.Stages.Rerank.MeanNDCGAt10Delta = rerankDelta
	return report
}

func TestComputeDiff_MetricsAndVerdictMoves(t *testing.T) {
	before := sampleReport("baseline", 0.20, 0.01, []CaseVerdict{
		{CaseID: "a", Passed: false},
		{CaseID: "b", Passed: true},
	})
	after := sampleReport("candidate", 0.55, 0.09, []CaseVerdict{
		{CaseID: "a", Passed: true},
		{CaseID: "b", Passed: false},
	})

	diff := ComputeDiff(before, after)

	assert.Equal(t, "baseline", diff.BeforeProfile)
	assert.Equal(t, "candidate", diff.AfterProfile)
	assert.Equal(t, []string{"a"}, diff.NewlyPassing)
	assert.Equal(t, []string{"b"}, diff.NewlyFailing)

	recall := findDelta(t, diff, "retrieval.recall@20")
	assert.InDelta(t, 0.35, recall.Delta, 0.0001)

	rerank := findDelta(t, diff, "rerank.ndcg@10_delta")
	assert.InDelta(t, 0.08, rerank.Delta, 0.0001)
}

func TestComputeDiff_SerializesToJSON(t *testing.T) {
	diff := ComputeDiff(
		sampleReport("baseline", 0.2, 0.0, []CaseVerdict{{CaseID: "a", Passed: false}}),
		sampleReport("candidate", 0.4, 0.0, []CaseVerdict{{CaseID: "a", Passed: true}}),
	)

	path := filepath.Join(t.TempDir(), "diff.json")
	require.NoError(t, SaveDiff(diff, path))

	data, err := os.ReadFile(path) // #nosec G304 -- path is under t.TempDir()
	require.NoError(t, err)

	var round ReportDiff
	require.NoError(t, json.Unmarshal(data, &round))
	assert.Equal(t, "candidate", round.AfterProfile)
	assert.NotEmpty(t, round.Metrics)
}

func TestDiffReports_RendersStageSections(t *testing.T) {
	out := DiffReports(
		sampleReport("baseline", 0.2, 0.0, []CaseVerdict{{CaseID: "a", Passed: false}}),
		sampleReport("candidate", 0.4, 0.0, []CaseVerdict{{CaseID: "a", Passed: true}}),
	)
	assert.Contains(t, out, "retrieval.recall@20")
	assert.Contains(t, out, "baseline")
	assert.Contains(t, out, "candidate")
	assert.Contains(t, out, "Newly Passing")
}

func TestSaveReport_RoundTripsStageMetrics(t *testing.T) {
	report := sampleReport("candidate", 0.42, 0.07, []CaseVerdict{{CaseID: "a", Passed: true}})
	report.Categories = map[string]CategorySummary{
		CategoryCrossLingual: {CaseCount: 1, PassCount: 1},
	}

	path := filepath.Join(t.TempDir(), "report.json")
	require.NoError(t, SaveReport(report, path))

	loaded, err := LoadReport(path)
	require.NoError(t, err)
	assert.InDelta(t, 0.42, loaded.Stages.Retrieval.MeanRecallAt20, 0.0001)
	assert.InDelta(t, 0.07, loaded.Stages.Rerank.MeanNDCGAt10Delta, 0.0001)
	assert.Equal(t, "candidate", loaded.Profile.Name)
	assert.Equal(t, 1, loaded.Categories[CategoryCrossLingual].PassCount)
}

func findDelta(t *testing.T, diff ReportDiff, name string) MetricDelta {
	t.Helper()
	for _, m := range diff.Metrics {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("metric %q not found in diff", name)
	return MetricDelta{}
}
