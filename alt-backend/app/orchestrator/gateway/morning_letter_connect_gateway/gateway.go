package morning_letter_connect_gateway

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	"alt/domain"
	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
	"alt/orchestrator/port/morning_letter_port"
	"alt/shared/domain/authcontext"
)

// Verify interface compliance at compile time.
var _ morning_letter_port.StreamChatPort = (*Gateway)(nil)

// Gateway provides Connect-RPC client for rag-orchestrator MorningLetter service
type Gateway struct {
	client morningletterv2connect.MorningLetterServiceClient
	logger *slog.Logger
}

// NewGateway creates a new MorningLetter Connect-RPC gateway. httpClient is
// injected by the DI container (mTLS or plaintext, chosen from the URL
// scheme in rag_module).
func NewGateway(httpClient *http.Client, baseURL string, logger *slog.Logger) *Gateway {
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

	connectReq := connect.NewRequest(req)
	if jwtToken, ok := authcontext.JWTFromContext(ctx); ok && jwtToken != "" {
		connectReq.Header().Set("X-Alt-Backend-Token", jwtToken)
	}
	if user, err := domain.GetUserFromContext(ctx); err == nil && user != nil {
		if uid := user.UserID.String(); uid != "" && uid != "00000000-0000-0000-0000-000000000000" {
			connectReq.Header().Set("X-Alt-User-Id", uid)
		}
		if tid := user.TenantID.String(); tid != "" && tid != "00000000-0000-0000-0000-000000000000" {
			connectReq.Header().Set("X-Alt-Tenant-Id", tid)
		}
	}

	stream, err := g.client.StreamChat(ctx, connectReq)
	if err != nil {
		g.logger.Error("failed to call rag-orchestrator", slog.String("error", err.Error()))
		return nil, err
	}

	return stream, nil
}
