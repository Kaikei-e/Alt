package sovereign_db

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcReprojectDiffSummary(t *testing.T) {
	tests := []struct {
		name string
		from versionStats
		to   versionStats
		want ReprojectDiffSummary
	}{
		{
			name: "all zeroes",
			from: versionStats{},
			to:   versionStats{},
			want: ReprojectDiffSummary{},
		},
		{
			name: "counts and scores differ",
			from: versionStats{count: 100, avgScore: 0.75, emptySummary: 5},
			to:   versionStats{count: 120, avgScore: 0.88, emptySummary: 1},
			want: ReprojectDiffSummary{
				FromCount:        100,
				ToCount:          120,
				FromAvgScore:     0.75,
				ToAvgScore:       0.88,
				FromEmptySummary: 5,
				ToEmptySummary:   1,
			},
		},
		{
			name: "identical stats",
			from: versionStats{count: 50, avgScore: 1.0, emptySummary: 0},
			to:   versionStats{count: 50, avgScore: 1.0, emptySummary: 0},
			want: ReprojectDiffSummary{
				FromCount:        50,
				ToCount:          50,
				FromAvgScore:     1.0,
				ToAvgScore:       1.0,
				FromEmptySummary: 0,
				ToEmptySummary:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcReprojectDiffSummary(tt.from, tt.to)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildCreateReprojectRunArgs(t *testing.T) {
	runID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	t0 := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		run  ReprojectRun
		want int
	}{
		{
			name: "empty JSON fields default to empty JSON object",
			run: ReprojectRun{
				ReprojectRunID: runID,
				ProjectionName: "knowledge-home",
				FromVersion:    "1",
				ToVersion:      "2",
				InitiatedBy:    &userID,
				Mode:           "full",
				Status:         "pending",
				CreatedAt:      t0,
			},
			want: 15,
		},
		{
			name: "populated JSON fields preserved",
			run: ReprojectRun{
				ReprojectRunID:    runID,
				ProjectionName:    "knowledge-home",
				FromVersion:       "1",
				ToVersion:         "2",
				InitiatedBy:       &userID,
				Mode:              "full",
				Status:            "completed",
				CheckpointPayload: json.RawMessage(`{"last_seq":100}`),
				StatsJSON:         json.RawMessage(`{"processed":100}`),
				DiffSummaryJSON:   json.RawMessage(`{"diff":0}`),
				CreatedAt:         t0,
			},
			want: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildCreateReprojectRunArgs(tt.run)
			require.Len(t, args, tt.want)
			assert.Equal(t, tt.run.ReprojectRunID, args[0])
			assert.Equal(t, tt.run.ProjectionName, args[1])
			assert.Equal(t, tt.run.FromVersion, args[2])
			assert.Equal(t, tt.run.ToVersion, args[3])
			assert.Equal(t, tt.run.InitiatedBy, args[4])
			assert.Equal(t, tt.run.Mode, args[5])
			assert.Equal(t, tt.run.Status, args[6])
			assert.NotEmpty(t, args[9])
			assert.NotEmpty(t, args[10])
			assert.NotEmpty(t, args[11])
		})
	}
}

func TestBuildUpdateReprojectRunArgs(t *testing.T) {
	runID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	t0 := time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC)

	run := ReprojectRun{
		ReprojectRunID:    runID,
		Status:            "completed",
		CheckpointPayload: json.RawMessage(`{"cursor":42}`),
		StatsJSON:         nil,
		DiffSummaryJSON:   nil,
		StartedAt:         &t0,
		FinishedAt:        &t1,
	}

	args := buildUpdateReprojectRunArgs(run)
	require.Len(t, args, 7)
	assert.Equal(t, runID, args[0])
	assert.Equal(t, "completed", args[1])
	assert.Equal(t, json.RawMessage(`{"cursor":42}`), args[2])
	assert.Equal(t, json.RawMessage(`{}`), args[3])
	assert.Equal(t, json.RawMessage(`{}`), args[4])
	assert.Equal(t, &t0, args[5])
	assert.Equal(t, &t1, args[6])
}

func TestBuildCreateProjectionAuditArgs(t *testing.T) {
	auditID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	t0 := time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		audit ProjectionAudit
	}{
		{
			name: "nil details normalized to empty JSON",
			audit: ProjectionAudit{
				AuditID:           auditID,
				ProjectionName:    "knowledge-home",
				ProjectionVersion: "2",
				CheckedAt:         t0,
				SampleSize:        50,
				MismatchCount:     0,
				DetailsJSON:       nil,
			},
		},
		{
			name: "populated details retained",
			audit: ProjectionAudit{
				AuditID:           auditID,
				ProjectionName:    "knowledge-home",
				ProjectionVersion: "2",
				CheckedAt:         t0,
				SampleSize:        50,
				MismatchCount:     2,
				DetailsJSON:       json.RawMessage(`{"mismatches":["a","b"]}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildCreateProjectionAuditArgs(tt.audit)
			require.Len(t, args, 7)
			assert.Equal(t, tt.audit.AuditID, args[0])
			assert.Equal(t, tt.audit.ProjectionName, args[1])
			assert.Equal(t, tt.audit.ProjectionVersion, args[2])
			assert.Equal(t, tt.audit.CheckedAt, args[3])
			assert.Equal(t, tt.audit.SampleSize, args[4])
			assert.Equal(t, tt.audit.MismatchCount, args[5])
			assert.NotEmpty(t, args[6])
		})
	}
}

func TestScanReprojectRun_Error(t *testing.T) {
	mock := &mockRow{scanFunc: func(_ ...interface{}) error {
		return errors.New("scan failed")
	}}

	_, err := scanReprojectRun(mock)
	require.Error(t, err)
}

func TestScanProjectionAudit_Error(t *testing.T) {
	mock := &mockRow{scanFunc: func(_ ...interface{}) error {
		return errors.New("scan failed")
	}}

	_, err := scanProjectionAudit(mock)
	require.Error(t, err)
}

func TestScanVersionStats_Error(t *testing.T) {
	mock := &mockRow{scanFunc: func(_ ...interface{}) error {
		return errors.New("scan failed")
	}}

	_, err := scanVersionStats(mock)
	require.Error(t, err)
}

func TestEmptyJSONIfNil(t *testing.T) {
	tests := []struct {
		name string
		in   json.RawMessage
		want string
	}{
		{
			name: "nil slice yields empty JSON object",
			in:   nil,
			want: "{}",
		},
		{
			name: "empty slice yields empty JSON object",
			in:   json.RawMessage([]byte{}),
			want: "{}",
		},
		{
			name: "valid JSON object preserved",
			in:   json.RawMessage(`{"key":"value"}`),
			want: `{"key":"value"}`,
		},
		{
			name: "valid JSON array preserved",
			in:   json.RawMessage(`[1,2,3]`),
			want: `[1,2,3]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := emptyJSONIfNil(tt.in)
			assert.Equal(t, tt.want, string(got))
		})
	}
}
