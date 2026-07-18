package job

import (
	"alt/domain"
	"alt/orchestrator/port/rag_integration_port"
	"alt/shared/driver/alt_db"
	"alt/shared/port/knowledge_event_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutboxWorkerJob returns a function suitable for the JobScheduler that
// processes pending outbox events.
func OutboxWorkerJob(repo *alt_db.AltDBRepository, ragIntegration rag_integration_port.RagIntegrationPort, knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processOutboxEvents(ctx, repo, ragIntegration, knowledgeEventPort)
	}
}

func processOutboxEvents(ctx context.Context, repo *alt_db.AltDBRepository, ragIntegration rag_integration_port.RagIntegrationPort, knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort) error {
	events, err := repo.FetchAndLockPendingOutboxEvents(ctx, 10)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to fetch pending outbox events", "error", err)
		return fmt.Errorf("fetch pending outbox events: %w", err)
	}

	if len(events) == 0 {
		return nil
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

			// Fire-and-forget: emit Knowledge Home ArticleCreated event (idempotent via dedupe_key)
			emitArticleCreatedEvent(ctx, knowledgeEventPort, event.Payload)
		} else {
			logger.Logger.WarnContext(ctx, "Unknown event type", "event_type", event.EventType, "event_id", event.ID)
			updateStatus(ctx, repo, event.ID, "FAILED", "Unknown event type")
		}
	}
	return nil
}

// emitArticleCreatedEvent appends a Knowledge Home ArticleCreated event to sovereign-db.
// Uses dedupe_key for idempotency — safe to call on every ARTICLE_UPSERT.
//
// port is a required composition-root dependency (job/registry.go always
// wires container.SovereignClient here). A nil port means DI forgot to wire
// the Knowledge Home event producer — panicking surfaces that immediately
// instead of silently dropping every ArticleCreated event (CLAUDE.md rule 8 /
// ADR-000928 root cause).
func emitArticleCreatedEvent(ctx context.Context, port knowledge_event_port.AppendKnowledgeEventPort, payload []byte) {
	if port == nil {
		panic("outbox_worker: knowledge_event_port.AppendKnowledgeEventPort is nil — the Knowledge Home ArticleCreated producer must be wired at composition root (see .claude/rules/di-wiring.md)")
	}

	var p struct {
		ArticleID string `json:"article_id"`
		URL       string `json:"url"`
		Title     string `json:"title"`
		UserID    string `json:"user_id"`
		// UpdatedAt is stamped once at outbox-enqueue time (save_article_driver.go),
		// i.e. when the article-upsert fact actually occurred. Reused below as
		// PublishedAt instead of re-stamping wall-clock time here: this handler
		// can run at an arbitrary, possibly much later time (worker poll delay,
		// crash-and-reprocess), so reading time.Now() here would make the same
		// event replay to a different PublishedAt each time it's processed.
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to unmarshal outbox payload for knowledge event", "error", err)
		return
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		logger.Logger.WarnContext(ctx, "invalid user_id for knowledge event, skipping", "user_id", p.UserID)
		return
	}

	publishedAt := p.UpdatedAt
	if publishedAt == "" {
		// Only reachable for outbox rows enqueued before this field existed.
		logger.Logger.WarnContext(ctx, "outbox payload missing updated_at, falling back to processing-time wall clock",
			"article_id", p.ArticleID)
		publishedAt = time.Now().Format(time.RFC3339)
	}

	// Marshal through the canonical domain.ArticleCreatedPayload struct so
	// the wire key for the article URL is locked to "url" — using a raw
	// map[string]any literal here historically wrote the legacy "link" key
	// which silently broke the projector (PM-2026-041). The shared struct
	// is the single source of truth for this wire schema.
	eventPayload, err := json.Marshal(domain.ArticleCreatedPayload{
		ArticleID:   p.ArticleID,
		Title:       p.Title,
		PublishedAt: publishedAt,
		TenantID:    p.UserID,
		URL:         p.URL,
	})
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to marshal knowledge ArticleCreated payload, skipping",
			"article_id", p.ArticleID, "error", err)
		return
	}

	kevent := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      userID,
		UserID:        &userID,
		ActorType:     domain.ActorService,
		ActorID:       "outbox-worker",
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   p.ArticleID,
		DedupeKey:     fmt.Sprintf(domain.DedupeKeyArticleCreated, p.ArticleID),
		Payload:       eventPayload,
	}

	if _, err := port.AppendKnowledgeEvent(ctx, kevent); err != nil {
		logger.Logger.WarnContext(ctx, "failed to append knowledge ArticleCreated event (non-fatal)",
			"article_id", p.ArticleID, "error", err)
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
