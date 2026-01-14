package consumer

import (
	"context"
	"encoding/json"
	"log/slog"

	"search-indexer/usecase"
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

// IndexArticlePayload represents the payload for IndexArticle event.
type IndexArticlePayload struct {
	ArticleID string `json:"article_id"`
	UserID    string `json:"user_id"`
	FeedID    string `json:"feed_id"`
}

// IndexEventHandler processes indexing events from the stream.
type IndexEventHandler struct {
	indexUsecase *usecase.IndexArticlesUsecase
	logger       *slog.Logger
}

// NewIndexEventHandler creates a new IndexEventHandler.
func NewIndexEventHandler(indexUsecase *usecase.IndexArticlesUsecase, logger *slog.Logger) *IndexEventHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &IndexEventHandler{
		indexUsecase: indexUsecase,
		logger:       logger,
	}
}

// HandleEvent processes a single event.
func (h *IndexEventHandler) HandleEvent(ctx context.Context, event Event) error {
	switch event.EventType {
	case "ArticleCreated":
		return h.handleArticleCreated(ctx, event)
	case "IndexArticle":
		return h.handleIndexArticle(ctx, event)
	default:
		h.logger.Warn("unknown event type, skipping",
			"event_type", event.EventType,
			"event_id", event.EventID,
		)
		return nil // Return nil to ACK unknown events
	}
}

// handleArticleCreated processes ArticleCreated events.
func (h *IndexEventHandler) handleArticleCreated(ctx context.Context, event Event) error {
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

	// Index the single article by its ID
	result, err := h.indexUsecase.ExecuteSingleArticle(ctx, payload.ArticleID)
	if err != nil {
		h.logger.Error("failed to index article",
			"article_id", payload.ArticleID,
			"error", err,
		)
		return err
	}

	h.logger.Info("article indexed successfully",
		"article_id", payload.ArticleID,
		"indexed", result.IndexedCount,
	)
	return nil
}

// handleIndexArticle processes IndexArticle events.
func (h *IndexEventHandler) handleIndexArticle(ctx context.Context, event Event) error {
	var payload IndexArticlePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		h.logger.Error("failed to unmarshal IndexArticle payload",
			"event_id", event.EventID,
			"error", err,
		)
		return err
	}

	h.logger.Info("processing IndexArticle event",
		"article_id", payload.ArticleID,
	)

	// Index the single article by its ID
	result, err := h.indexUsecase.ExecuteSingleArticle(ctx, payload.ArticleID)
	if err != nil {
		h.logger.Error("failed to index article",
			"article_id", payload.ArticleID,
			"error", err,
		)
		return err
	}

	h.logger.Info("article indexed successfully",
		"article_id", payload.ArticleID,
		"indexed", result.IndexedCount,
	)
	return nil
}
