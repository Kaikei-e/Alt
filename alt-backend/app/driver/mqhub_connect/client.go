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
	EventTypeArticleCreated     = "ArticleCreated"
	EventTypeSummarizeRequested = "SummarizeRequested"
	EventTypeIndexArticle       = "IndexArticle"
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
