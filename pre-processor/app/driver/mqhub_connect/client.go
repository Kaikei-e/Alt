// Package mqhub_connect provides Connect-RPC client for mq-hub service.
package mqhub_connect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	mqhubv1 "pre-processor/gen/proto/services/mqhub/v1"
	"pre-processor/gen/proto/services/mqhub/v1/mqhubv1connect"
)

// StreamKey constants matching mq-hub domain.
const (
	StreamKeySummaries = "alt:events:summaries"
)

// EventType constants matching mq-hub domain.
const (
	EventTypeArticleSummarized = "ArticleSummarized"
)

// authTransport wraps an http.RoundTripper to inject a Bearer token.
type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// Client provides Connect-RPC client for mq-hub.
type Client struct {
	client  mqhubv1connect.MQHubServiceClient
	enabled bool
}

// NewClient creates a new mq-hub Connect-RPC client.
func NewClient(baseURL string, tokenPath string, enabled bool) (*Client, error) {
	if !enabled {
		return &Client{enabled: false}, nil
	}

	content, err := os.ReadFile(tokenPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to read MQHUB_AUTH_TOKEN_FILE: %w", err)
	}
	token := strings.TrimSpace(string(content))
	if token == "" {
		return nil, fmt.Errorf("token is empty")
	}
	for _, c := range token {
		if c < 32 || c > 126 {
			return nil, fmt.Errorf("token contains non-ASCII or control characters")
		}
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &authTransport{
			base:  http.DefaultTransport,
			token: token,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Disable redirect to prevent secret leak
			return http.ErrUseLastResponse
		},
	}

	client := mqhubv1connect.NewMQHubServiceClient(
		httpClient,
		baseURL,
	)
	return &Client{
		client:  client,
		enabled: true,
	}, nil
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
