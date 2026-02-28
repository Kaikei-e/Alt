package augur

import (
	"context"
	"log/slog"
	"strings"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"

	"rag-orchestrator/internal/usecase"

	"connectrpc.com/connect"
)

// sanitizeUTF8 removes invalid UTF-8 sequences from a string.
// This is necessary because Ollama LLM may return chunks containing
// invalid UTF-8, which causes protobuf serialization to fail with
// "string field contains invalid UTF-8" errors.
func sanitizeUTF8(s string) string {
	return strings.ToValidUTF8(s, "")
}

// Handler implements augurv2connect.AugurServiceHandler
type Handler struct {
	answerUsecase   usecase.AnswerWithRAGUsecase
	retrieveUsecase usecase.RetrieveContextUsecase
	logger          *slog.Logger
}

// Ensure Handler implements the interface
var _ augurv2connect.AugurServiceHandler = (*Handler)(nil)

// NewHandler creates a new AugurService handler
func NewHandler(
	answerUsecase usecase.AnswerWithRAGUsecase,
	retrieveUsecase usecase.RetrieveContextUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		answerUsecase:   answerUsecase,
		retrieveUsecase: retrieveUsecase,
		logger:          logger,
	}
}

// StreamChat implements streaming RAG chat
func (h *Handler) StreamChat(
	ctx context.Context,
	req *connect.Request[augurv2.StreamChatRequest],
	stream *connect.ServerStream[augurv2.StreamChatResponse],
) error {
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

	h.logger.Info("starting augur stream chat",
		slog.String("query", query))

	// Build input for AnswerWithRAGUsecase
	input := usecase.AnswerWithRAGInput{
		Query:  query,
		Locale: "ja", // Default to Japanese
	}

	// Stream answer using AnswerWithRAGUsecase
	events := h.answerUsecase.Stream(ctx, input)

	// Process stream events and convert to Connect-RPC events
	for event := range events {
		select {
		case <-ctx.Done():
			h.logger.Info("stream chat cancelled by client")
			return nil
		default:
		}

		protoEvent, shouldContinue := h.convertStreamEvent(event)
		if protoEvent != nil {
			if err := stream.Send(protoEvent); err != nil {
				h.logger.Error("failed to send event", slog.String("error", err.Error()))
				return connect.NewError(connect.CodeInternal, err)
			}
		}

		if !shouldContinue {
			break
		}
	}

	h.logger.Info("augur stream chat completed")
	return nil
}

// convertStreamEvent converts usecase.StreamEvent to augurv2.StreamChatResponse
func (h *Handler) convertStreamEvent(event usecase.StreamEvent) (*augurv2.StreamChatResponse, bool) {
	switch event.Kind {
	case usecase.StreamEventKindDelta:
		delta, ok := event.Payload.(string)
		if !ok {
			return nil, true
		}
		return &augurv2.StreamChatResponse{
			Kind: "delta",
			Payload: &augurv2.StreamChatResponse_Delta{
				Delta: sanitizeUTF8(delta),
			},
		}, true

	case usecase.StreamEventKindMeta:
		meta, ok := event.Payload.(usecase.StreamMeta)
		if !ok {
			return nil, true
		}
		citations := h.convertContextsToCitations(meta.Contexts)
		return &augurv2.StreamChatResponse{
			Kind: "meta",
			Payload: &augurv2.StreamChatResponse_Meta{
				Meta: &augurv2.MetaPayload{
					Citations: citations,
				},
			},
		}, true

	case usecase.StreamEventKindDone:
		output, ok := event.Payload.(*usecase.AnswerWithRAGOutput)
		if !ok {
			return nil, false
		}
		// ADR 000093: Use output.Citations (only citations LLM actually used)
		// instead of output.Contexts (all search results)
		citations := h.convertCitationsToProtoCitations(output.Citations)
		return &augurv2.StreamChatResponse{
			Kind: "done",
			Payload: &augurv2.StreamChatResponse_Done{
				Done: &augurv2.DonePayload{
					Answer:    sanitizeUTF8(output.Answer),
					Citations: citations,
				},
			},
		}, false

	case usecase.StreamEventKindFallback:
		reason, _ := event.Payload.(string)
		return &augurv2.StreamChatResponse{
			Kind: "fallback",
			Payload: &augurv2.StreamChatResponse_FallbackCode{
				FallbackCode: sanitizeUTF8(reason),
			},
		}, false

	case usecase.StreamEventKindError:
		errMsg, _ := event.Payload.(string)
		return &augurv2.StreamChatResponse{
			Kind: "error",
			Payload: &augurv2.StreamChatResponse_ErrorMessage{
				ErrorMessage: sanitizeUTF8(errMsg),
			},
		}, false

	case usecase.StreamEventKindThinking:
		thinking, ok := event.Payload.(string)
		if !ok {
			return nil, true
		}
		return &augurv2.StreamChatResponse{
			Kind: "thinking",
			Payload: &augurv2.StreamChatResponse_ThinkingDelta{
				ThinkingDelta: sanitizeUTF8(thinking),
			},
		}, true

	case usecase.StreamEventKindHeartbeat:
		return &augurv2.StreamChatResponse{
			Kind: "heartbeat",
			Payload: &augurv2.StreamChatResponse_Delta{
				Delta: "",
			},
		}, true

	case usecase.StreamEventKindProgress:
		progress, ok := event.Payload.(string)
		if !ok {
			return nil, true
		}
		// Reuse delta payload as carrier for progress messages (e.g. "searching", "generating").
		// The kind field distinguishes this from actual content deltas.
		return &augurv2.StreamChatResponse{
			Kind: "progress",
			Payload: &augurv2.StreamChatResponse_Delta{
				Delta: sanitizeUTF8(progress),
			},
		}, true

	default:
		h.logger.Warn("unknown stream event kind", slog.String("kind", string(event.Kind)))
		return nil, true
	}
}

