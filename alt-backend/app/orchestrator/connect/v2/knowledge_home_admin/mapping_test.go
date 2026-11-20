package knowledge_home_admin

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	"alt/orchestrator/usecase/knowledge_url_backfill_usecase"
	"alt/utils/safeconv"
)

func TestConvertBackfillJob(t *testing.T) {
	jobID := uuid.New()
	createdAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC)
	completedAt := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    *domain.KnowledgeBackfillJob
		wantNil  bool
		validate func(t *testing.T, proto *domain.KnowledgeBackfillJob)
	}{
		{
			name:    "nil job returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "full job converts correctly",
			input: &domain.KnowledgeBackfillJob{
				JobID:             jobID,
				Status:            "completed",
				ProjectionVersion: 2,
				TotalEvents:       100,
				ProcessedEvents:   100,
				ErrorMessage:      "",
				CreatedAt:         createdAt,
				StartedAt:         &startedAt,
				CompletedAt:       &completedAt,
			},
			wantNil: false,
		},
		{
			name: "job with nil timestamps",
			input: &domain.KnowledgeBackfillJob{
				JobID:             jobID,
				Status:            "pending",
				ProjectionVersion: 1,
				TotalEvents:       50,
				ProcessedEvents:   0,
				ErrorMessage:      "none",
				CreatedAt:         createdAt,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertBackfillJob(tt.input)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.input.JobID.String(), got.GetJobId())
			assert.Equal(t, tt.input.Status, got.GetStatus())
			assert.Equal(t, safeconv.Int32(tt.input.ProjectionVersion), got.GetProjectionVersion())
			assert.Equal(t, safeconv.Int32(tt.input.TotalEvents), got.GetTotalEvents())
			assert.Equal(t, safeconv.Int32(tt.input.ProcessedEvents), got.GetProcessedEvents())
			assert.Equal(t, tt.input.ErrorMessage, got.GetErrorMessage())
			assert.Equal(t, tt.input.CreatedAt.Format(time.RFC3339), got.GetCreatedAt())
			if tt.input.StartedAt != nil {
				assert.Equal(t, tt.input.StartedAt.Format(time.RFC3339), got.GetStartedAt())
			} else {
				assert.Empty(t, got.GetStartedAt())
			}
			if tt.input.CompletedAt != nil {
				assert.Equal(t, tt.input.CompletedAt.Format(time.RFC3339), got.GetCompletedAt())
			} else {
				assert.Empty(t, got.GetCompletedAt())
			}
		})
	}
}

func TestConvertReprojectRun(t *testing.T) {
	runID := uuid.New()
	userID := uuid.New()
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   *domain.ReprojectRun
		wantNil bool
	}{
		{
			name:    "nil run returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "full run converts correctly",
			input: &domain.ReprojectRun{
				ReprojectRunID:  runID,
				ProjectionName:  "knowledge_home",
				FromVersion:     "v1",
				ToVersion:       "v2",
				Mode:            "full",
				Status:          "running",
				InitiatedBy:     &userID,
				RangeStart:      &now,
				RangeEnd:        &now,
				StartedAt:       &now,
				FinishedAt:      &now,
				StatsJSON:       []byte(`{"processed":10}`),
				DiffSummaryJSON: []byte(`{"diffs":0}`),
				CreatedAt:       now,
			},
			wantNil: false,
		},
		{
			name: "minimal run converts correctly",
			input: &domain.ReprojectRun{
				ReprojectRunID: runID,
				ProjectionName: "knowledge_home",
				FromVersion:    "v1",
				ToVersion:      "v2",
				Mode:           "incremental",
				Status:         "queued",
				CreatedAt:      now,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertReprojectRun(tt.input)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.input.ReprojectRunID.String(), got.GetReprojectRunId())
			assert.Equal(t, tt.input.ProjectionName, got.GetProjectionName())
			assert.Equal(t, tt.input.FromVersion, got.GetFromVersion())
			assert.Equal(t, tt.input.ToVersion, got.GetToVersion())
			assert.Equal(t, tt.input.Mode, got.GetMode())
			assert.Equal(t, tt.input.Status, got.GetStatus())
			assert.Equal(t, string(tt.input.StatsJSON), got.GetStatsJson())
			assert.Equal(t, string(tt.input.DiffSummaryJSON), got.GetDiffSummaryJson())
			assert.Equal(t, tt.input.CreatedAt.Format(time.RFC3339), got.GetCreatedAt())
			if tt.input.InitiatedBy != nil {
				assert.Equal(t, tt.input.InitiatedBy.String(), got.GetInitiatedBy())
			}
		})
	}
}

