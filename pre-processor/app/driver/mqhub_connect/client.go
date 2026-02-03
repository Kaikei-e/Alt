// Package mqhub_connect provides Connect-RPC client for mq-hub service.
package mqhub_connect

import (
	"context"
	"encoding/json"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	mqhubv1 "pre-processor/gen/proto/clients/mqhub/v1"
	"pre-processor/gen/proto/clients/mqhub/v1/mqhubv1connect"
)

// StreamKey constants matching mq-hub domain.
const (
	StreamKeySummaries = "alt:events:summaries"
)

// EventType constants matching mq-hub domain.
const (
	EventTypeArticleSummarized = "ArticleSummarized"
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

// ArticleSummarizedPayload represents the payload for ArticleSummarized event.
type ArticleSummarizedPayload struct {
	ArticleID string `json:"article_id"`
	UserID    string `json:"user_id"`
	Summary   string `json:"summary"`
}

// PublishArticleSummarized publishes an ArticleSummarized event.
func (c *Client) PublishArticleSummarized(ctx context.Context, payload ArticleSummarizedPayload) (string, error) {
	if !c.enabled {
		return "", nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	event := &mqhubv1.Event{
		EventId:   uuid.New().String(),
		EventType: EventTypeArticleSummarized,
		Source:    "pre-processor",
		CreatedAt: timestamppb.Now(),
		Payload:   payloadBytes,
		Metadata:  map[string]string{},
	}

	resp, err := c.client.Publish(ctx, connect.NewRequest(&mqhubv1.PublishRequest{
		Stream: StreamKeySummaries,
		Event:  event,
	}))
	if err != nil {
		return "", err
	}

	return resp.Msg.MessageId, nil
}
