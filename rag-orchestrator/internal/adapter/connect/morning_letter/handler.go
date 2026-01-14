package morning_letter

import (
	"context"
	"log/slog"
	"time"

	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"connectrpc.com/connect"
)

// Handler implements morningletterv2connect.MorningLetterServiceHandler
type Handler struct {
	articleClient domain.ArticleClient
	answerUsecase usecase.AnswerWithRAGUsecase
	logger        *slog.Logger
}

// Ensure Handler implements the interface
var _ morningletterv2connect.MorningLetterServiceHandler = (*Handler)(nil)

// NewHandler creates a new MorningLetterService handler
func NewHandler(
	articleClient domain.ArticleClient,
	answerUsecase usecase.AnswerWithRAGUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		articleClient: articleClient,
		answerUsecase: answerUsecase,
		logger:        logger,
	}
}

// StreamChat implements streaming chat with time-bounded RAG context
func (h *Handler) StreamChat(
	ctx context.Context,
	req *connect.Request[morningletterv2.StreamChatRequest],
	stream *connect.ServerStream[morningletterv2.StreamChatEvent],
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

	// Get time window (default 24 hours, max 168 hours = 7 days)
	withinHours := int(req.Msg.WithinHours)
	if withinHours <= 0 {
		withinHours = 24
	}
	if withinHours > 168 {
		withinHours = 168
	}

	now := time.Now()
	since := now.Add(-time.Duration(withinHours) * time.Hour)

	h.logger.Info("starting morning letter stream chat",
		slog.String("query", query),
		slog.Int("within_hours", withinHours))

	// 1. Fetch recent articles from alt-backend (limit=0 means no limit, relying on time constraint only)
	articles, err := h.articleClient.GetRecentArticles(ctx, withinHours, 0)
	if err != nil {
		h.logger.Error("failed to fetch recent articles", slog.String("error", err.Error()))
		return h.sendErrorEvent(stream, "Failed to fetch recent articles")
	}

	h.logger.Info("fetched recent articles", slog.Int("count", len(articles)))

	if len(articles) == 0 {
		// Send fallback for no articles
		return h.sendFallbackEvent(stream, "no_articles_in_timeframe")
	}

	// 2. Extract article IDs for candidate filtering
	articleIDs := make([]string, len(articles))
	for i, a := range articles {
		articleIDs[i] = a.ID.String()
	}

	// 3. Send meta event with time window info
	metaEvent := &morningletterv2.StreamChatEvent{
		Kind: "meta",
		Payload: &morningletterv2.StreamChatEvent_Meta{
			Meta: &morningletterv2.MetaPayload{
				Citations: nil, // Will be populated after retrieval
				TimeWindow: &morningletterv2.TimeWindow{
					Since: since.Format(time.RFC3339),
					Until: now.Format(time.RFC3339),
				},
				ArticlesScanned: int32(min(len(articles), 1<<31-1)), //nolint:gosec // safe: articles count never exceeds int32 max
			},
		},
	}
	if err := stream.Send(metaEvent); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	// 4. Stream answer using AnswerWithRAGUsecase
	input := usecase.AnswerWithRAGInput{
		Query:               query,
		CandidateArticleIDs: articleIDs,
		Locale:              "ja", // Default to Japanese for morning letter
	}

	events := h.answerUsecase.Stream(ctx, input)

	// 5. Process stream events and convert to Connect-RPC events
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

	h.logger.Info("morning letter stream chat completed")
	return nil
}

// convertStreamEvent converts usecase.StreamEvent to morningletterv2.StreamChatEvent
func (h *Handler) convertStreamEvent(event usecase.StreamEvent) (*morningletterv2.StreamChatEvent, bool) {
	switch event.Kind {
	case usecase.StreamEventKindDelta:
		delta, ok := event.Payload.(string)
		if !ok {
			return nil, true
		}
		return &morningletterv2.StreamChatEvent{
			Kind: "delta",
			Payload: &morningletterv2.StreamChatEvent_Delta{
				Delta: delta,
			},
		}, true

	case usecase.StreamEventKindMeta:
		meta, ok := event.Payload.(usecase.StreamMeta)
		if !ok {
			return nil, true
		}
		citations := h.convertContextsToCitations(meta.Contexts)
		return &morningletterv2.StreamChatEvent{
			Kind: "meta",
			Payload: &morningletterv2.StreamChatEvent_Meta{
				Meta: &morningletterv2.MetaPayload{
					Citations: citations,
				},
			},
		}, true

	case usecase.StreamEventKindDone:
		output, ok := event.Payload.(*usecase.AnswerWithRAGOutput)
		if !ok {
			return nil, false
		}
		// Use output.Citations (only citations LLM actually used) instead of output.Contexts (all search results)
		citations := h.convertCitationsToProtoCitations(output.Citations)
		return &morningletterv2.StreamChatEvent{
			Kind: "done",
			Payload: &morningletterv2.StreamChatEvent_Done{
				Done: &morningletterv2.DonePayload{
					Answer:    output.Answer,
					Citations: citations,
				},
			},
		}, false

	case usecase.StreamEventKindFallback:
		reason, _ := event.Payload.(string)
		return &morningletterv2.StreamChatEvent{
			Kind: "fallback",
			Payload: &morningletterv2.StreamChatEvent_FallbackCode{
				FallbackCode: reason,
			},
		}, false

	case usecase.StreamEventKindError:
		errMsg, _ := event.Payload.(string)
		return &morningletterv2.StreamChatEvent{
			Kind: "error",
			Payload: &morningletterv2.StreamChatEvent_ErrorMessage{
				ErrorMessage: errMsg,
			},
		}, false

	default:
		h.logger.Warn("unknown stream event kind", slog.String("kind", string(event.Kind)))
		return nil, true
	}
}

// convertContextsToCitations converts usecase.ContextItem slice to morningletterv2.Citation slice
func (h *Handler) convertContextsToCitations(contexts []usecase.ContextItem) []*morningletterv2.Citation {
	citations := make([]*morningletterv2.Citation, 0, len(contexts))
	for _, ctx := range contexts {
		citations = append(citations, &morningletterv2.Citation{
			Url:         ctx.URL,
			Title:       ctx.Title,
			PublishedAt: ctx.PublishedAt,
		})
	}
	return citations
}

// convertCitationsToProtoCitations converts usecase.Citation slice to morningletterv2.Citation slice
// Used for Done event to return only the citations that LLM actually used in its response.
func (h *Handler) convertCitationsToProtoCitations(citations []usecase.Citation) []*morningletterv2.Citation {
	result := make([]*morningletterv2.Citation, 0, len(citations))
	for _, c := range citations {
		result = append(result, &morningletterv2.Citation{
			Url:   c.URL,
			Title: c.Title,
		})
	}
	return result
}

// sendErrorEvent sends an error event to the stream
func (h *Handler) sendErrorEvent(stream *connect.ServerStream[morningletterv2.StreamChatEvent], message string) error {
	event := &morningletterv2.StreamChatEvent{
		Kind: "error",
		Payload: &morningletterv2.StreamChatEvent_ErrorMessage{
			ErrorMessage: message,
		},
	}
	if err := stream.Send(event); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	return nil
}

// sendFallbackEvent sends a fallback event to the stream
func (h *Handler) sendFallbackEvent(stream *connect.ServerStream[morningletterv2.StreamChatEvent], code string) error {
	event := &morningletterv2.StreamChatEvent{
		Kind: "fallback",
		Payload: &morningletterv2.StreamChatEvent_FallbackCode{
			FallbackCode: code,
		},
	}
	if err := stream.Send(event); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	return nil
}