func TestConvertReprojectDiff(t *testing.T) {
	tests := []struct {
		name  string
		input *domain.ReprojectDiffSummary
	}{
		{
			name: "valid diff converts distributions",
			input: &domain.ReprojectDiffSummary{
				FromItemCount:       100,
				ToItemCount:         105,
				FromEmptyCount:      5,
				ToEmptyCount:        2,
				FromAvgScore:        0.75,
				ToAvgScore:          0.82,
				FromWhyDistribution: map[string]int64{"recency": 80, "hotspot": 20},
				ToWhyDistribution:   map[string]int64{"recency": 85, "hotspot": 20},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertReprojectDiff(tt.input)
			require.NotNil(t, got)
			assert.Equal(t, tt.input.FromItemCount, got.GetFromItemCount())
			assert.Equal(t, tt.input.ToItemCount, got.GetToItemCount())
			assert.Equal(t, tt.input.FromEmptyCount, got.GetFromEmptyCount())
			assert.Equal(t, tt.input.ToEmptyCount, got.GetToEmptyCount())
			assert.InDelta(t, tt.input.FromAvgScore, got.GetFromAvgScore(), 0.001)
			assert.InDelta(t, tt.input.ToAvgScore, got.GetToAvgScore(), 0.001)
			assert.Contains(t, got.GetFromWhyDistribution(), "recency")
			assert.Contains(t, got.GetToWhyDistribution(), "hotspot")
		})
	}
}

