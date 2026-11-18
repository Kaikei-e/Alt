package evalconnections

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateRepresentations_ThresholdSelectionAndUnsureHandling(t *testing.T) {
	tests := []struct {
		name           string
		key            KeyFile
		rows           []SheetRow
		wantPassed     bool
		wantRep        string
		wantMinSim     float64
		wantProposed   int
		wantJudged     int
		wantConnected  int
		wantUnsure     int
		wantShare      float64
		wantSkipSumVec bool
	}{
		{
			name: "picks representation and threshold proposing most pairs at >= 0.60 excluding unsure from denominator",
			key: KeyFile{
				ThresholdVersion: 1,
				Representations: map[string]string{
					"first_chunk":        "available",
					"chunk_mean":         "available",
					"nearest_chunk_pair": "available",
					"summary_vector":     "skipped",
				},
				Pairs: []KeyPair{
					// Under chunk_mean:
					// >= 0.60: 5 pairs (3 connected, 1 not_connected, 1 unsure) -> proposed=5, judged=4, share=3/4=0.75 (>=0.60)
					// >= 0.50: 6 pairs (3 connected, 2 not_connected, 1 unsure) -> proposed=6, judged=5, share=3/5=0.60 (>=0.60) -> wins max proposed!
					// >= 0.40: 7 pairs (3 connected, 3 not_connected, 1 unsure) -> proposed=7, judged=6, share=3/6=0.50 (<0.60)
					{PairNumber: 1, Similarities: map[string]float64{"first_chunk": 0.90, "chunk_mean": 0.90, "nearest_chunk_pair": 0.80}},
					{PairNumber: 2, Similarities: map[string]float64{"first_chunk": 0.80, "chunk_mean": 0.85, "nearest_chunk_pair": 0.75}},
					{PairNumber: 3, Similarities: map[string]float64{"first_chunk": 0.10, "chunk_mean": 0.80, "nearest_chunk_pair": 0.70}},
					{PairNumber: 4, Similarities: map[string]float64{"first_chunk": 0.75, "chunk_mean": 0.70, "nearest_chunk_pair": 0.60}},
					{PairNumber: 5, Similarities: map[string]float64{"first_chunk": 0.60, "chunk_mean": 0.60, "nearest_chunk_pair": 0.50}},
					{PairNumber: 6, Similarities: map[string]float64{"first_chunk": 0.50, "chunk_mean": 0.50, "nearest_chunk_pair": 0.40}},
					{PairNumber: 7, Similarities: map[string]float64{"first_chunk": 0.40, "chunk_mean": 0.40, "nearest_chunk_pair": 0.30}},
				},
			},
			rows: []SheetRow{
				{PairNumber: 1, Verdict: "connected"},
				{PairNumber: 2, Verdict: "connected"},
				{PairNumber: 3, Verdict: "connected"},
				{PairNumber: 4, Verdict: "unsure"},
				{PairNumber: 5, Verdict: "not_connected"},
				{PairNumber: 6, Verdict: "not_connected"},
				{PairNumber: 7, Verdict: "not_connected"},
			},
			wantPassed:     true,
			wantRep:        "chunk_mean",
			wantMinSim:     0.50,
			wantProposed:   6,
			wantJudged:     5,
			wantConnected:  3,
			wantUnsure:     1,
			wantShare:      0.60,
			wantSkipSumVec: true,
		},
		{
			name: "FAIL when no representation reaches 0.60 connected share",
			key: KeyFile{
				ThresholdVersion: 1,
				Representations: map[string]string{
					"first_chunk":        "available",
					"chunk_mean":         "available",
					"nearest_chunk_pair": "available",
					"summary_vector":     "skipped",
				},
				Pairs: []KeyPair{
					{PairNumber: 1, Similarities: map[string]float64{"first_chunk": 0.90, "chunk_mean": 0.90, "nearest_chunk_pair": 0.90}},
					{PairNumber: 2, Similarities: map[string]float64{"first_chunk": 0.90, "chunk_mean": 0.90, "nearest_chunk_pair": 0.90}},
					{PairNumber: 3, Similarities: map[string]float64{"first_chunk": 0.50, "chunk_mean": 0.50, "nearest_chunk_pair": 0.50}},
				},
			},
			rows: []SheetRow{
				{PairNumber: 1, Verdict: "not_connected"},
				{PairNumber: 2, Verdict: "not_connected"},
				{PairNumber: 3, Verdict: "connected"},
			},
			wantPassed:     false,
			wantSkipSumVec: true,
		},
		{
			name: "summary_vector evaluated when available",
			key: KeyFile{
				ThresholdVersion: 1,
				Representations: map[string]string{
					"first_chunk":        "available",
					"chunk_mean":         "available",
					"nearest_chunk_pair": "available",
					"summary_vector":     "available",
				},
				Pairs: []KeyPair{
					{PairNumber: 1, Similarities: map[string]float64{"first_chunk": 0.50, "chunk_mean": 0.50, "nearest_chunk_pair": 0.50, "summary_vector": 0.90}},
					{PairNumber: 2, Similarities: map[string]float64{"first_chunk": 0.40, "chunk_mean": 0.40, "nearest_chunk_pair": 0.40, "summary_vector": 0.85}},
					{PairNumber: 3, Similarities: map[string]float64{"first_chunk": 0.30, "chunk_mean": 0.30, "nearest_chunk_pair": 0.30, "summary_vector": 0.80}},
					{PairNumber: 4, Similarities: map[string]float64{"first_chunk": 0.90, "chunk_mean": 0.90, "nearest_chunk_pair": 0.90, "summary_vector": 0.10}},
					{PairNumber: 5, Similarities: map[string]float64{"first_chunk": 0.85, "chunk_mean": 0.85, "nearest_chunk_pair": 0.85, "summary_vector": 0.10}},
					{PairNumber: 6, Similarities: map[string]float64{"first_chunk": 0.80, "chunk_mean": 0.80, "nearest_chunk_pair": 0.80, "summary_vector": 0.10}},
					{PairNumber: 7, Similarities: map[string]float64{"first_chunk": 0.75, "chunk_mean": 0.75, "nearest_chunk_pair": 0.75, "summary_vector": 0.10}},
				},
			},
			rows: []SheetRow{
				{PairNumber: 1, Verdict: "connected"},
				{PairNumber: 2, Verdict: "connected"},
				{PairNumber: 3, Verdict: "connected"},
				{PairNumber: 4, Verdict: "not_connected"},
				{PairNumber: 5, Verdict: "not_connected"},
				{PairNumber: 6, Verdict: "not_connected"},
				{PairNumber: 7, Verdict: "not_connected"},
			},
			wantPassed:     true,
			wantRep:        "summary_vector",
			wantMinSim:     0.80,
			wantProposed:   3,
			wantJudged:     3,
			wantConnected:  3,
			wantUnsure:     0,
			wantShare:      1.0,
			wantSkipSumVec: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report, err := EvaluateRepresentations(tc.rows, tc.key)
			require.NoError(t, err)
			assert.Equal(t, ThresholdVersion, report.ThresholdVersion)
			assert.Equal(t, tc.wantPassed, report.Passed)

			var sumStatus string
			for _, r := range report.Representations {
				if r.Representation == "summary_vector" {
					sumStatus = r.Status
				}
			}
			if tc.wantSkipSumVec {
				assert.Equal(t, "skipped", sumStatus)
			} else {
				assert.Equal(t, "available", sumStatus)
			}

			if tc.wantPassed {
				require.NotNil(t, report.Recommendation)
				assert.Equal(t, tc.wantRep, report.Recommendation.Representation)
				assert.InDelta(t, tc.wantMinSim, report.Recommendation.MinSimilarity, 1e-9)
				assert.Equal(t, tc.wantProposed, report.Recommendation.ProposedCount)
				assert.Equal(t, tc.wantJudged, report.Recommendation.JudgedCount)
				assert.Equal(t, tc.wantConnected, report.Recommendation.ConnectedCount)
				assert.Equal(t, tc.wantUnsure, report.Recommendation.UnsureCount)
				assert.InDelta(t, tc.wantShare, report.Recommendation.ConnectedShare, 1e-9)
			} else {
				assert.Nil(t, report.Recommendation)
				assert.Contains(t, strings.ToLower(report.Message), "no ui")
				formatted := FormatScoreReport(report)
				assert.Contains(t, formatted, "FAIL")
				assert.Contains(t, strings.ToLower(formatted), "no ui")
			}

			// Verify JSON round-trip works cleanly.
			raw, err := json.Marshal(report)
			require.NoError(t, err)
			var decoded ScoreReport
			require.NoError(t, json.Unmarshal(raw, &decoded))
			assert.Equal(t, report.Passed, decoded.Passed)
			assert.Equal(t, report.ThresholdVersion, decoded.ThresholdVersion)
		})
	}
}

func TestEvaluateRepresentations_RejectsInvalidOrMissingVerdict(t *testing.T) {
	key := KeyFile{
		ThresholdVersion: 1,
		Representations:  map[string]string{"first_chunk": "available"},
		Pairs:            []KeyPair{{PairNumber: 1, Similarities: map[string]float64{"first_chunk": 0.9}}},
	}

	_, err := EvaluateRepresentations([]SheetRow{{PairNumber: 1, Verdict: ""}}, key)
	require.Error(t, err)

	_, err = EvaluateRepresentations([]SheetRow{{PairNumber: 1, Verdict: "maybe"}}, key)
	require.Error(t, err)

	_, err = EvaluateRepresentations([]SheetRow{}, key)
	require.Error(t, err)
}
