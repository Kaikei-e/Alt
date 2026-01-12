// Package consumer provides event handling for pre-processor.
package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
)

// EventType constants.
const (
	EventTypeArticleCreated     = "ArticleCreated"
	EventTypeSummarizeRequested = "SummarizeRequested"
)

// ArticleCreatedPayload represents the payload for ArticleCreated event.
type ArticleCreatedPayload struct {
	ArticleID   string `json:"article_id"`
	UserID      string `json:"user_id"`
	FeedID      string `json:"feed_id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at"`
}

// SummarizeRequestedPayload represents the payload for SummarizeRequested event.
type SummarizeRequestedPayload struct {
	ArticleID string `json:"article_id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	Streaming bool   `json:"streaming"`
}

// SummarizeService defines the interface for summarization operations.
type SummarizeService interface {
	// SummarizeArticle processes an article for summarization.
	SummarizeArticle(ctx context.Context, articleID, title string) error
}

// PreProcessorEventHandler handles events for pre-processor service.
type PreProcessorEventHandler struct {
	summarizeService SummarizeService
	logger           *slog.Logger
}

// NewPreProcessorEventHandler creates a new PreProcessorEventHandler.
func NewPreProcessorEventHandler(summarizeService SummarizeService, logger *slog.Logger) *PreProcessorEventHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PreProcessorEventHandler{
		summarizeService: summarizeService,
		logger:           logger,
	}
}

// HandleEvent processes a single event based on its type.
func (h *PreProcessorEventHandler) HandleEvent(ctx context.Context, event Event) error {
	h.logger.Info("handling event",
		"event_id", event.EventID,
		"event_type", event.EventType,
		"message_id", event.MessageID,
	)

	switch event.EventType {
	case EventTypeArticleCreated:
		return h.handleArticleCreated(ctx, event)
	case EventTypeSummarizeRequested:
		return h.handleSummarizeRequested(ctx, event)
	default:
		h.logger.Debug("ignoring unknown event type", "event_type", event.EventType)
		return nil
	}
}

// handleArticleCreated processes an ArticleCreated event.
func (h *PreProcessorEventHandler) handleArticleCreated(ctx context.Context, event Event) error {
	var payload ArticleCreatedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		h.logger.Error("failed to unmarshal ArticleCreated payload",
			"event_id", event.EventID,
			"error", err,
		)
		return err
	}

	h.logger.Info("processing ArticleCreated event",
		"article_id", payload.ArticleID,
		"title", payload.Title,
	)

	// Queue article for summarization
	if err := h.summarizeService.SummarizeArticle(ctx, payload.ArticleID, payload.Title); err != nil {
		h.logger.Error("failed to summarize article",
			"article_id", payload.ArticleID,
			"error", err,
		)
		return err
	}

	return nil
}

// handleSummarizeRequested processes a SummarizeRequested event.
func (h *PreProcessorEventHandler) handleSummarizeRequested(ctx context.Context, event Event) error {
	var payload SummarizeRequestedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		h.logger.Error("failed to unmarshal SummarizeRequested payload",
			"event_id", event.EventID,
			"error", err,
		)
		return err
	}

	h.logger.Info("processing SummarizeRequested event",
		"article_id", payload.ArticleID,
		"title", payload.Title,
		"streaming", payload.Streaming,
	)

	// Process summarization request
	if err := h.summarizeService.SummarizeArticle(ctx, payload.ArticleID, payload.Title); err != nil {
		h.logger.Error("failed to summarize article",
			"article_id", payload.ArticleID,
			"error", err,
		)
		return err
	}

	return nil
}