// convertContextsToCitations converts usecase.ContextItem slice to augurv2.Citation slice
// Used for Meta event (all search results as potential citations)
func (h *Handler) convertContextsToCitations(contexts []usecase.ContextItem) []*augurv2.Citation {
	citations := make([]*augurv2.Citation, 0, len(contexts))
	for _, ctx := range contexts {
		citations = append(citations, &augurv2.Citation{
			Url:         sanitizeUTF8(ctx.URL),
			Title:       sanitizeUTF8(ctx.Title),
			PublishedAt: ctx.PublishedAt,
		})
	}
	return citations
}

// convertCitationsToProtoCitations converts usecase.Citation slice to augurv2.Citation slice
// Used for Done event to return only the citations that LLM actually used in its response.
func (h *Handler) convertCitationsToProtoCitations(citations []usecase.Citation) []*augurv2.Citation {
	result := make([]*augurv2.Citation, 0, len(citations))
	for _, c := range citations {
		result = append(result, &augurv2.Citation{
			Url:   sanitizeUTF8(c.URL),
			Title: sanitizeUTF8(c.Title),
		})
	}
	return result
}

// RetrieveContext retrieves relevant context for a query without generating an answer
func (h *Handler) RetrieveContext(
	ctx context.Context,
	req *connect.Request[augurv2.RetrieveContextRequest],
) (*connect.Response[augurv2.RetrieveContextResponse], error) {
	query := req.Msg.Query
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	h.logger.Info("retrieving context",
		slog.String("query", query),
		slog.Int("limit", int(req.Msg.Limit)))

	input := usecase.RetrieveContextInput{
		Query: query,
	}

	output, err := h.retrieveUsecase.Execute(ctx, input)
	if err != nil {
		h.logger.Error("failed to retrieve context", slog.String("error", err.Error()))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Convert to proto ContextItems
	contexts := make([]*augurv2.ContextItem, 0, len(output.Contexts))
	for _, c := range output.Contexts {
		contexts = append(contexts, &augurv2.ContextItem{
			Url:         c.URL,
			Title:       c.Title,
			PublishedAt: c.PublishedAt,
			Score:       c.Score,
		})
	}

	// Apply limit if specified
	limit := int(req.Msg.Limit)
	if limit > 0 && limit < len(contexts) {
		contexts = contexts[:limit]
	}

	return connect.NewResponse(&augurv2.RetrieveContextResponse{
		Contexts: contexts,
	}), nil
}
