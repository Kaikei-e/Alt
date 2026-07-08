// Package domain contains core domain types for mq-hub.
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of domain event.
type EventType string

// Event types for the Alt platform.
const (
	// EventTypeArticleCreated is emitted when a new article is saved.
	EventTypeArticleCreated EventType = "ArticleCreated"
	// EventTypeArticleUpdated is emitted when an existing article's content
	// or tags change. Consumers (search-indexer) upsert by article_id.
	EventTypeArticleUpdated EventType = "ArticleUpdated"
	// EventTypeSummarizeRequested is emitted when summarization is requested.
	EventTypeSummarizeRequested EventType = "SummarizeRequested"
	// EventTypeArticleSummarized is emitted when summarization completes.
	EventTypeArticleSummarized EventType = "ArticleSummarized"
	// EventTypeTagsGenerated is emitted when tags are generated.
	EventTypeTagsGenerated EventType = "TagsGenerated"
	// EventTypeIndexArticle is emitted to trigger article indexing.
	EventTypeIndexArticle EventType = "IndexArticle"
	// EventTypeTagGenerationRequested is emitted for synchronous tag generation requests.
	EventTypeTagGenerationRequested EventType = "TagGenerationRequested"
	// EventTypeTagGenerationCompleted is the reply event for tag generation.
	EventTypeTagGenerationCompleted EventType = "TagGenerationCompleted"
)

// Event represents a domain event to be published to Redis Streams.
type Event struct {
	// EventID is the unique identifier for this event (UUID v4).
	EventID string
	// EventType identifies what kind of event this is.
	EventType EventType
	// Source identifies the service that produced this event.
	Source string
	// CreatedAt is when the event was created.
	CreatedAt time.Time
	// Payload contains the event-specific data (JSON or protobuf bytes).
	Payload []byte
	// Metadata contains additional context (tracing, correlation IDs).
	Metadata map[string]string
}

// NewEvent creates a new Event with a generated UUID and current timestamp.
func NewEvent(eventType EventType, source string, payload []byte, metadata map[string]string) (*Event, error) {
	event := &Event{
		EventID:   uuid.New().String(),
		EventType: eventType,
		Source:    source,
		CreatedAt: time.Now(),
		Payload:   payload,
		Metadata:  metadata,
	}

	if err := event.Validate(); err != nil {
		return nil, err
	}

	return event, nil
}

// ErrInvalidEvent is the sentinel wrapped by every Validate failure so
// callers (e.g. RPC handlers) can classify validation errors via errors.Is
// instead of matching on the error message.
var ErrInvalidEvent = errors.New("invalid event")

// Validate checks if the event has all required fields.
func (e *Event) Validate() error {
	if e.EventID == "" {
		return fmt.Errorf("event_id is required: %w", ErrInvalidEvent)
	}
	if e.EventType == "" {
		return fmt.Errorf("event_type is required: %w", ErrInvalidEvent)
	}
	if e.Source == "" {
		return fmt.Errorf("source is required: %w", ErrInvalidEvent)
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required: %w", ErrInvalidEvent)
	}
	return nil
}
