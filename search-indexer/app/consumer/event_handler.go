package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"search-indexer/domain"
	"search-indexer/usecase"
)

const (
	batchFlushSize     = 10
	batchFlushInterval = 2 * time.Second
)

// ArticleCreatedPayload represents the payload for ArticleCreated event.
// Supports fat events with optional content and tags fields.
type ArticleCreatedPayload struct {
	ArticleID   string   `json:"article_id"`
	UserID      string   `json:"user_id"`
	FeedID      string   `json:"feed_id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Content     string   `json:"content,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	PublishedAt string   `json:"published_at"`
}

// IndexArticlePayload represents the payload for IndexArticle event.
type IndexArticlePayload struct {
	ArticleID string `json:"article_id"`
	UserID    string `json:"user_id"`
	FeedID    string `json:"feed_id"`
}

// IndexEventHandler processes indexing events from the stream.
// It buffers article IDs and flushes them in batches to reduce
// per-event Meilisearch round-trips. For fat events with content,
// it indexes directly without API/DB lookups.
type IndexEventHandler struct {
	indexUsecase *usecase.IndexArticlesUsecase
	logger       *slog.Logger

	mu      sync.Mutex
	buffer  []string
	timer   *time.Timer
	ctx     context.Context
	cancel  context.CancelFunc
	flushed chan struct{} // closed on each flush for testing

	// Fat event buffer for direct indexing
	fatMu      sync.Mutex
	fatBuffer  []domain.SearchDocument
	fatTimer   *time.Timer
}

// NewIndexEventHandler creates a new IndexEventHandler.
func NewIndexEventHandler(indexUsecase *usecase.IndexArticlesUsecase, logger *slog.Logger) *IndexEventHandler {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &IndexEventHandler{
		indexUsecase: indexUsecase,
		logger:       logger,
		buffer:       make([]string, 0, batchFlushSize),
		ctx:          ctx,
		cancel:       cancel,
		flushed:      make(chan struct{}, 1),
		fatBuffer:    make([]domain.SearchDocument, 0, batchFlushSize),
	}
	return h
}

// Stop cancels the background flush timer.
func (h *IndexEventHandler) Stop() {
	h.cancel()
	h.mu.Lock()
	if h.timer != nil {
		h.timer.Stop()
	}
	h.mu.Unlock()
	h.fatMu.Lock()
	if h.fatTimer != nil {
		h.fatTimer.Stop()
	}
	h.fatMu.Unlock()
	// Flush remaining
	h.flush()
	h.flushFat()
}

// HandleEvent processes a single event. Article IDs are buffered and
// flushed when the batch reaches batchFlushSize or after batchFlushInterval.
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
		return nil
	}
}

func (h *IndexEventHandler) handleArticleCreated(ctx context.Context, event Event) error {
	var payload ArticleCreatedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		h.logger.Error("failed to unmarshal ArticleCreated payload",
			"event_id", event.EventID,
			"error", err,
		)
		return err
	}

	// Fat event path: if content is present, index directly without DB/API lookup
	if payload.Content != "" {
		h.logger.Info("indexing ArticleCreated fat event directly",
			"article_id", payload.ArticleID,
			"title", payload.Title,
		)
		doc := domain.SearchDocument{
			ID:      payload.ArticleID,
			Title:   payload.Title,
			Content: payload.Content,
			Tags:    payload.Tags,
			UserID:  payload.UserID,
		}
		h.enqueueFatEvent(doc)
		return nil
	}

	// Thin event fallback: buffer article ID for batch lookup via API
	h.logger.Info("buffering ArticleCreated event",
		"article_id", payload.ArticleID,
		"title", payload.Title,
	)

	h.enqueue(payload.ArticleID)
	return nil
}

func (h *IndexEventHandler) handleIndexArticle(ctx context.Context, event Event) error {
	var payload IndexArticlePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		h.logger.Error("failed to unmarshal IndexArticle payload",
			"event_id", event.EventID,
			"error", err,
		)
		return err
	}

	h.logger.Info("buffering IndexArticle event",
		"article_id", payload.ArticleID,
	)

	h.enqueue(payload.ArticleID)
	return nil
}

// enqueue adds an article ID to the buffer and triggers a flush if the
// batch size threshold is reached. A timer is started on the first enqueue
// to ensure timely flushing even when events arrive slowly.
func (h *IndexEventHandler) enqueue(articleID string) {
	h.mu.Lock()
	h.buffer = append(h.buffer, articleID)
	size := len(h.buffer)

	if size == 1 {
		// First item in batch: start the flush timer
		h.timer = time.AfterFunc(batchFlushInterval, func() {
			h.flush()
		})
	}
	h.mu.Unlock()

	if size >= batchFlushSize {
		h.flush()
	}
}

// flush sends all buffered article IDs to the usecase in one batch call.
func (h *IndexEventHandler) flush() {
	h.mu.Lock()
	if len(h.buffer) == 0 {
		h.mu.Unlock()
		return
	}
	ids := h.buffer
	h.buffer = make([]string, 0, batchFlushSize)
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	h.mu.Unlock()

	// Deduplicate IDs within the batch
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}

	h.logger.Info("flushing batch", "count", len(unique))

	result, err := h.indexUsecase.ExecuteBatchArticles(h.ctx, unique)
	if err != nil {
		h.logger.Error("batch indexing failed", "count", len(unique), "error", err)
		return
	}

	h.logger.Info("batch indexed successfully", "indexed", result.IndexedCount)

	// Signal flush completion (non-blocking for tests)
	select {
	case h.flushed <- struct{}{}:
	default:
	}
}

// enqueueFatEvent adds a pre-built search document to the fat event buffer.
func (h *IndexEventHandler) enqueueFatEvent(doc domain.SearchDocument) {
	h.fatMu.Lock()
	h.fatBuffer = append(h.fatBuffer, doc)
	size := len(h.fatBuffer)

	if size == 1 {
		h.fatTimer = time.AfterFunc(batchFlushInterval, func() {
			h.flushFat()
		})
	}
	h.fatMu.Unlock()

	if size >= batchFlushSize {
		h.flushFat()
	}
}

// flushFat sends all buffered fat event documents to the search engine directly.
func (h *IndexEventHandler) flushFat() {
	h.fatMu.Lock()
	if len(h.fatBuffer) == 0 {
		h.fatMu.Unlock()
		return
	}
	docs := h.fatBuffer
	h.fatBuffer = make([]domain.SearchDocument, 0, batchFlushSize)
	if h.fatTimer != nil {
		h.fatTimer.Stop()
		h.fatTimer = nil
	}
	h.fatMu.Unlock()

	// Deduplicate by ID
	seen := make(map[string]struct{}, len(docs))
	unique := make([]domain.SearchDocument, 0, len(docs))
	for _, doc := range docs {
		if _, ok := seen[doc.ID]; !ok {
			seen[doc.ID] = struct{}{}
			unique = append(unique, doc)
		}
	}

	h.logger.Info("flushing fat event batch", "count", len(unique))

	result, err := h.indexUsecase.IndexDocumentsDirectly(h.ctx, unique)
	if err != nil {
		h.logger.Error("fat event batch indexing failed", "count", len(unique), "error", err)
		return
	}

	h.logger.Info("fat event batch indexed successfully", "indexed", result.IndexedCount)

	select {
	case h.flushed <- struct{}{}:
	default:
	}
}
