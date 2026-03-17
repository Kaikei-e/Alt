package create_summary_version_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/summary_version_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateSummaryVersionUsecase creates a new summary version and emits an event.
type CreateSummaryVersionUsecase struct {
	summaryPort summary_version_port.CreateSummaryVersionPort
	eventPort   knowledge_event_port.AppendKnowledgeEventPort
}

// NewCreateSummaryVersionUsecase creates a new CreateSummaryVersionUsecase.
func NewCreateSummaryVersionUsecase(
	summaryPort summary_version_port.CreateSummaryVersionPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) *CreateSummaryVersionUsecase {
	return &CreateSummaryVersionUsecase{
		summaryPort: summaryPort,
		eventPort:   eventPort,
	}
}

// Execute creates a summary version and appends a SummaryVersionCreated event.
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

	// Emit event
	payload, _ := json.Marshal(map[string]string{
		"summary_version_id": sv.SummaryVersionID.String(),
		"article_id":         sv.ArticleID.String(),
		"model":              sv.Model,
		"prompt_version":     sv.PromptVersion,
	})

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      sv.UserID, // tenant context
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
		// Non-fatal: version was created, event will be retried
		return fmt.Errorf("append summary version event: %w", err)
	}

	return nil
}
