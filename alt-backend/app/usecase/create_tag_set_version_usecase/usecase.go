package create_tag_set_version_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/tag_set_version_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateTagSetVersionUsecase creates a new tag set version and emits an event.
type CreateTagSetVersionUsecase struct {
	tagSetPort         tag_set_version_port.CreateTagSetVersionPort
	eventPort          knowledge_event_port.AppendKnowledgeEventPort
	markSupersededPort tag_set_version_port.MarkTagSetVersionSupersededPort
}

// NewCreateTagSetVersionUsecase creates a new CreateTagSetVersionUsecase.
func NewCreateTagSetVersionUsecase(
	tagSetPort tag_set_version_port.CreateTagSetVersionPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
	markSupersededPort tag_set_version_port.MarkTagSetVersionSupersededPort,
) *CreateTagSetVersionUsecase {
	return &CreateTagSetVersionUsecase{
		tagSetPort:         tagSetPort,
		eventPort:          eventPort,
		markSupersededPort: markSupersededPort,
	}
}

// Execute creates a tag set version and appends a TagSetVersionCreated event.
// If a previous version exists, it also marks the old versions as superseded
// and emits a TagSetSuperseded event.
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

	// Mark previous versions as superseded and emit TagSetSuperseded event
	if u.markSupersededPort != nil {
		prev, err := u.markSupersededPort.MarkTagSetVersionSuperseded(ctx, tsv.ArticleID, tsv.TagSetVersionID)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to mark tag set version superseded",
				"error", err, "article_id", tsv.ArticleID)
		} else if prev != nil {
			// Extract previous tag names from JSON
			var prevTagNames []string
			var tagItems []struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(prev.TagsJSON, &tagItems); err == nil {
				for _, t := range tagItems {
					if t.Name != "" {
						prevTagNames = append(prevTagNames, t.Name)
					}
				}
			}

			supersedePayload, _ := json.Marshal(map[string]interface{}{
				"article_id":             tsv.ArticleID.String(),
				"new_tag_set_version_id": tsv.TagSetVersionID.String(),
				"old_tag_set_version_id": prev.TagSetVersionID.String(),
				"previous_tags":          prevTagNames,
			})

			supersedeEvent := domain.KnowledgeEvent{
				EventID:       uuid.New(),
				OccurredAt:    time.Now(),
				TenantID:      tsv.UserID,
				UserID:        &tsv.UserID,
				ActorType:     domain.ActorService,
				ActorID:       "tag-generator",
				EventType:     domain.EventTagSetSuperseded,
				AggregateType: domain.AggregateArticle,
				AggregateID:   tsv.ArticleID.String(),
				DedupeKey:     fmt.Sprintf("TagSetSuperseded:%s", tsv.TagSetVersionID),
				Payload:       supersedePayload,
			}

			if err := u.eventPort.AppendKnowledgeEvent(ctx, supersedeEvent); err != nil {
				logger.Logger.ErrorContext(ctx, "failed to append TagSetSuperseded event",
					"error", err, "article_id", tsv.ArticleID)
			}
		}
	}

	return nil
}
