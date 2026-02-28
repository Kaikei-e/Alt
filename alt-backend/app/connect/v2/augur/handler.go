package augur

import (
	"context"
	"log/slog"

	"alt/connect/errorhandler"
	"alt/domain"
	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/port/rag_stream_port"
	"alt/usecase/retrieve_context_usecase"

	"connectrpc.com/connect"
)

// Handler implements augurv2connect.AugurServiceHandler
type Handler struct {
	retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase
	ragStreamPort          rag_stream_port.RagStreamPort
	logger                 *slog.Logger
}

// Ensure Handler implements the interface
var _ augurv2connect.AugurServiceHandler = (*Handler)(nil)

// NewHandler creates a new AugurService handler
func NewHandler(
	retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase,
	ragStreamPort rag_stream_port.RagStreamPort,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		retrieveContextUsecase: retrieveContextUsecase,
		ragStreamPort:          ragStreamPort,
		logger:                 logger,
	}
}

// StreamChat implements streaming chat with RAG context.
// This method forwards requests directly to rag-orchestrator via Connect-RPC,
// eliminating the need for SSE parsing.
func (h *Handler) StreamChat(
	ctx context.Context,
	req *connect.Request[augurv2.StreamChatRequest],
	stream *connect.ServerStream[augurv2.StreamChatResponse],
) error {
	// Authentication check (handled by interceptor, but double-check)
	_, err := domain.GetUserFromContext(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "authentication failed", "error", err)
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate request has user message
	var query string
	for i := len(req.Msg.Messages) - 1; i >= 0; i-- {
		if req.Msg.Messages[i].Role == "user" {
			query = req.Msg.Messages[i].Content
			break
		}
	}

	if query == "" {
		h.logger.WarnContext(ctx, "no user message found in request")
		return connect.NewError(connect.CodeInvalidArgument, nil)
	}

	h.logger.InfoContext(ctx, "starting stream chat via Connect-RPC", "query_length", len(query))

	// Call rag-orchestrator directly via Connect-RPC
	ragStream, err := h.ragStreamPort.StreamChat(ctx, req)
	if err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.RagConnectClient")
	}
	defer ragStream.Close()

	// Forward events from rag-orchestrator to client
	for ragStream.Receive() {
		event := ragStream.Msg()

		// Sanitize meta payload to remove sensitive data
		if event.Kind == "meta" {
			if meta := event.GetMeta(); meta != nil {
				event = h.sanitizeMetaEvent(event)
			}
		}

		if err := stream.Send(event); err != nil {
			return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.SendEvent")
		}
	}

	if err := ragStream.Err(); err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.RagStreamError")
	}

	h.logger.InfoContext(ctx, "stream chat completed")
	return nil
}

// sanitizeMetaEvent creates a sanitized copy of the meta event,
// keeping only safe fields (URL, Title, PublishedAt) in citations.
func (h *Handler) sanitizeMetaEvent(event *augurv2.StreamChatResponse) *augurv2.StreamChatResponse {
	meta := event.GetMeta()
	if meta == nil {
		return event
	}

	// Create sanitized citations (rag-orchestrator already sends Citation proto,
	// but we re-create to ensure no extra fields leak through)
	sanitizedCitations := make([]*augurv2.Citation, 0, len(meta.Citations))
	for _, c := range meta.Citations {
		sanitizedCitations = append(sanitizedCitations, &augurv2.Citation{
			Url:         c.Url,
			Title:       c.Title,
			PublishedAt: c.PublishedAt,
		})
	}

	return &augurv2.StreamChatResponse{
		Kind: "meta",
		Payload: &augurv2.StreamChatResponse_Meta{
			Meta: &augurv2.MetaPayload{
				Citations: sanitizedCitations,
			},
		},
	}
}

// RetrieveContext retrieves relevant context for a query without generating an answer
func (h *Handler) RetrieveContext(
	ctx context.Context,
	req *connect.Request[augurv2.RetrieveContextRequest],
) (*connect.Response[augurv2.RetrieveContextResponse], error) {
	// Authentication check (handled by interceptor, but double-check)
	_, err := domain.GetUserFromContext(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "authentication failed", "error", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	query := req.Msg.Query
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	h.logger.InfoContext(ctx, "retrieving context", "query", query)

	// Call usecase
	contexts, err := h.retrieveContextUsecase.Execute(ctx, query)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "RetrieveContext")
	}

	// Convert to protobuf response
	resp := &augurv2.RetrieveContextResponse{
		Contexts: make([]*augurv2.ContextItem, 0, len(contexts)),
	}

	for _, c := range contexts {
		publishedAt := ""
		if c.PublishedAt != nil {
			publishedAt = c.PublishedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		resp.Contexts = append(resp.Contexts, &augurv2.ContextItem{
			Url:         c.URL,
			Title:       c.Title,
			PublishedAt: publishedAt,
			Score:       c.Score,
		})
	}

	return connect.NewResponse(resp), nil
}
