// Package event_publisher_gateway provides gateway implementation for event publishing.
package event_publisher_gateway

import (
	"context"
	"log/slog"

	"alt/driver/mqhub_connect"
	"alt/port/event_publisher_port"
)

// EventPublisherGateway implements EventPublisherPort using mq-hub.
type EventPublisherGateway struct {
	client *mqhub_connect.Client
	logger *slog.Logger
}

// NewEventPublisherGateway creates a new EventPublisherGateway.
func NewEventPublisherGateway(client *mqhub_connect.Client, logger *slog.Logger) *EventPublisherGateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventPublisherGateway{
		client: client,
		logger: logger,
	}
}

// PublishArticleCreated publishes an ArticleCreated event.
func (g *EventPublisherGateway) PublishArticleCreated(ctx context.Context, event event_publisher_port.ArticleCreatedEvent) error {
	if !g.client.IsEnabled() {
		return nil
	}

	payload := mqhub_connect.ArticleCreatedPayload{
		ArticleID:   event.ArticleID,
		UserID:      event.UserID,
		FeedID:      event.FeedID,
		Title:       event.Title,
		URL:         event.URL,
		Content:     event.Content,
		Tags:        event.Tags,
		PublishedAt: event.PublishedAt,
	}

	messageID, err := g.client.PublishArticleCreated(ctx, payload)
	if err != nil {
		g.logger.Error("failed to publish ArticleCreated event",
			"article_id", event.ArticleID,
			"error", err,
		)
		return err
	}

	g.logger.Info("published ArticleCreated event",
		"article_id", event.ArticleID,
		"message_id", messageID,
	)
	return nil
}

// PublishSummarizeRequested publishes a SummarizeRequested event.
func (g *EventPublisherGateway) PublishSummarizeRequested(ctx context.Context, event event_publisher_port.SummarizeRequestedEvent) error {
	if !g.client.IsEnabled() {
		return nil
	}

	payload := mqhub_connect.SummarizeRequestedPayload{
		ArticleID: event.ArticleID,
		UserID:    event.UserID,
		Title:     event.Title,
		Streaming: event.Streaming,
	}

	messageID, err := g.client.PublishSummarizeRequested(ctx, payload)
	if err != nil {
		g.logger.Error("failed to publish SummarizeRequested event",
			"article_id", event.ArticleID,
			"error", err,
		)
		return err
	}

	g.logger.Info("published SummarizeRequested event",
		"article_id", event.ArticleID,
		"message_id", messageID,
	)
	return nil
}

// PublishIndexArticle publishes an IndexArticle event.
func (g *EventPublisherGateway) PublishIndexArticle(ctx context.Context, event event_publisher_port.IndexArticleEvent) error {
	if !g.client.IsEnabled() {
		return nil
	}

	payload := mqhub_connect.IndexArticlePayload{
		ArticleID: event.ArticleID,
		UserID:    event.UserID,
		FeedID:    event.FeedID,
	}

	messageID, err := g.client.PublishIndexArticle(ctx, payload)
	if err != nil {
		g.logger.Error("failed to publish IndexArticle event",
			"article_id", event.ArticleID,
			"error", err,
		)
		return err
	}

	g.logger.Info("published IndexArticle event",
		"article_id", event.ArticleID,
		"message_id", messageID,
	)
	return nil
}

// IsEnabled returns true if event publishing is enabled.
func (g *EventPublisherGateway) IsEnabled() bool {
	return g.client.IsEnabled()
}
