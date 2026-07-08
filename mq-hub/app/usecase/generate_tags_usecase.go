// Package usecase contains business logic for mq-hub.
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"mq-hub/domain"
	"mq-hub/port"
)

const (
	// DefaultTagGenerationTimeoutMs is the default timeout for tag generation.
	DefaultTagGenerationTimeoutMs = 60000
	// ReplyStreamPrefix is the prefix for reply streams.
	ReplyStreamPrefix = "alt:replies:tags:"
	// replyStreamCleanupTimeout bounds the reply-stream cleanup's own Redis
	// calls. It is intentionally independent of the request context: on
	// request timeout/cancellation the request ctx is already Done, so using
	// it for cleanup would make the cleanup itself fail every time.
	replyStreamCleanupTimeout = 5 * time.Second
	// replyStreamTTL is a safety-net expiry applied to the reply stream during
	// cleanup. It bounds the key's lifetime even if DeleteStream fails, or a
	// worker replies late and recreates the stream (via XADD) after this
	// cleanup has already run.
	replyStreamTTL = 5 * time.Minute
	// maxTagGenerationTimeoutMs bounds the caller-supplied TimeoutMs so a
	// client cannot force a near-unbounded blocking XREAD (TimeoutMs is
	// int32 milliseconds and would otherwise allow ~24 days).
	maxTagGenerationTimeoutMs = 120_000
)

// GenerateTagsRequest represents a request to generate tags for an article.
type GenerateTagsRequest struct {
	ArticleID string
	Title     string
	Content   string
	FeedID    string
	TimeoutMs int32
}

// GeneratedTag represents a single generated tag.
type GeneratedTag struct {
	ID         string
	Name       string
	Confidence float32
}

// GenerateTagsResponse represents the response from tag generation.
type GenerateTagsResponse struct {
	Success      bool
	ArticleID    string
	Tags         []GeneratedTag
	InferenceMs  float32
	ErrorMessage string
}

// GenerateTagsUsecase handles synchronous tag generation via request-reply pattern.
type GenerateTagsUsecase struct {
	streamPort port.StreamPort
}

// NewGenerateTagsUsecase creates a new GenerateTagsUsecase.
func NewGenerateTagsUsecase(streamPort port.StreamPort) *GenerateTagsUsecase {
	return &GenerateTagsUsecase{streamPort: streamPort}
}

// GenerateTagsForArticle synchronously generates tags for an article.
// Uses request-reply pattern over Redis Streams.
func (u *GenerateTagsUsecase) GenerateTagsForArticle(ctx context.Context, req *GenerateTagsRequest) (*GenerateTagsResponse, error) {
	// Generate correlation ID and reply stream
	correlationID := uuid.New().String()
	replyStream := domain.StreamKey(ReplyStreamPrefix + correlationID)

	// Ensure cleanup of reply stream. Use a context detached from the request
	// ctx so cleanup still runs on timeout/cancellation instead of failing
	// alongside the request (see replyStreamCleanupTimeout doc above).
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), replyStreamCleanupTimeout)
		defer cancel()

		if err := u.streamPort.Expire(cleanupCtx, replyStream, replyStreamTTL); err != nil {
			slog.WarnContext(ctx, "failed to set reply stream TTL during cleanup",
				"stream", replyStream.String(), "error", err)
		}
		if err := u.streamPort.DeleteStream(cleanupCtx, replyStream); err != nil {
			slog.WarnContext(ctx, "failed to delete reply stream during cleanup",
				"stream", replyStream.String(), "error", err)
		}
	}()

	// Build request payload
	payload, err := json.Marshal(map[string]interface{}{
		"article_id": req.ArticleID,
		"title":      req.Title,
		"content":    req.Content,
		"feed_id":    req.FeedID,
	})
	if err != nil {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    req.ArticleID,
			ErrorMessage: fmt.Sprintf("marshal payload: %v", err),
		}, fmt.Errorf("marshal payload: %w", err)
	}

	// Create request event
	event, err := domain.NewEvent(
		domain.EventTypeTagGenerationRequested,
		"mq-hub",
		payload,
		map[string]string{
			"correlation_id": correlationID,
			"reply_to":       replyStream.String(),
		},
	)
	if err != nil {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    req.ArticleID,
			ErrorMessage: fmt.Sprintf("create event: %v", err),
		}, fmt.Errorf("create event: %w", err)
	}

	// Publish request to dedicated tags stream
	_, err = u.streamPort.Publish(ctx, domain.StreamKeyTags, event)
	if err != nil {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    req.ArticleID,
			ErrorMessage: fmt.Sprintf("publish request: %v", err),
		}, fmt.Errorf("publish request: %w", err)
	}

	// Determine timeout, clamped to maxTagGenerationTimeoutMs so a client
	// can't tie up a connection/goroutine with a near-unbounded blocking XREAD.
	timeoutMs := req.TimeoutMs
	switch {
	case timeoutMs <= 0:
		timeoutMs = DefaultTagGenerationTimeoutMs
	case timeoutMs > maxTagGenerationTimeoutMs:
		timeoutMs = maxTagGenerationTimeoutMs
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	// Wait for reply
	replyEvent, err := u.streamPort.SubscribeWithTimeout(ctx, replyStream, timeout)
	if err != nil {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    req.ArticleID,
			ErrorMessage: fmt.Sprintf("timeout waiting for reply: %v", err),
		}, fmt.Errorf("wait for reply: %w", err)
	}

	// Parse reply
	return u.parseReply(req.ArticleID, replyEvent)
}

// parseReply extracts the response from a reply event.
func (u *GenerateTagsUsecase) parseReply(articleID string, event *domain.Event) (*GenerateTagsResponse, error) {
	if event == nil || len(event.Payload) == 0 {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    articleID,
			ErrorMessage: "empty reply",
		}, nil
	}

	var reply struct {
		Success      bool    `json:"success"`
		ArticleID    string  `json:"article_id"`
		InferenceMs  float32 `json:"inference_ms"`
		ErrorMessage string  `json:"error_message"`
		Tags         []struct {
			ID         string  `json:"id"`
			Name       string  `json:"name"`
			Confidence float32 `json:"confidence"`
		} `json:"tags"`
	}

	if err := json.Unmarshal(event.Payload, &reply); err != nil {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    articleID,
			ErrorMessage: fmt.Sprintf("parse reply: %v", err),
		}, nil
	}

	// Convert tags
	tags := make([]GeneratedTag, len(reply.Tags))
	for i, t := range reply.Tags {
		tags[i] = GeneratedTag{
			ID:         t.ID,
			Name:       t.Name,
			Confidence: t.Confidence,
		}
	}

	return &GenerateTagsResponse{
		Success:      reply.Success,
		ArticleID:    reply.ArticleID,
		Tags:         tags,
		InferenceMs:  reply.InferenceMs,
		ErrorMessage: reply.ErrorMessage,
	}, nil
}