func TestConvertSLIStatuses(t *testing.T) {
	tests := []struct {
		name     string
		input    []domain.SLIResult
		expected int
	}{
		{
			name:     "empty SLIs",
			input:    nil,
			expected: 0,
		},
		{
			name: "multiple SLIs",
			input: []domain.SLIResult{
				{
					Name:                   "availability",
					CurrentValue:           99.9,
					TargetValue:            99.0,
					Unit:                   "%",
					Status:                 "passing",
					ErrorBudgetConsumedPct: 10.0,
				},
				{
					Name:                   "latency_p95",
					CurrentValue:           120.0,
					TargetValue:            200.0,
					Unit:                   "ms",
					Status:                 "passing",
					ErrorBudgetConsumedPct: 25.0,
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSLIStatuses(tt.input)
			assert.Len(t, got, tt.expected)
			for i, sli := range tt.input {
				assert.Equal(t, sli.Name, got[i].GetName())
				assert.InDelta(t, sli.CurrentValue, got[i].GetCurrentValue(), 1e-9)
				assert.InDelta(t, sli.TargetValue, got[i].GetTargetValue(), 1e-9)
				assert.Equal(t, sli.Unit, got[i].GetUnit())
				assert.Equal(t, sli.Status, got[i].GetStatus())
				assert.InDelta(t, sli.ErrorBudgetConsumedPct, got[i].GetErrorBudgetConsumedPct(), 1e-9)
			}
		})
	}
}

func TestConvertAlertSummaries(t *testing.T) {
	firedAt := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	tests := []struct {
		name     string
		input    []domain.AlertSummary
		expected int
	}{
		{
			name:     "empty alerts",
			input:    nil,
			expected: 0,
		},
		{
			name: "multiple alerts",
			input: []domain.AlertSummary{
				{
					AlertName:   "HighLag",
					Severity:    "warning",
					Status:      "firing",
					FiredAt:     firedAt,
					Description: "Projector lag exceeded 60s",
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertAlertSummaries(tt.input)
			assert.Len(t, got, tt.expected)
			if len(tt.input) > 0 {
				assert.Equal(t, tt.input[0].AlertName, got[0].GetAlertName())
				assert.Equal(t, tt.input[0].Severity, got[0].GetSeverity())
				assert.Equal(t, tt.input[0].Status, got[0].GetStatus())
				assert.Equal(t, tt.input[0].FiredAt.Format(time.RFC3339), got[0].GetFiredAt())
				assert.Equal(t, tt.input[0].Description, got[0].GetDescription())
			}
		})
	}
}

func TestConvertProjectionAudit(t *testing.T) {
	auditID := uuid.New()
	checkedAt := time.Date(2026, 3, 2, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		input *domain.ProjectionAudit
	}{
		{
			name: "valid audit converts correctly",
			input: &domain.ProjectionAudit{
				AuditID:           auditID,
				ProjectionName:    "knowledge_home",
				ProjectionVersion: "v2",
				CheckedAt:         checkedAt,
				SampleSize:        500,
				MismatchCount:     3,
				DetailsJSON:       []byte(`{"mismatches":[]}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertProjectionAudit(tt.input)
			require.NotNil(t, got)
			assert.Equal(t, tt.input.AuditID.String(), got.GetAuditId())
			assert.Equal(t, tt.input.ProjectionName, got.GetProjectionName())
			assert.Equal(t, tt.input.ProjectionVersion, got.GetProjectionVersion())
			assert.Equal(t, tt.input.CheckedAt.Format(time.RFC3339), got.GetCheckedAt())
			assert.Equal(t, safeconv.Int32(tt.input.SampleSize), got.GetSampleSize())
			assert.Equal(t, safeconv.Int32(tt.input.MismatchCount), got.GetMismatchCount())
			assert.Equal(t, string(tt.input.DetailsJSON), got.GetDetailsJson())
		})
	}
}

func TestConvertSystemMetrics(t *testing.T) {
	now := time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		input *domain.SystemMetrics
	}{
		{
			name: "full system metrics converts correctly",
			input: &domain.SystemMetrics{
				Projector: domain.ProjectorMetrics{
					EventsProcessed:    1000,
					LagSeconds:         1.5,
					BatchDurationMsP50: 10.0,
					BatchDurationMsP95: 25.0,
					BatchDurationMsP99: 50.0,
					Errors:             0,
				},
				Handler: domain.HandlerMetrics{
					PagesServed:     500,
					PagesDegraded:   5,
					DegradedRatePct: 1.0,
				},
				Tracking: domain.TrackingMetrics{
					ItemsExposed:   300,
					ItemsOpened:    60,
					ItemsDismissed: 10,
					OpenRatePct:    20.0,
					DismissRatePct: 3.3,
				},
				Stream: domain.StreamMetrics{
					ConnectionsTotal:  40,
					DisconnectsTotal:  2,
					ReconnectsTotal:   1,
					DeliveriesTotal:   800,
					DisconnectRatePct: 5.0,
				},
				Correctness: domain.CorrectnessMetrics{
					EmptyResponses:      2,
					MalformedWhy:        0,
					OrphanItems:         0,
					SupersedeMismatch:   0,
					RequestsTotal:       500,
					CorrectnessScorePct: 99.6,
				},
				Sovereign: domain.SovereignMetrics{
					MutationsApplied:      150,
					MutationsErrors:       1,
					MutationDurationMsP50: 8.0,
					MutationDurationMsP95: 18.0,
					ErrorRatePct:          0.67,
				},
				Recall: domain.RecallMetrics{
					SignalsAppended:        50,
					SignalErrors:           0,
					CandidatesGenerated:    30,
					CandidatesEmpty:        2,
					UsersProcessed:         10,
					ProjectorDurationMsP50: 12.0,
					ProjectorDurationMsP95: 22.0,
				},
				ServiceHealth: []domain.ServiceHealthStatus{
					{
						ServiceName:  "sovereign",
						Endpoint:     "http://localhost:9100",
						Status:       "healthy",
						LatencyMs:    5,
						CheckedAt:    now,
						ErrorMessage: "",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSystemMetrics(tt.input)
			require.NotNil(t, got)
			assert.Equal(t, tt.input.Projector.EventsProcessed, got.GetProjector().GetEventsProcessed())
			assert.Equal(t, tt.input.Handler.PagesServed, got.GetHandler().GetPagesServed())
			assert.Equal(t, tt.input.Tracking.ItemsExposed, got.GetTracking().GetItemsExposed())
			assert.Equal(t, tt.input.Stream.ConnectionsTotal, got.GetStream().GetConnectionsTotal())
			assert.Equal(t, tt.input.Correctness.EmptyResponses, got.GetCorrectness().GetEmptyResponses())
			assert.Equal(t, tt.input.Sovereign.MutationsApplied, got.GetSovereign().GetMutationsApplied())
			assert.Equal(t, tt.input.Recall.SignalsAppended, got.GetRecall().GetSignalsAppended())
			require.Len(t, got.GetServiceHealth(), 1)
			assert.Equal(t, "sovereign", got.GetServiceHealth()[0].GetServiceName())
		})
	}
}

func TestConvertEmitArticleUrlBackfillResponse(t *testing.T) {
	tests := []struct {
		name  string
		input *knowledge_url_backfill_usecase.EmitResult
	}{
		{
			name: "valid result converts correctly",
			input: &knowledge_url_backfill_usecase.EmitResult{
				ArticlesScanned:      100,
				EventsAppended:       80,
				SkippedBlockedScheme: 10,
				SkippedDuplicate:     10,
				MoreRemaining:        true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertEmitArticleUrlBackfillResponse(tt.input)
			require.NotNil(t, got)
			assert.Equal(t, safeconv.Int32(tt.input.ArticlesScanned), got.GetArticlesScanned())
			assert.Equal(t, safeconv.Int32(tt.input.EventsAppended), got.GetEventsAppended())
			assert.Equal(t, safeconv.Int32(tt.input.SkippedBlockedScheme), got.GetSkippedBlockedScheme())
			assert.Equal(t, safeconv.Int32(tt.input.SkippedDuplicate), got.GetSkippedDuplicate())
			assert.Equal(t, tt.input.MoreRemaining, got.GetMoreRemaining())
		})
	}
}
