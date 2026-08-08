package augur

import (
	"context"
	"log/slog"

	"alt/connect/errorhandler"
	"alt/domain"
	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/orchestrator/port/rag_stream_port"
	"alt/orchestrator/usecase/retrieve_context_usecase"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

// userIDHeader propagates the authenticated caller's UUID to rag-orchestrator
// so persisted Ask Augur conversations can be scoped to a user. alt-backend is
// the JWT trust boundary; rag-orchestrator trusts this header implicitly.
const userIDHeader = "X-Alt-User-Id"

// tenantIDHeader carries the caller's tenant uuid so rag-orchestrator can
// stamp augur.conversation_linked.v1 events with a non-empty tenant_id when
// it forwards them to knowledge-sovereign. Wave 4-A (ADR-000853) made this
// header required end-to-end — Surface Planner v2's resolver multi-tenant
// isolation depends on a physical tenant binding rather than session lookup.
const tenantIDHeader = "X-Alt-Tenant-Id"

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
	user, err := domain.GetUserFromContext(ctx)
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

	// Propagate the authenticated user id to rag-orchestrator so it can scope
	// conversation persistence. Client-provided headers are overwritten.
	req.Header().Set(userIDHeader, user.UserID.String())
	req.Header().Set(tenantIDHeader, user.TenantID.String())

	// Call rag-orchestrator directly via Connect-RPC
	ragStream, err := h.ragStreamPort.StreamChat(ctx, req)
	if err != nil {
		return errorhandler.HandleUpstreamError(ctx, h.logger, err, "StreamChat.RagConnectClient")
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
		return errorhandler.HandleUpstreamError(ctx, h.logger, err, "StreamChat.RagStreamError")
	}

	h.logger.InfoContext(ctx, "stream chat completed")
	return nil
}

// sanitizeMetaEvent returns a defensive deep copy of the meta event so the
// forwarded message is not aliased to the stream's receive buffer.
//
// It used to rebuild the event field by field: a three-field citation
// allowlist (url, title, published_at) inside a hand-built envelope. That
// allowlist is a fossil: before ADR-000154 this handler parsed
// rag-orchestrator's *SSE JSON* envelope, whose context items carried
// ChunkText, ChunkID, Score, DocumentVersion and a Debug object — none of
// which may reach the browser. The Connect-RPC migration deleted the JSON
// parse and mechanically kept the allowlist, which at that moment matched
// Citation exactly and therefore stripped nothing. It stayed a no-op until
// ADR-000926 added `kind` and `ref_id`, after which every meta citation
// silently reached the UI as CITATION_KIND_UNSPECIFIED with an empty ref_id —
// the dead source link ADR-000927 believed it had eliminated.
//
// Cloning the whole event, envelope included, cannot re-expose what the
// allowlist was written to strip: those fields exist only in the retired JSON
// shape and have no counterpart anywhere in alt.augur.v2.StreamChatResponse,
// so rag-orchestrator has no way to put them on this wire. Nor was the
// allowlist load-bearing as a filter — the done event carries the same
// Citation list and is already forwarded untouched. Do not reintroduce a
// field-by-field rebuild at either level; it rots the day a proto field is
// added, which is exactly how the citation allowlist failed.
func (h *Handler) sanitizeMetaEvent(event *augurv2.StreamChatResponse) *augurv2.StreamChatResponse {
	if event.GetMeta() == nil {
		return event
	}

	return proto.Clone(event).(*augurv2.StreamChatResponse)
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
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "RetrieveContext")
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

// ListConversations forwards the caller's chat history request to rag-orchestrator,
// scoped by the authenticated user id header.
func (h *Handler) ListConversations(
	ctx context.Context,
	req *connect.Request[augurv2.ListConversationsRequest],
) (*connect.Response[augurv2.ListConversationsResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	req.Header().Set(userIDHeader, user.UserID.String())
	req.Header().Set(tenantIDHeader, user.TenantID.String())
	resp, err := h.ragStreamPort.ListConversations(ctx, req)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ListConversations")
	}
	return resp, nil
}

// GetConversation forwards a single-conversation read to rag-orchestrator.
//
// rag-orchestrator returns CodeNotFound when (id, user_id) does not match a
// row in augur_conversations — for example when the FE polls a conversation
// id whose insert has not yet been replicated. The previous wrapper turned
// every such response into CodeInternal with an "internal server error
// (caused by: not_found)" body, which the UI rendered as a red
// "Error ID: <hash>" banner. Forward NotFound transparently so the FE can
// render a graceful "conversation not yet available" state. Other upstream
// codes still flow through HandleInternalError so internal details stay
// sanitised.
func (h *Handler) GetConversation(
	ctx context.Context,
	req *connect.Request[augurv2.GetConversationRequest],
) (*connect.Response[augurv2.GetConversationResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	req.Header().Set(userIDHeader, user.UserID.String())
	req.Header().Set(tenantIDHeader, user.TenantID.String())
	resp, err := h.ragStreamPort.GetConversation(ctx, req)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetConversation")
	}
	return resp, nil
}

// DeleteConversation forwards a destructive delete to rag-orchestrator.
func (h *Handler) DeleteConversation(
	ctx context.Context,
	req *connect.Request[augurv2.DeleteConversationRequest],
) (*connect.Response[augurv2.DeleteConversationResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	req.Header().Set(userIDHeader, user.UserID.String())
	req.Header().Set(tenantIDHeader, user.TenantID.String())
	resp, err := h.ragStreamPort.DeleteConversation(ctx, req)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "DeleteConversation")
	}
	return resp, nil
}
