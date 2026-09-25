package knowledge_home_admin

import (
	"encoding/json"
	"time"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/knowledge_url_backfill_usecase"
	"alt/utils/safeconv"
)

// convertBackfillJob converts domain backfill job to proto.
func convertBackfillJob(job *domain.KnowledgeBackfillJob) *knowledgehomev1.BackfillJob {
	if job == nil {
		return nil
	}
	proto := &knowledgehomev1.BackfillJob{
		JobId:             job.JobID.String(),
		Status:            job.Status,
		ProjectionVersion: safeconv.Int32(job.ProjectionVersion),
		TotalEvents:       safeconv.Int32(job.TotalEvents),
		ProcessedEvents:   safeconv.Int32(job.ProcessedEvents),
		ErrorMessage:      job.ErrorMessage,
		CreatedAt:         job.CreatedAt.Format(time.RFC3339),
	}
	if job.StartedAt != nil {
		proto.StartedAt = job.StartedAt.Format(time.RFC3339)
	}
	if job.CompletedAt != nil {
		proto.CompletedAt = job.CompletedAt.Format(time.RFC3339)
	}
	return proto
}

// convertReprojectRun converts domain reproject run to proto.
func convertReprojectRun(run *domain.ReprojectRun) *knowledgehomev1.ReprojectRun {
	if run == nil {
		return nil
	}
	proto := &knowledgehomev1.ReprojectRun{
		ReprojectRunId:  run.ReprojectRunID.String(),
		ProjectionName:  run.ProjectionName,
		FromVersion:     run.FromVersion,
		ToVersion:       run.ToVersion,
		Mode:            run.Mode,
		Status:          run.Status,
		StatsJson:       string(run.StatsJSON),
		DiffSummaryJson: string(run.DiffSummaryJSON),
		CreatedAt:       run.CreatedAt.Format(time.RFC3339),
	}
	if run.InitiatedBy != nil {
		proto.InitiatedBy = run.InitiatedBy.String()
	}
	if run.RangeStart != nil {
		proto.RangeStart = run.RangeStart.Format(time.RFC3339)
	}
	if run.RangeEnd != nil {
		proto.RangeEnd = run.RangeEnd.Format(time.RFC3339)
	}
	if run.StartedAt != nil {
		proto.StartedAt = run.StartedAt.Format(time.RFC3339)
	}
	if run.FinishedAt != nil {
		proto.FinishedAt = run.FinishedAt.Format(time.RFC3339)
	}
	return proto
}

// convertReprojectDiff converts reproject diff summary to proto.
func convertReprojectDiff(diff *domain.ReprojectDiffSummary) *knowledgehomev1.ReprojectDiffSummary {
	fromWhyJSON, _ := json.Marshal(diff.FromWhyDistribution)
	toWhyJSON, _ := json.Marshal(diff.ToWhyDistribution)

	return &knowledgehomev1.ReprojectDiffSummary{
		FromItemCount:       diff.FromItemCount,
		ToItemCount:         diff.ToItemCount,
		FromEmptyCount:      diff.FromEmptyCount,
		ToEmptyCount:        diff.ToEmptyCount,
		FromAvgScore:        diff.FromAvgScore,
		ToAvgScore:          diff.ToAvgScore,
		FromWhyDistribution: string(fromWhyJSON),
		ToWhyDistribution:   string(toWhyJSON),
	}
}

// convertSLIStatuses converts domain SLI results to proto.
func convertSLIStatuses(slis []domain.SLIResult) []*knowledgehomev1.SLIStatus {
	protoSLIs := make([]*knowledgehomev1.SLIStatus, 0, len(slis))
	for _, sli := range slis {
		protoSLIs = append(protoSLIs, &knowledgehomev1.SLIStatus{
			Name:                   sli.Name,
			CurrentValue:           sli.CurrentValue,
			TargetValue:            sli.TargetValue,
			Unit:                   sli.Unit,
			Status:                 sli.Status,
			ErrorBudgetConsumedPct: sli.ErrorBudgetConsumedPct,
		})
	}
	return protoSLIs
}

// convertAlertSummaries converts domain alert summaries to proto.
func convertAlertSummaries(alerts []domain.AlertSummary) []*knowledgehomev1.AlertSummary {
	protoAlerts := make([]*knowledgehomev1.AlertSummary, 0, len(alerts))
	for _, alert := range alerts {
		protoAlerts = append(protoAlerts, &knowledgehomev1.AlertSummary{
			AlertName:   alert.AlertName,
			Severity:    alert.Severity,
			Status:      alert.Status,
			FiredAt:     alert.FiredAt.Format(time.RFC3339),
			Description: alert.Description,
		})
	}
	return protoAlerts
}

// convertProjectionAudit converts domain audit result to proto.
func convertProjectionAudit(audit *domain.ProjectionAudit) *knowledgehomev1.ProjectionAudit {
	return &knowledgehomev1.ProjectionAudit{
		AuditId:           audit.AuditID.String(),
		ProjectionName:    audit.ProjectionName,
		ProjectionVersion: audit.ProjectionVersion,
		CheckedAt:         audit.CheckedAt.Format(time.RFC3339),
		SampleSize:        safeconv.Int32(audit.SampleSize),
		MismatchCount:     safeconv.Int32(audit.MismatchCount),
		DetailsJson:       string(audit.DetailsJSON),
	}
}

