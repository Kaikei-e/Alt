// Package domain contains core domain types for mq-hub.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of domain event.
type EventType string

// Event types for the Alt platform.
const (
	// EventTypeArticleCreated is emitted when a new article is saved.
	EventTypeArticleCreated EventType = "ArticleCreated"
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

// Validate checks if the event has all required fields.
func (e *Event) Validate() error {
	if e.EventID == "" {
		return errors.New("event_id is required")
	}
	if e.EventType == "" {
		return errors.New("event_type is required")
	}
	if e.Source == "" {
		return errors.New("source is required")
	}
	if e.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	return nil
}
