// Package mqhub_connect provides Connect-RPC client for mq-hub service.
package mqhub_connect

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	mqhubv1 "alt/gen/proto/clients/mqhub/v1"
	"alt/gen/proto/clients/mqhub/v1/mqhubv1connect"
)

// StreamKey constants matching mq-hub domain.
const (
	StreamKeyArticles  = "alt:events:articles"
	StreamKeySummaries = "alt:events:summaries"
	StreamKeyTags      = "alt:events:tags"
	StreamKeyIndex     = "alt:events:index"
)

// EventType constants matching mq-hub domain.
const (
	EventTypeArticleCreated            = "ArticleCreated"
	EventTypeSummarizeRequested        = "SummarizeRequested"
	EventTypeIndexArticle              = "IndexArticle"
	EventTypeTagGenerationRequested    = "TagGenerationRequested"
	EventTypeTagGenerationCompleted    = "TagGenerationCompleted"
)

// Client provides Connect-RPC client for mq-hub.
type Client struct {
	client  mqhubv1connect.MQHubServiceClient
	enabled bool
}

// NewClient creates a new mq-hub Connect-RPC client.
func NewClient(baseURL string, enabled bool) *Client {
	if !enabled {
		return &Client{enabled: false}
	}

	client := mqhubv1connect.NewMQHubServiceClient(
		http.DefaultClient,
		baseURL,
	)
	return &Client{
		client:  client,
		enabled: true,
	}
}

// IsEnabled returns true if the client is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// ArticleCreatedPayload represents the payload for ArticleCreated event.
type ArticleCreatedPayload struct {
	ArticleID   string    `json:"article_id"`
	UserID      string    `json:"user_id"`
	FeedID      string    `json:"feed_id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Content     string    `json:"content,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

// SummarizeRequestedPayload represents the payload for SummarizeRequested event.
type SummarizeRequestedPayload struct {
	ArticleID string `json:"article_id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	Streaming bool   `json:"streaming"`
}

// IndexArticlePayload represents the payload for IndexArticle event.
type IndexArticlePayload struct {
	ArticleID string `json:"article_id"`
	UserID    string `json:"user_id"`
	FeedID    string `json:"feed_id"`
}

// PublishArticleCreated publishes an ArticleCreated event.
func (c *Client) PublishArticleCreated(ctx context.Context, payload ArticleCreatedPayload) (string, error) {
	if !c.enabled {
		return "", nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	event := &mqhubv1.Event{
		EventId:   uuid.New().String(),
		EventType: EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: timestamppb.Now(),
		Payload:   payloadBytes,
		Metadata:  map[string]string{},
	}

	resp, err := c.client.Publish(ctx, connect.NewRequest(&mqhubv1.PublishRequest{
		Stream: StreamKeyArticles,
		Event:  event,
	}))
	if err != nil {
		return "", err
	}

	return resp.Msg.MessageId, nil
}

// PublishSummarizeRequested publishes a SummarizeRequested event.
func (c *Client) PublishSummarizeRequested(ctx context.Context, payload SummarizeRequestedPayload) (string, error) {
	if !c.enabled {
		return "", nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	event := &mqhubv1.Event{
		EventId:   uuid.New().String(),
		EventType: EventTypeSummarizeRequested,
		Source:    "alt-backend",
		CreatedAt: timestamppb.Now(),
		Payload:   payloadBytes,
		Metadata:  map[string]string{},
	}

	resp, err := c.client.Publish(ctx, connect.NewRequest(&mqhubv1.PublishRequest{
		Stream: StreamKeyArticles,
		Event:  event,
	}))
	if err != nil {
		return "", err
	}

	return resp.Msg.MessageId, nil
}

// PublishIndexArticle publishes an IndexArticle event.
func (c *Client) PublishIndexArticle(ctx context.Context, payload IndexArticlePayload) (string, error) {
	if !c.enabled {
		return "", nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	event := &mqhubv1.Event{
		EventId:   uuid.New().String(),
		EventType: EventTypeIndexArticle,
		Source:    "alt-backend",
		CreatedAt: timestamppb.Now(),
		Payload:   payloadBytes,
		Metadata:  map[string]string{},
	}

	resp, err := c.client.Publish(ctx, connect.NewRequest(&mqhubv1.PublishRequest{
		Stream: StreamKeyIndex,
		Event:  event,
	}))
	if err != nil {
		return "", err
	}

	return resp.Msg.MessageId, nil
}

// GenerateTagsRequest represents a request for synchronous tag generation.
type GenerateTagsRequest struct {
	ArticleID string
	Title     string
	Content   string
	FeedID    string
	TimeoutMs int32
}

// GeneratedTag represents a generated tag with confidence.
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

// GenerateTagsForArticle synchronously generates tags for an article via mq-hub.
// Returns the generated tags or an error if the operation fails.
func (c *Client) GenerateTagsForArticle(ctx context.Context, req GenerateTagsRequest) (*GenerateTagsResponse, error) {
	if !c.enabled {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    req.ArticleID,
			ErrorMessage: "mq-hub client is disabled",
		}, nil
	}

	protoReq := &mqhubv1.GenerateTagsForArticleRequest{
		ArticleId: req.ArticleID,
		Title:     req.Title,
		Content:   req.Content,
		FeedId:    req.FeedID,
		TimeoutMs: req.TimeoutMs,
	}

	resp, err := c.client.GenerateTagsForArticle(ctx, connect.NewRequest(protoReq))
	if err != nil {
		return &GenerateTagsResponse{
			Success:      false,
			ArticleID:    req.ArticleID,
			ErrorMessage: err.Error(),
		}, err
	}

	// Convert proto tags to domain tags
	tags := make([]GeneratedTag, len(resp.Msg.Tags))
	for i, protoTag := range resp.Msg.Tags {
		tags[i] = GeneratedTag{
			ID:         protoTag.Id,
			Name:       protoTag.Name,
			Confidence: protoTag.Confidence,
		}
	}

	return &GenerateTagsResponse{
		Success:      resp.Msg.Success,
		ArticleID:    resp.Msg.ArticleId,
		Tags:         tags,
		InferenceMs:  resp.Msg.InferenceMs,
		ErrorMessage: resp.Msg.ErrorMessage,
	}, nil
}
