package augur

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/connect/errorhandler"
	"alt/domain"
	"alt/port/rag_integration_port"
	"alt/usecase/answer_chat_usecase"
	"alt/usecase/retrieve_context_usecase"

	"connectrpc.com/connect"
)

// Handler implements augurv2connect.AugurServiceHandler
type Handler struct {
	retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase
	answerChatUsecase      answer_chat_usecase.AnswerChatUsecase
	logger                 *slog.Logger
}

// Ensure Handler implements the interface
var _ augurv2connect.AugurServiceHandler = (*Handler)(nil)

// NewHandler creates a new AugurService handler
func NewHandler(
	retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase,
	answerChatUsecase answer_chat_usecase.AnswerChatUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		retrieveContextUsecase: retrieveContextUsecase,
		answerChatUsecase:      answerChatUsecase,
		logger:                 logger,
	}
}

// StreamChat implements streaming chat with RAG context
func (h *Handler) StreamChat(
	ctx context.Context,
	req *connect.Request[augurv2.StreamChatRequest],
	stream *connect.ServerStream[augurv2.StreamChatEvent],
) error {
	// Authentication check (handled by interceptor, but double-check)
	_, err := domain.GetUserFromContext(ctx)
	if err != nil {
		h.logger.Error("authentication failed", "error", err)
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Extract last user message as query
	var query string
	for i := len(req.Msg.Messages) - 1; i >= 0; i-- {
		if req.Msg.Messages[i].Role == "user" {
			query = req.Msg.Messages[i].Content
			break
		}
	}

	if query == "" {
		h.logger.Warn("no user message found in request")
		return connect.NewError(connect.CodeInvalidArgument, nil)
	}

	h.logger.Info("starting stream chat", "query_length", len(query))

	// Call usecase to get SSE stream
	input := rag_integration_port.AnswerInput{
		Query:  query,
		Stream: true,
	}

	answerChan, err := h.answerChatUsecase.Execute(ctx, input)
	if err != nil {
		return errorhandler.HandleInternalError(h.logger, err, "StreamChat.ExecuteAnswerChat")
	}

	// Process SSE stream and convert to Connect-RPC events
	buffer := ""

	for chunk := range answerChan {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			h.logger.Info("stream chat cancelled by client")
			return nil
		default:
		}

		buffer += chunk

		// Process complete events separated by double newline
		for {
			splitIdx := strings.Index(buffer, "\n\n")
			if splitIdx == -1 {
				break // No complete event yet
			}

			// Extract event string (without the double newline)
			eventStr := buffer[:splitIdx]
			buffer = buffer[splitIdx+2:]

			// Parse SSE event
			event, err := h.parseSSEEvent(eventStr)
			if err != nil {
				h.logger.Warn("failed to parse SSE event", "error", err, "event", eventStr)
				continue
			}

			// Send to stream
			if err := stream.Send(event); err != nil {
				return errorhandler.HandleInternalError(h.logger, err, "StreamChat.SendEvent")
			}
		}
	}

	h.logger.Info("stream chat completed")
	return nil
}

// parseSSEEvent parses an SSE event string and returns a StreamChatEvent
func (h *Handler) parseSSEEvent(eventStr string) (*augurv2.StreamChatEvent, error) {
	lines := strings.Split(eventStr, "\n")

	var eventType string
	var dataPayload string

	for _, line := range lines {
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataPayload = strings.TrimPrefix(line, "data:")
		}
	}

	event := &augurv2.StreamChatEvent{
		Kind: eventType,
	}

	switch eventType {
	case "delta":
		// Text chunk
		event.Payload = &augurv2.StreamChatEvent_Delta{
			Delta: dataPayload,
		}

	case "meta":
		// Parse and sanitize meta event
		meta, err := h.sanitizeMetaPayload(dataPayload)
		if err != nil {
			h.logger.Warn("failed to sanitize meta payload", "error", err)
			// Return empty meta on error
			meta = &augurv2.MetaPayload{Citations: []*augurv2.Citation{}}
		}
		event.Payload = &augurv2.StreamChatEvent_Meta{
			Meta: meta,
		}

	case "done":
		// Parse done payload
		done, err := h.parseDonePayload(dataPayload)
		if err != nil {
			h.logger.Warn("failed to parse done payload", "error", err)
			done = &augurv2.DonePayload{Answer: "", Citations: []*augurv2.Citation{}}
		}
		event.Payload = &augurv2.StreamChatEvent_Done{
			Done: done,
		}

	case "fallback":
		event.Payload = &augurv2.StreamChatEvent_FallbackCode{
			FallbackCode: dataPayload,
		}

	case "error":
		event.Payload = &augurv2.StreamChatEvent_ErrorMessage{
			ErrorMessage: dataPayload,
		}

	default:
		// Unknown event type, treat as delta
		event.Kind = "delta"
		event.Payload = &augurv2.StreamChatEvent_Delta{
			Delta: dataPayload,
		}
	}

	return event, nil
}

