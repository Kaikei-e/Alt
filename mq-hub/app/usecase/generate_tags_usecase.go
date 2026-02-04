// Package usecase contains business logic for mq-hub.
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"mq-hub/domain"
	"mq-hub/port"
)

const (
	// DefaultTagGenerationTimeoutMs is the default timeout for tag generation.
	DefaultTagGenerationTimeoutMs = 30000
	// ReplyStreamPrefix is the prefix for reply streams.
	ReplyStreamPrefix = "alt:replies:tags:"
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

	// Ensure cleanup of reply stream
	defer func() {
		_ = u.streamPort.DeleteStream(ctx, replyStream)
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

	// Publish request to articles stream
	_, err = u.streamPort.Publish(ctx, domain.StreamKeyArticles, event)
	if err != nil {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    req.ArticleID,
			ErrorMessage: fmt.Sprintf("publish request: %v", err),
		}, fmt.Errorf("publish request: %w", err)
	}

	// Determine timeout
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if req.TimeoutMs <= 0 {
		timeout = time.Duration(DefaultTagGenerationTimeoutMs) * time.Millisecond
	}

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
