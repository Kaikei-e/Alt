package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"search-indexer/usecase"
)

const (
	batchFlushSize     = 10
	batchFlushInterval = 2 * time.Second

	// shutdownFlushTimeout bounds the final flush issued from Stop() so a
	// stuck Meilisearch call cannot hang shutdown forever.
	shutdownFlushTimeout = 10 * time.Second
)

// bufferedArticle pairs a buffered article ID with the Redis Stream message
// ID that produced it, so the message can be XACKed once the batch flush
// that included it durably succeeds -- see
// .claude/rules/event-stream-consumer.md ("ACK after durable write").
type bufferedArticle struct {
	articleID string
	messageID string
}

// ArticleCreatedPayload represents the payload for ArticleCreated and
// ArticleUpdated events. The event names the article; the body is read back
// from alt-backend by article_id, so no content/tags field is decoded even
// while the producer still embeds them.
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
// It buffers article IDs and flushes them in batches to reduce
// per-event Meilisearch round-trips.
type IndexEventHandler struct {
	indexUsecase *usecase.IndexArticlesUsecase
	logger       *slog.Logger
	acker        Acknowledger

	mu      sync.Mutex
	buffer  []bufferedArticle
	timer   *time.Timer
	ctx     context.Context
	cancel  context.CancelFunc
	flushed chan struct{} // closed on each flush for testing
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
		buffer:       make([]bufferedArticle, 0, batchFlushSize),
		ctx:          ctx,
		cancel:       cancel,
		flushed:      make(chan struct{}, 1),
	}
	return h
}

// SetAcker injects the Redis Stream Acknowledger used to XAck message IDs
// once a batch flush durably persists their side effect. Wired
// automatically by consumer.NewConsumer via the AckSetter interface.
func (h *IndexEventHandler) SetAcker(a Acknowledger) {
	h.acker = a
}

// Stop flushes any buffered events with a bounded timeout, then cancels the
// background flush timers and the handler's context. Order matters: the
// previous implementation cancelled the context first, so the final flush
// always ran against an already-canceled context and failed outright --
// see .claude/rules/event-stream-consumer.md shutdown ordering (flush
// before cancel, with its own deadline).
func (h *IndexEventHandler) Stop() {
	h.mu.Lock()
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	h.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownFlushTimeout)
	defer cancel()
	h.flush(shutdownCtx)

	h.cancel()
}

// HandleEvent processes a single event. Article IDs are buffered and
// flushed when the batch reaches batchFlushSize or after batchFlushInterval.
func (h *IndexEventHandler) HandleEvent(ctx context.Context, event Event) error {
	switch event.EventType {
	case "ArticleCreated", "ArticleUpdated":
		// ArticleUpdated shares the payload with ArticleCreated and
		// Meilisearch AddDocuments is upsert-by-primary-key, so re-using the
		// same handler is correct: the freshly fetched body overwrites the
		// existing document atomically. Before this branch existed,
		// ArticleUpdated fell through the default case and the search index
		// silently went stale for every article edit published by alt-backend.
		return h.handleArticleCreated(ctx, event)
	case "IndexArticle":
		return h.handleIndexArticle(ctx, event)
	default:
		h.logger.Warn("unknown event type, skipping",
			"event_type", event.EventType,
			"event_id", event.EventID,
		)
		// Nothing to buffer or retry: ack immediately so this message
		// doesn't sit in the PEL forever and eventually get routed to the
		// DLQ as if it were a poison message.
		h.ack(ctx, []string{event.MessageID})
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

	// Buffer the article ID for batch lookup via API. The event carries no
	// body -- see ADR-000953.
	h.logger.Info("buffering ArticleCreated event",
		"article_id", payload.ArticleID,
		"title", payload.Title,
	)

	h.enqueue(payload.ArticleID, event.MessageID)
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

	h.enqueue(payload.ArticleID, event.MessageID)
	return nil
}

// enqueue adds an article ID to the buffer and triggers a flush if the
// batch size threshold is reached. A timer is started on the first enqueue
// to ensure timely flushing even when events arrive slowly.
func (h *IndexEventHandler) enqueue(articleID, messageID string) {
	h.mu.Lock()
	h.buffer = append(h.buffer, bufferedArticle{articleID: articleID, messageID: messageID})
	size := len(h.buffer)

	if size == 1 {
		// First item in batch: start the flush timer
		h.timer = time.AfterFunc(batchFlushInterval, func() {
			h.flush(h.ctx)
		})
	}
	h.mu.Unlock()

	if size >= batchFlushSize {
		h.flush(h.ctx)
	}
}

// flush sends all buffered article IDs to the usecase in one batch call and
// ACKs their source message IDs only after the write durably succeeds. On
// failure the message IDs are left un-ACKed -- the reclaim loop's
// XAUTOCLAIM sweep redelivers them for a retry (or routes them to the DLQ
// once MaxDeliveries is exceeded), per
// .claude/rules/event-stream-consumer.md.
func (h *IndexEventHandler) flush(ctx context.Context) {
	h.mu.Lock()
	if len(h.buffer) == 0 {
		h.mu.Unlock()
		return
	}
	items := h.buffer
	h.buffer = make([]bufferedArticle, 0, batchFlushSize)
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	h.mu.Unlock()

	// Deduplicate article IDs for the usecase call, but keep every message
	// ID -- including duplicates -- so all of them get ACKed once the batch
	// write is durable.
	seen := make(map[string]struct{}, len(items))
	unique := make([]string, 0, len(items))
	messageIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.messageID != "" {
			messageIDs = append(messageIDs, item.messageID)
		}
		if _, ok := seen[item.articleID]; !ok {
			seen[item.articleID] = struct{}{}
			unique = append(unique, item.articleID)
		}
	}

	h.logger.Info("flushing batch", "count", len(unique))

	result, err := h.indexUsecase.ExecuteBatchArticles(ctx, unique)
	if err != nil {
		h.logger.Error("batch indexing failed", "count", len(unique), "error", err)
		return
	}

	h.logger.Info("batch indexed successfully", "indexed", result.IndexedCount)

	h.ack(ctx, messageIDs)

	// Signal flush completion (non-blocking for tests)
	select {
	case h.flushed <- struct{}{}:
	default:
	}
}

// ack acknowledges message IDs once their processing side effect is
// durable. A no-op if no Acknowledger has been wired (e.g. in unit tests
// that construct the handler directly without going through
// consumer.NewConsumer).
func (h *IndexEventHandler) ack(ctx context.Context, messageIDs []string) {
	if h.acker == nil || len(messageIDs) == 0 {
		return
	}
	if err := h.acker.Ack(ctx, messageIDs...); err != nil {
		h.logger.Error("failed to ack flushed messages", "count", len(messageIDs), "error", err)
	}
}
