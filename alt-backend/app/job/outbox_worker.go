package job

import (
	"alt/driver/alt_db"
	"alt/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
)

// OutboxWorkerJob returns a function suitable for the JobScheduler that
// processes pending outbox events.
func OutboxWorkerJob(repo *alt_db.AltDBRepository, ragIntegration rag_integration_port.RagIntegrationPort) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		processOutboxEvents(ctx, repo, ragIntegration)
		return nil
	}
}

// OutboxWorkerRunner is kept for backward compatibility.
// Deprecated: Use OutboxWorkerJob with JobScheduler instead.
func OutboxWorkerRunner(ctx context.Context, repo *alt_db.AltDBRepository, ragIntegration rag_integration_port.RagIntegrationPort) {
	processOutboxEvents(ctx, repo, ragIntegration)
}

func processOutboxEvents(ctx context.Context, repo *alt_db.AltDBRepository, ragIntegration rag_integration_port.RagIntegrationPort) {
	events, err := repo.FetchAndLockPendingOutboxEvents(ctx, 10)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to fetch pending outbox events", "error", err)
		return
	}

	if len(events) == 0 {
		return
	}

	logger.Logger.InfoContext(ctx, "Processing outbox events", "count", len(events))

	for _, event := range events {
		if event.EventType == "ARTICLE_UPSERT" {
			var upsertInput rag_integration_port.UpsertArticleInput
			if err := json.Unmarshal(event.Payload, &upsertInput); err != nil {
				logger.Logger.ErrorContext(ctx, "Failed to unmarshal outbox event payload", "event_id", event.ID, "error", err)
				updateStatus(ctx, repo, event.ID, "FAILED", err.Error())
				continue
			}

			// Call RAG Orchestrator
			// Step A (direct call) is kept for now, but this worker ensures reliability.
			// It might be redundant if Step A succeeded, but RAG upsert should be idempotent.
			if err := ragIntegration.UpsertArticle(ctx, upsertInput); err != nil {
				logger.Logger.ErrorContext(ctx, "Failed to upsert article to RAG from outbox", "event_id", event.ID, "error", err)
				updateStatus(ctx, repo, event.ID, "FAILED", err.Error())
			} else {
				logger.Logger.InfoContext(ctx, "Successfully processed outbox event", "event_id", event.ID)
				updateStatus(ctx, repo, event.ID, "PROCESSED", "")
			}
		} else {
			logger.Logger.WarnContext(ctx, "Unknown event type", "event_type", event.EventType, "event_id", event.ID)
			updateStatus(ctx, repo, event.ID, "FAILED", "Unknown event type")
		}
	}
}

func updateStatus(ctx context.Context, repo *alt_db.AltDBRepository, id string, status string, errMsg string) {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	if err := repo.UpdateOutboxEventStatus(ctx, id, status, errPtr); err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to update outbox event status", "event_id", id, "status", status, "error", err)
	}
}
