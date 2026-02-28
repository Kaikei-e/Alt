package rag_connect_gateway

import (
	"context"
	"log/slog"
	"net/http"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/port/rag_stream_port"

	"connectrpc.com/connect"
)

// Client provides Connect-RPC client for rag-orchestrator Augur service.
// This client is used by the Augur handler to forward StreamChat requests
// directly to rag-orchestrator using Connect-RPC, avoiding SSE parsing.
type Client struct {
	augurClient augurv2connect.AugurServiceClient
	logger      *slog.Logger
}

// Ensure Client implements rag_stream_port.RagStreamPort
var _ rag_stream_port.RagStreamPort = (*Client)(nil)

// NewClient creates a new Augur Connect-RPC client.
func NewClient(baseURL string, logger *slog.Logger) *Client {
	httpClient := &http.Client{}
	client := augurv2connect.NewAugurServiceClient(
		httpClient,
		baseURL,
		connect.WithGRPC(),
	)
	return &Client{
		augurClient: client,
		logger:      logger,
	}
}

// StreamChat connects to rag-orchestrator and returns a server stream.
// The stream directly provides StreamChatResponse proto messages, eliminating
// the need for SSE parsing.
func (c *Client) StreamChat(
	ctx context.Context,
	req *connect.Request[augurv2.StreamChatRequest],
) (*connect.ServerStreamForClient[augurv2.StreamChatResponse], error) {
	c.logger.Info("calling rag-orchestrator Augur.StreamChat",
		slog.Int("message_count", len(req.Msg.Messages)))

	stream, err := c.augurClient.StreamChat(ctx, req)
	if err != nil {
		c.logger.Error("failed to call rag-orchestrator StreamChat",
			slog.String("error", err.Error()))
		return nil, err
	}

	return stream, nil
}
