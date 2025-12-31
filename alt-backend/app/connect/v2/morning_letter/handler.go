package morning_letter

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"

	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
	"alt/domain"
	"alt/gateway/morning_letter_connect_gateway"
)

// Handler implements morningletterv2connect.MorningLetterServiceHandler
type Handler struct {
	gateway *morning_letter_connect_gateway.Gateway
	logger  *slog.Logger
}

// Ensure Handler implements the interface
var _ morningletterv2connect.MorningLetterServiceHandler = (*Handler)(nil)

// NewHandler creates a new MorningLetterService handler
func NewHandler(
	gateway *morning_letter_connect_gateway.Gateway,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		gateway: gateway,
		logger:  logger,
	}
}

// StreamChat proxies streaming chat requests to rag-orchestrator
func (h *Handler) StreamChat(
	ctx context.Context,
	req *connect.Request[morningletterv2.StreamChatRequest],
	stream *connect.ServerStream[morningletterv2.StreamChatEvent],
) error {
	// Authentication check (handled by interceptor, but double-check)
	_, err := domain.GetUserFromContext(ctx)
	if err != nil {
		h.logger.Error("authentication failed", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Validate request
	if len(req.Msg.Messages) == 0 {
		h.logger.Warn("no messages in request")
		return connect.NewError(connect.CodeInvalidArgument, nil)
	}

	withinHours := req.Msg.WithinHours
	if withinHours <= 0 {
		withinHours = 24 // Default to 24 hours
	}
	if withinHours > 168 {
		withinHours = 168 // Max 7 days
	}

	h.logger.Info("proxying MorningLetter.StreamChat to rag-orchestrator",
		slog.Int("message_count", len(req.Msg.Messages)),
		slog.Int("within_hours", int(withinHours)))

	// Call rag-orchestrator via gateway
	upstreamStream, err := h.gateway.StreamChat(ctx, req.Msg.Messages, withinHours)
	if err != nil {
		h.logger.Error("failed to connect to rag-orchestrator", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeInternal, err)
	}
	defer func() {
		if closeErr := upstreamStream.Close(); closeErr != nil {
			h.logger.Debug("failed to close upstream stream", slog.String("error", closeErr.Error()))
		}
	}()

	// Proxy events from rag-orchestrator to client
	eventCount := 0
	for upstreamStream.Receive() {
		event := upstreamStream.Msg()

		// Send to downstream client
		if err := stream.Send(event); err != nil {
			h.logger.Error("failed to send event to client", slog.String("error", err.Error()))
			return connect.NewError(connect.CodeInternal, err)
		}
		eventCount++
	}

	// Check for upstream errors
	if err := upstreamStream.Err(); err != nil {
		h.logger.Error("upstream stream error", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeInternal, err)
	}

	h.logger.Info("MorningLetter.StreamChat completed",
		slog.Int("events_sent", eventCount))

	return nil
}
