package morning_letter_connect_gateway

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
	"alt/port/morning_letter_port"
)

// Verify interface compliance at compile time.
var _ morning_letter_port.StreamChatPort = (*Gateway)(nil)

// Gateway provides Connect-RPC client for rag-orchestrator MorningLetter service
type Gateway struct {
	client morningletterv2connect.MorningLetterServiceClient
	logger *slog.Logger
}

// NewGateway creates a new MorningLetter Connect-RPC gateway
func NewGateway(baseURL string, logger *slog.Logger) *Gateway {
	httpClient := &http.Client{}
	client := morningletterv2connect.NewMorningLetterServiceClient(
		httpClient,
		baseURL,
		connect.WithGRPC(),
	)
	return &Gateway{
		client: client,
		logger: logger,
	}
}

// StreamChat connects to rag-orchestrator and returns a server stream
func (g *Gateway) StreamChat(
	ctx context.Context,
	messages []*morningletterv2.ChatMessage,
	withinHours int32,
) (*connect.ServerStreamForClient[morningletterv2.StreamChatResponse], error) {
	req := &morningletterv2.StreamChatRequest{
		Messages:    messages,
		WithinHours: withinHours,
	}

	g.logger.Info("calling rag-orchestrator MorningLetter.StreamChat",
		slog.Int("message_count", len(messages)),
		slog.Int("within_hours", int(withinHours)))

	stream, err := g.client.StreamChat(ctx, connect.NewRequest(req))
	if err != nil {
		g.logger.Error("failed to call rag-orchestrator", slog.String("error", err.Error()))
		return nil, err
	}

	return stream, nil
}
