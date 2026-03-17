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

// KnowledgeBackfillJob returns a function that processes a single batch of the
// oldest running backfill job. Designed to be called by the JobScheduler.
func KnowledgeBackfillJob(
	getJobPort knowledge_backfill_port.GetBackfillJobPort,
	updateJobPort knowledge_backfill_port.UpdateBackfillJobPort,
	listJobsPort knowledge_backfill_port.ListBackfillJobsPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processBackfillBatch(ctx, getJobPort, updateJobPort, listJobsPort, eventPort)
	}
}

func processBackfillBatch(
	ctx context.Context,
	_ knowledge_backfill_port.GetBackfillJobPort,
	updateJobPort knowledge_backfill_port.UpdateBackfillJobPort,
	listJobsPort knowledge_backfill_port.ListBackfillJobsPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) error {
	// Find the first running or pending job
	jobs, err := listJobsPort.ListBackfillJobs(ctx)
	if err != nil {
		return fmt.Errorf("list backfill jobs: %w", err)
	}

	var activeJob *domain.KnowledgeBackfillJob
	for i := range jobs {
		if jobs[i].Status == domain.BackfillStatusRunning || jobs[i].Status == domain.BackfillStatusPending {
			activeJob = &jobs[i]
			break
		}
	}

	if activeJob == nil {
		return nil // No active backfill job
	}

	// If pending, transition to running
	if activeJob.Status == domain.BackfillStatusPending {
		now := time.Now()
		activeJob.Status = domain.BackfillStatusRunning
		activeJob.StartedAt = &now
		if err := updateJobPort.UpdateBackfillJob(ctx, *activeJob); err != nil {
			return fmt.Errorf("start backfill job: %w", err)
		}
	}

	// Generate a synthetic ArticleCreated event as a placeholder batch step
	// Real implementation would query historical articles, summaries, tags
	// and emit corresponding events in batches
	logger.Logger.InfoContext(ctx, "backfill job processing",
		"job_id", activeJob.JobID,
		"processed", activeJob.ProcessedEvents,
		"total", activeJob.TotalEvents,
	)

	// If no work remaining, mark completed
	if activeJob.ProcessedEvents >= activeJob.TotalEvents && activeJob.TotalEvents > 0 {
		now := time.Now()
		activeJob.Status = domain.BackfillStatusCompleted
		activeJob.CompletedAt = &now
		if err := updateJobPort.UpdateBackfillJob(ctx, *activeJob); err != nil {
			return fmt.Errorf("complete backfill job: %w", err)
		}
		logger.Logger.InfoContext(ctx, "backfill job completed", "job_id", activeJob.JobID)
	}

	_ = eventPort // Used during actual event generation

	return nil
}

// GenerateBackfillEvent creates a synthetic event for backfill purposes.
// The dedupe_key ensures idempotency if the same article is backfilled again.
func GenerateBackfillEvent(tenantID uuid.UUID, userID *uuid.UUID, articleID uuid.UUID, title string, publishedAt time.Time) domain.KnowledgeEvent {
	payload, _ := json.Marshal(articleCreatedPayload{
		ArticleID:   articleID.String(),
		Title:       title,
		PublishedAt: publishedAt.Format(time.RFC3339),
		TenantID:    tenantID.String(),
	})

	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tenantID,
		UserID:        userID,
		ActorType:     domain.ActorService,
		ActorID:       "backfill",
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   articleID.String(),
		DedupeKey:     fmt.Sprintf("backfill:%s", articleID),
		Payload:       payload,
	}
}
