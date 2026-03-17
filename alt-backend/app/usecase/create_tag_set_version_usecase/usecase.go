package create_tag_set_version_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/tag_set_version_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateTagSetVersionUsecase creates a new tag set version and emits an event.
type CreateTagSetVersionUsecase struct {
	tagSetPort tag_set_version_port.CreateTagSetVersionPort
	eventPort  knowledge_event_port.AppendKnowledgeEventPort
}

// NewCreateTagSetVersionUsecase creates a new CreateTagSetVersionUsecase.
func NewCreateTagSetVersionUsecase(
	tagSetPort tag_set_version_port.CreateTagSetVersionPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) *CreateTagSetVersionUsecase {
	return &CreateTagSetVersionUsecase{
		tagSetPort: tagSetPort,
		eventPort:  eventPort,
	}
}

// Execute creates a tag set version and appends a TagSetVersionCreated event.
func (u *CreateTagSetVersionUsecase) Execute(ctx context.Context, tsv domain.TagSetVersion) error {
	if tsv.ArticleID == uuid.Nil {
		return errors.New("article_id is required")
	}
	if len(tsv.TagsJSON) == 0 {
		return errors.New("tags_json is required")
	}

	// Generate ID if not set
	if tsv.TagSetVersionID == uuid.Nil {
		tsv.TagSetVersionID = uuid.New()
	}
	if tsv.GeneratedAt.IsZero() {
		tsv.GeneratedAt = time.Now()
	}

	// Create the version
	if err := u.tagSetPort.CreateTagSetVersion(ctx, tsv); err != nil {
		return fmt.Errorf("create tag set version: %w", err)
	}

	// Emit event
	payload, _ := json.Marshal(map[string]string{
		"tag_set_version_id": tsv.TagSetVersionID.String(),
		"article_id":         tsv.ArticleID.String(),
		"generator":          tsv.Generator,
	})

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tsv.UserID,
		UserID:        &tsv.UserID,
		ActorType:     domain.ActorService,
		ActorID:       "tag-generator",
		EventType:     domain.EventTagSetVersionCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   tsv.ArticleID.String(),
		DedupeKey:     fmt.Sprintf("TagSetVersionCreated:%s", tsv.TagSetVersionID),
		Payload:       payload,
	}

	if err := u.eventPort.AppendKnowledgeEvent(ctx, event); err != nil {
		return fmt.Errorf("append tag set version event: %w", err)
	}

	return nil
}
