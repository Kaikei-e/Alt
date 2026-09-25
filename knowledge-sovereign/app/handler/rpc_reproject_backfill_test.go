package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestValidateAndBuildProjectionAudit(t *testing.T) {
	validAuditID := uuid.New().String()
	checked := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	pbChecked := timestamppb.New(checked)

	tests := []struct {
		name    string
		in      *sovereignv1.ProjectionAudit
		wantErr string
	}{
		{
			name:    "nil audit returns error",
			in:      nil,
			wantErr: "audit is required",
		},
		{
			name: "invalid audit_id",
			in: &sovereignv1.ProjectionAudit{
				AuditId: "not-a-uuid",
			},
			wantErr: "invalid audit_id",
		},
		{
			name: "valid audit",
			in: &sovereignv1.ProjectionAudit{
				AuditId:           validAuditID,
				ProjectionName:    "knowledge_home",
				ProjectionVersion: "v2",
				CheckedAt:         pbChecked,
				SampleSize:        100,
				MismatchCount:     0,
				DetailsJson:       []byte(`{"status":"ok"}`),
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndBuildProjectionAudit(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, validAuditID, got.AuditID.String())
				assert.Equal(t, "knowledge_home", got.ProjectionName)
				assert.Equal(t, "v2", got.ProjectionVersion)
				assert.Equal(t, 100, got.SampleSize)
				assert.Equal(t, 0, got.MismatchCount)
				assert.Equal(t, json.RawMessage(`{"status":"ok"}`), got.DetailsJSON)
				assert.True(t, got.CheckedAt.Equal(checked))
			}
		})
	}
}

func TestReprojectRunMappersRoundTrip(t *testing.T) {
	runID := uuid.New()
	userID := uuid.New()
	created := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 9, 25, 10, 5, 0, 0, time.UTC)
	fin := time.Date(2026, 9, 25, 10, 30, 0, 0, time.UTC)

	run := sovereign_db.ReprojectRun{
		ReprojectRunID:    runID,
		ProjectionName:    "knowledge_trail",
		FromVersion:       "v1",
		ToVersion:         "v2",
		InitiatedBy:       &userID,
		Mode:              "shadow",
		Status:            "completed",
		CheckpointPayload: json.RawMessage(`{"last_seq": 1000}`),
		StatsJSON:         json.RawMessage(`{"duration_ms": 25000}`),
		DiffSummaryJSON:   json.RawMessage(`{"mismatches": 0}`),
		CreatedAt:         created,
		RangeStart:        &start,
		RangeEnd:          &fin,
		StartedAt:         &start,
		FinishedAt:        &fin,
	}

	pb := reprojectRunToProto(run)
	require.NotNil(t, pb)
	assert.Equal(t, runID.String(), pb.ReprojectRunId)
	assert.Equal(t, "knowledge_trail", pb.ProjectionName)
	assert.Equal(t, userID.String(), pb.InitiatedBy)

	back, err := protoToReprojectRun(pb)
	require.NoError(t, err)
	assert.Equal(t, run.ReprojectRunID, back.ReprojectRunID)
	assert.Equal(t, run.ProjectionName, back.ProjectionName)
	assert.Equal(t, run.FromVersion, back.FromVersion)
	assert.Equal(t, run.ToVersion, back.ToVersion)
	assert.Equal(t, *run.InitiatedBy, *back.InitiatedBy)
	assert.Equal(t, run.Mode, back.Mode)
	assert.Equal(t, run.Status, back.Status)
	assert.Equal(t, run.CheckpointPayload, back.CheckpointPayload)
	assert.Equal(t, run.StatsJSON, back.StatsJSON)
	assert.Equal(t, run.DiffSummaryJSON, back.DiffSummaryJSON)
}

func TestProtoToReprojectRun_Nil(t *testing.T) {
	got, err := protoToReprojectRun(nil)
	require.NoError(t, err)
	assert.Equal(t, sovereign_db.ReprojectRun{}, got)
}

func TestProtoToReprojectRun_InvalidRunID(t *testing.T) {
	_, err := protoToReprojectRun(&sovereignv1.ReprojectRun{
		ReprojectRunId: "not-a-uuid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid reproject_run_id")
}

func TestBackfillJobMappersRoundTrip(t *testing.T) {
	jobID := uuid.New()
	cursorUserID := uuid.New()
	cursorArticleID := uuid.New()
	cursorDate := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 9, 25, 8, 0, 0, 0, time.UTC)
	started := time.Date(2026, 9, 25, 8, 1, 0, 0, time.UTC)
	completed := time.Date(2026, 9, 25, 8, 15, 0, 0, time.UTC)

	job := sovereign_db.BackfillJob{
		JobID:             jobID,
		Status:            "completed",
		Kind:              "historical_reindex",
		ProjectionVersion: 2,
		TotalEvents:       5000,
		ProcessedEvents:   5000,
		ErrorMessage:      "",
		CursorUserID:      &cursorUserID,
		CursorDate:        &cursorDate,
		CursorArticleID:   &cursorArticleID,
		CreatedAt:         created,
		StartedAt:         &started,
		CompletedAt:       &completed,
		UpdatedAt:         completed,
	}

	pb := backfillJobToProto(job)
	require.NotNil(t, pb)
	assert.Equal(t, jobID.String(), pb.JobId)
	assert.Equal(t, cursorUserID.String(), pb.CursorUserId)
	assert.Equal(t, "2026-09-24", pb.CursorDate)
	assert.Equal(t, cursorArticleID.String(), pb.CursorArticleId)

	back, err := protoToBackfillJob(pb)
	require.NoError(t, err)
	assert.Equal(t, job.JobID, back.JobID)
	assert.Equal(t, job.Status, back.Status)
	assert.Equal(t, job.Kind, back.Kind)
	assert.Equal(t, job.ProjectionVersion, back.ProjectionVersion)
	assert.Equal(t, job.TotalEvents, back.TotalEvents)
	assert.Equal(t, job.ProcessedEvents, back.ProcessedEvents)
	assert.Equal(t, *job.CursorUserID, *back.CursorUserID)
	assert.Equal(t, *job.CursorArticleID, *back.CursorArticleID)
	assert.Equal(t, "2026-09-24", back.CursorDate.Format("2006-01-02"))
}

func TestProtoToBackfillJob_Nil(t *testing.T) {
	got, err := protoToBackfillJob(nil)
	require.NoError(t, err)
	assert.Equal(t, sovereign_db.BackfillJob{}, got)
}

func TestProtoToBackfillJob_InvalidJobID(t *testing.T) {
	_, err := protoToBackfillJob(&sovereignv1.BackfillJob{
		JobId: "not-a-uuid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid job_id")
}
