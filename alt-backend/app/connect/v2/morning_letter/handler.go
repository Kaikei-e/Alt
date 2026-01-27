package morning_letter

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"

	"alt/connect/errorhandler"
	"alt/domain"
	"alt/gateway/morning_letter_connect_gateway"
	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
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
		h.logger.ErrorContext(ctx, "authentication failed", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate request
	if len(req.Msg.Messages) == 0 {
		h.logger.WarnContext(ctx, "no messages in request")
		return connect.NewError(connect.CodeInvalidArgument, nil)
	}

	withinHours := req.Msg.WithinHours
	if withinHours <= 0 {
		withinHours = 24 // Default to 24 hours
	}
	if withinHours > 168 {
		withinHours = 168 // Max 7 days
	}

	h.logger.InfoContext(ctx, "proxying MorningLetter.StreamChat to rag-orchestrator",
		slog.Int("message_count", len(req.Msg.Messages)),
		slog.Int("within_hours", int(withinHours)))

	// Call rag-orchestrator via gateway
	upstreamStream, err := h.gateway.StreamChat(ctx, req.Msg.Messages, withinHours)
	if err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.ConnectUpstream")
	}
	defer func() {
		if closeErr := upstreamStream.Close(); closeErr != nil {
			h.logger.DebugContext(ctx, "failed to close upstream stream", slog.String("error", closeErr.Error()))
		}
	}()

	// Proxy events from rag-orchestrator to client
	eventCount := 0
	for upstreamStream.Receive() {
		event := upstreamStream.Msg()

		// Send to downstream client
		if err := stream.Send(event); err != nil {
			return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.SendEvent")
		}
		eventCount++
	}

	// Check for upstream errors
	if err := upstreamStream.Err(); err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.UpstreamError")
	}

	h.logger.InfoContext(ctx, "MorningLetter.StreamChat completed",
		slog.Int("events_sent", eventCount))

	return nil
}