// sanitizeMetaPayload parses and sanitizes the meta payload, removing sensitive fields
func (h *Handler) sanitizeMetaPayload(payload string) (*augurv2.MetaPayload, error) {
	// Incoming structure from rag-orchestrator
	type ContextItem struct {
		ChunkText       string  `json:"ChunkText"` // Sensitive - remove
		URL             string  `json:"URL"`
		Title           string  `json:"Title"`
		PublishedAt     string  `json:"PublishedAt"`
		Score           float64 `json:"Score"`
		DocumentVersion int     `json:"DocumentVersion"`
		ChunkID         string  `json:"ChunkID"` // Internal - remove
	}

	type IncomingMeta struct {
		Contexts []ContextItem `json:"Contexts"`
		Debug    interface{}   `json:"Debug"` // Sensitive - remove
	}

	var in IncomingMeta
	if err := json.Unmarshal([]byte(payload), &in); err != nil {
		return nil, err
	}

	// Convert to safe protobuf structure
	meta := &augurv2.MetaPayload{
		Citations: make([]*augurv2.Citation, 0, len(in.Contexts)),
	}

	for _, ctx := range in.Contexts {
		meta.Citations = append(meta.Citations, &augurv2.Citation{
			Url:         ctx.URL,
			Title:       ctx.Title,
			PublishedAt: ctx.PublishedAt,
		})
	}

	return meta, nil
}

// parseDonePayload parses the done payload from SSE
func (h *Handler) parseDonePayload(payload string) (*augurv2.DonePayload, error) {
	type ContextItem struct {
		URL         string `json:"URL"`
		Title       string `json:"Title"`
		PublishedAt string `json:"PublishedAt"`
	}

	type DoneResponse struct {
		Answer   string        `json:"Answer"`
		Contexts []ContextItem `json:"Contexts"`
	}

	var in DoneResponse
	if err := json.Unmarshal([]byte(payload), &in); err != nil {
		return nil, err
	}

	done := &augurv2.DonePayload{
		Answer:    in.Answer,
		Citations: make([]*augurv2.Citation, 0, len(in.Contexts)),
	}

	for _, ctx := range in.Contexts {
		done.Citations = append(done.Citations, &augurv2.Citation{
			Url:         ctx.URL,
			Title:       ctx.Title,
			PublishedAt: ctx.PublishedAt,
		})
	}

	return done, nil
}

// RetrieveContext retrieves relevant context for a query without generating an answer
func (h *Handler) RetrieveContext(
	ctx context.Context,
	req *connect.Request[augurv2.RetrieveContextRequest],
) (*connect.Response[augurv2.RetrieveContextResponse], error) {
	// Authentication check (handled by interceptor, but double-check)
	_, err := domain.GetUserFromContext(ctx)
	if err != nil {
		h.logger.Error("authentication failed", "error", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	query := req.Msg.Query
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	h.logger.Info("retrieving context", "query", query)

	// Call usecase
	contexts, err := h.retrieveContextUsecase.Execute(ctx, query)
	if err != nil {
		return nil, errorhandler.HandleInternalError(h.logger, err, "RetrieveContext")
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
