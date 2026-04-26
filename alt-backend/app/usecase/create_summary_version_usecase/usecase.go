package create_summary_version_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/summary_version_port"
	"alt/utils/logger"
	"alt/utils/textutil"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const maxPreviousExcerptLen = 200

// CreateSummaryVersionUsecase creates a new summary version and emits an event.
type CreateSummaryVersionUsecase struct {
	summaryPort        summary_version_port.CreateSummaryVersionPort
	eventPort          knowledge_event_port.AppendKnowledgeEventPort
	markSupersededPort summary_version_port.MarkSummaryVersionSupersededPort
}

// NewCreateSummaryVersionUsecase creates a new CreateSummaryVersionUsecase.
func NewCreateSummaryVersionUsecase(
	summaryPort summary_version_port.CreateSummaryVersionPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
	markSupersededPort summary_version_port.MarkSummaryVersionSupersededPort,
) *CreateSummaryVersionUsecase {
	return &CreateSummaryVersionUsecase{
		summaryPort:        summaryPort,
		eventPort:          eventPort,
		markSupersededPort: markSupersededPort,
	}
}

// Execute creates a summary version and appends a SummaryVersionCreated event.
// If a previous version exists, it also marks the old versions as superseded
// and emits a SummarySuperseded event.
func (u *CreateSummaryVersionUsecase) Execute(ctx context.Context, sv domain.SummaryVersion) error {
	if sv.ArticleID == uuid.Nil {
		return errors.New("article_id is required")
	}
	if sv.SummaryText == "" {
		return errors.New("summary_text is required")
	}

	// Generate ID if not set
	if sv.SummaryVersionID == uuid.Nil {
		sv.SummaryVersionID = uuid.New()
	}
	if sv.GeneratedAt.IsZero() {
		sv.GeneratedAt = time.Now()
	}

	// Create the version
	if err := u.summaryPort.CreateSummaryVersion(ctx, sv); err != nil {
		return fmt.Errorf("create summary version: %w", err)
	}

	// Emit SummaryVersionCreated event. article_title is captured into the
	// payload so the Knowledge Loop projector's reproject-safe enricher
	// (knowledge-sovereign/usecase/knowledge_loop_projector/enricher.go) can
	// render a substantive narrative without a latest-state lookup. Empty
	// title falls back to the generic fallback narrative — both branches stay
	// pure on the projector side.
	payloadFields := map[string]string{
		"summary_version_id": sv.SummaryVersionID.String(),
		"article_id":         sv.ArticleID.String(),
		"model":              sv.Model,
		"prompt_version":     sv.PromptVersion,
	}
	if sv.ArticleTitle != "" {
		payloadFields["article_title"] = sv.ArticleTitle
	}
	payload, _ := json.Marshal(payloadFields)

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      sv.UserID,
		UserID:        &sv.UserID,
		ActorType:     domain.ActorService,
		ActorID:       "news-creator",
		EventType:     domain.EventSummaryVersionCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   sv.ArticleID.String(),
		DedupeKey:     fmt.Sprintf("SummaryVersionCreated:%s", sv.SummaryVersionID),
		Payload:       payload,
	}

	if err := u.eventPort.AppendKnowledgeEvent(ctx, event); err != nil {
		return fmt.Errorf("append summary version event: %w", err)
	}

	// Mark previous versions as superseded and emit SummarySuperseded event
	if u.markSupersededPort != nil {
		prev, err := u.markSupersededPort.MarkSummaryVersionSuperseded(ctx, sv.ArticleID, sv.SummaryVersionID)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to mark summary version superseded",
				"error", err, "article_id", sv.ArticleID)
			// Non-fatal: the version was already created
		} else if prev != nil {
			// Previous version existed — emit SummarySuperseded event
			excerpt := textutil.TruncateValidUTF8(prev.SummaryText, maxPreviousExcerptLen)

			supersedePayload, _ := json.Marshal(map[string]string{
				"article_id":               sv.ArticleID.String(),
				"new_summary_version_id":   sv.SummaryVersionID.String(),
				"old_summary_version_id":   prev.SummaryVersionID.String(),
				"previous_summary_excerpt": excerpt,
			})

			supersedeEvent := domain.KnowledgeEvent{
				EventID:       uuid.New(),
				OccurredAt:    time.Now(),
				TenantID:      sv.UserID,
				UserID:        &sv.UserID,
				ActorType:     domain.ActorService,
				ActorID:       "news-creator",
				EventType:     domain.EventSummarySuperseded,
				AggregateType: domain.AggregateArticle,
				AggregateID:   sv.ArticleID.String(),
				DedupeKey:     fmt.Sprintf("SummarySuperseded:%s", sv.SummaryVersionID),
				Payload:       supersedePayload,
			}

			if err := u.eventPort.AppendKnowledgeEvent(ctx, supersedeEvent); err != nil {
				logger.Logger.ErrorContext(ctx, "failed to append SummarySuperseded event",
					"error", err, "article_id", sv.ArticleID)
			}
		}
	}

	return nil
}