// convertSystemMetrics converts domain system metrics to proto response.
func convertSystemMetrics(metrics *domain.SystemMetrics) *knowledgehomev1.GetSystemMetricsResponse {
	protoHealth := make([]*knowledgehomev1.ServiceHealthStatus, 0, len(metrics.ServiceHealth))
	for _, sh := range metrics.ServiceHealth {
		protoHealth = append(protoHealth, &knowledgehomev1.ServiceHealthStatus{
			ServiceName:  sh.ServiceName,
			Endpoint:     sh.Endpoint,
			Status:       sh.Status,
			LatencyMs:    sh.LatencyMs,
			CheckedAt:    sh.CheckedAt.Format(time.RFC3339),
			ErrorMessage: sh.ErrorMessage,
		})
	}

	return &knowledgehomev1.GetSystemMetricsResponse{
		Projector: &knowledgehomev1.ProjectorMetrics{
			EventsProcessed:    metrics.Projector.EventsProcessed,
			LagSeconds:         metrics.Projector.LagSeconds,
			BatchDurationMsP50: metrics.Projector.BatchDurationMsP50,
			BatchDurationMsP95: metrics.Projector.BatchDurationMsP95,
			BatchDurationMsP99: metrics.Projector.BatchDurationMsP99,
			Errors:             metrics.Projector.Errors,
		},
		Handler: &knowledgehomev1.HandlerMetrics{
			PagesServed:     metrics.Handler.PagesServed,
			PagesDegraded:   metrics.Handler.PagesDegraded,
			DegradedRatePct: metrics.Handler.DegradedRatePct,
		},
		Tracking: &knowledgehomev1.TrackingMetrics{
			ItemsExposed:   metrics.Tracking.ItemsExposed,
			ItemsOpened:    metrics.Tracking.ItemsOpened,
			ItemsDismissed: metrics.Tracking.ItemsDismissed,
			OpenRatePct:    metrics.Tracking.OpenRatePct,
			DismissRatePct: metrics.Tracking.DismissRatePct,
		},
		Stream: &knowledgehomev1.StreamMetrics{
			ConnectionsTotal:  metrics.Stream.ConnectionsTotal,
			DisconnectsTotal:  metrics.Stream.DisconnectsTotal,
			ReconnectsTotal:   metrics.Stream.ReconnectsTotal,
			DeliveriesTotal:   metrics.Stream.DeliveriesTotal,
			DisconnectRatePct: metrics.Stream.DisconnectRatePct,
		},
		Correctness: &knowledgehomev1.CorrectnessMetrics{
			EmptyResponses:      metrics.Correctness.EmptyResponses,
			MalformedWhy:        metrics.Correctness.MalformedWhy,
			OrphanItems:         metrics.Correctness.OrphanItems,
			SupersedeMismatch:   metrics.Correctness.SupersedeMismatch,
			RequestsTotal:       metrics.Correctness.RequestsTotal,
			CorrectnessScorePct: metrics.Correctness.CorrectnessScorePct,
		},
		Sovereign: &knowledgehomev1.SovereignMetrics{
			MutationsApplied:      metrics.Sovereign.MutationsApplied,
			MutationsErrors:       metrics.Sovereign.MutationsErrors,
			MutationDurationMsP50: metrics.Sovereign.MutationDurationMsP50,
			MutationDurationMsP95: metrics.Sovereign.MutationDurationMsP95,
			ErrorRatePct:          metrics.Sovereign.ErrorRatePct,
		},
		Recall: &knowledgehomev1.RecallMetrics{
			SignalsAppended:        metrics.Recall.SignalsAppended,
			SignalErrors:           metrics.Recall.SignalErrors,
			CandidatesGenerated:    metrics.Recall.CandidatesGenerated,
			CandidatesEmpty:        metrics.Recall.CandidatesEmpty,
			UsersProcessed:         metrics.Recall.UsersProcessed,
			ProjectorDurationMsP50: metrics.Recall.ProjectorDurationMsP50,
			ProjectorDurationMsP95: metrics.Recall.ProjectorDurationMsP95,
		},
		ServiceHealth: protoHealth,
	}
}

// convertEmitArticleUrlBackfillResponse converts url backfill emit result to proto response.
func convertEmitArticleUrlBackfillResponse(res *knowledge_url_backfill_usecase.EmitResult) *knowledgehomev1.EmitArticleUrlBackfillResponse {
	return &knowledgehomev1.EmitArticleUrlBackfillResponse{
		ArticlesScanned:      safeconv.Int32(res.ArticlesScanned),
		EventsAppended:       safeconv.Int32(res.EventsAppended),
		SkippedBlockedScheme: safeconv.Int32(res.SkippedBlockedScheme),
		SkippedDuplicate:     safeconv.Int32(res.SkippedDuplicate),
		MoreRemaining:        res.MoreRemaining,
	}
}
