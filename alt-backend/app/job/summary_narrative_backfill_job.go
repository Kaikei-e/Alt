package job

import (
	"alt/domain"
	"alt/port/knowledge_backfill_port"
	"alt/port/knowledge_event_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SummaryNarrativeBackfillJob (ADR-000846) walks summary_versions JOIN articles
// and emits a discovered SummaryNarrativeBackfilled event per pair. The event
// repairs Knowledge Loop entries whose original SummaryVersionCreated event
// pre-dated the producer's article_title capture.
//
// Pattern mirrors KnowledgeBackfillJob (cursor + dedupe_key + status flow);
// the only differences are the source SQL (summary_versions JOIN articles)
// and the synthetic event factory.
//
// Cursor uses (generated_at, summary_version_id) — repurposing
// CursorDate / CursorArticleID from the shared knowledge_backfill_jobs row.
func SummaryNarrativeBackfillJob(
	updateJobPort knowledge_backfill_port.UpdateBackfillJobPort,
	listJobsPort knowledge_backfill_port.ListBackfillJobsPort,
	listSummaryTitlesPort knowledge_backfill_port.ListBackfillSummaryTitlesPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processSummaryNarrativeBatch(ctx, updateJobPort, listJobsPort, listSummaryTitlesPort, eventPort)
	}
}

func processSummaryNarrativeBatch(
	ctx context.Context,
	updateJobPort knowledge_backfill_port.UpdateBackfillJobPort,
	listJobsPort knowledge_backfill_port.ListBackfillJobsPort,
	listSummaryTitlesPort knowledge_backfill_port.ListBackfillSummaryTitlesPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) error {
	jobs, err := listJobsPort.ListBackfillJobs(ctx)
	if err != nil {
		return fmt.Errorf("list backfill jobs: %w", err)
	}

	// Pick the first pending/running job whose kind is 'summary_narratives'.
	// Other-kind rows are intentionally ignored; the kind column was added
	// for exactly this discrimination (ADR-000846).
	var activeJob *domain.KnowledgeBackfillJob
	for i := range jobs {
		if jobs[i].Kind != domain.BackfillKindSummaryNarratives {
			continue
		}
		if jobs[i].Status == domain.BackfillStatusRunning || jobs[i].Status == domain.BackfillStatusPending {
			activeJob = &jobs[i]
			break
		}
	}
	if activeJob == nil {
		return nil
	}

	if activeJob.Status == domain.BackfillStatusPending {
		now := time.Now()
		activeJob.Status = domain.BackfillStatusRunning
		activeJob.StartedAt = &now
		if err := updateJobPort.UpdateBackfillJob(ctx, *activeJob); err != nil {
			return fmt.Errorf("start summary-narrative backfill job: %w", err)
		}
	}

	// Cursor reuse: CursorDate carries the last generated_at, CursorArticleID
	// carries the last summary_version_id. The shared schema's column names
	// were chosen for the article-replay stream, so these reads / writes
	// repurpose them. The driver's ListBackfillSummaryTitles signature makes
	// the semantics explicit at its boundary.
	rows, err := listSummaryTitlesPort.ListBackfillSummaryTitles(
		ctx, activeJob.CursorDate, activeJob.CursorArticleID, batchSize,
	)
	if err != nil {
		return fmt.Errorf("list backfill summary titles: %w", err)
	}

	logger.Logger.InfoContext(ctx, "summary-narrative backfill job processing",
		"job_id", activeJob.JobID,
		"processed", activeJob.ProcessedEvents,
		"total", activeJob.TotalEvents,
		"batch_size", len(rows),
	)

	if len(rows) == 0 {
		now := time.Now()
		activeJob.Status = domain.BackfillStatusCompleted
		activeJob.CompletedAt = &now
		if err := updateJobPort.UpdateBackfillJob(ctx, *activeJob); err != nil {
			return fmt.Errorf("complete summary-narrative backfill job: %w", err)
		}
		logger.Logger.InfoContext(ctx, "summary-narrative backfill job completed", "job_id", activeJob.JobID)
		return nil
	}

	for _, row := range rows {
		event := GenerateSummaryNarrativeBackfilledEvent(row)
		if _, err := eventPort.AppendKnowledgeEvent(ctx, event); err != nil {
			return fmt.Errorf("append summary-narrative backfill event: %w", err)
		}
		activeJob.ProcessedEvents++
		// Cursor advances on (generated_at, summary_version_id).
		gen := row.GeneratedAt
		svID := row.SummaryVersionID
		activeJob.CursorDate = &gen
		activeJob.CursorArticleID = &svID
	}

	if activeJob.TotalEvents > 0 && activeJob.ProcessedEvents >= activeJob.TotalEvents {
		now := time.Now()
		activeJob.Status = domain.BackfillStatusCompleted
		activeJob.CompletedAt = &now
	}

	if err := updateJobPort.UpdateBackfillJob(ctx, *activeJob); err != nil {
		return fmt.Errorf("update summary-narrative backfill job: %w", err)
	}
	return nil
}

// GenerateSummaryNarrativeBackfilledEvent (ADR-000846) builds the discovered
// event the projector consumes via its patch-only-why path. OccurredAt is the
// summary's GeneratedAt — the original business-fact time at which the
// summary became available — to honour the canonical contract's event-time
// purity invariant. Wall-clock 'when did the backfill run' belongs in
// projected_at (debug-only) and is intentionally not surfaced here.
func GenerateSummaryNarrativeBackfilledEvent(row domain.KnowledgeBackfillSummaryTitle) domain.KnowledgeEvent {
	payload, _ := json.Marshal(map[string]string{
		"summary_version_id": row.SummaryVersionID.String(),
		"article_id":         row.ArticleID.String(),
		"article_title":      row.Title,
	})
	uid := row.UserID
	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    row.GeneratedAt,
		TenantID:      row.TenantID,
		UserID:        &uid,
		ActorType:     domain.ActorService,
		ActorID:       "summary-narrative-backfill",
		EventType:     domain.EventSummaryNarrativeBackfilled,
		AggregateType: domain.AggregateArticle,
		AggregateID:   row.ArticleID.String(),
		DedupeKey:     fmt.Sprintf("summary-narrative-backfill:%s", row.SummaryVersionID),
		Payload:       payload,
	}
}
