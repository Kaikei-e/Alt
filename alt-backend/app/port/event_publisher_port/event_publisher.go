// Package event_publisher_port defines interfaces for event publishing.
package event_publisher_port

import (
	"context"
	"time"
)

// ArticleCreatedEvent represents an article creation event.
type ArticleCreatedEvent struct {
	ArticleID   string
	UserID      string
	FeedID      string
	Title       string
	URL         string
	PublishedAt time.Time
}

// SummarizeRequestedEvent represents a summarization request event.
type SummarizeRequestedEvent struct {
	ArticleID string
	UserID    string
	Title     string
	Streaming bool
}

// IndexArticleEvent represents an article indexing request.
type IndexArticleEvent struct {
	ArticleID string
	UserID    string
	FeedID    string
}

// EventPublisherPort defines the interface for publishing domain events.
type EventPublisherPort interface {
	// PublishArticleCreated publishes an ArticleCreated event.
	PublishArticleCreated(ctx context.Context, event ArticleCreatedEvent) error

	// PublishSummarizeRequested publishes a SummarizeRequested event.
	PublishSummarizeRequested(ctx context.Context, event SummarizeRequestedEvent) error

	// PublishIndexArticle publishes an IndexArticle event.
	PublishIndexArticle(ctx context.Context, event IndexArticleEvent) error

	// IsEnabled returns true if event publishing is enabled.
	IsEnabled() bool
}
