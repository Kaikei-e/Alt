package augur

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// userIDHeader carries the authenticated caller's UUID across the
// alt-backend → rag-orchestrator hop. alt-backend validates the JWT and sets
// this header; rag-orchestrator never accepts unauthenticated traffic.
const userIDHeader = "X-Alt-User-Id"

// sanitizeUTF8 removes invalid UTF-8 sequences from a string.
// This is necessary because Ollama LLM may return chunks containing
// invalid UTF-8, which causes protobuf serialization to fail with
// "string field contains invalid UTF-8" errors.
func sanitizeUTF8(s string) string {
	return strings.ToValidUTF8(s, "")
}

// Handler implements augurv2connect.AugurServiceHandler
type Handler struct {
	answerUsecase       usecase.AnswerWithRAGUsecase
	retrieveUsecase     usecase.RetrieveContextUsecase
	conversationUsecase usecase.AugurConversationUsecase
	logger              *slog.Logger
}

// Ensure Handler implements the interface
var _ augurv2connect.AugurServiceHandler = (*Handler)(nil)

// NewHandler creates a new AugurService handler
func NewHandler(
	answerUsecase usecase.AnswerWithRAGUsecase,
	retrieveUsecase usecase.RetrieveContextUsecase,
	conversationUsecase usecase.AugurConversationUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		answerUsecase:       answerUsecase,
		retrieveUsecase:     retrieveUsecase,
		conversationUsecase: conversationUsecase,
		logger:              logger,
	}
}

// extractUserID reads the authenticated caller from the X-Alt-User-Id header.
// Empty or malformed values become a connect.CodeUnauthenticated error: chat
// persistence requires a stable user id.
func extractUserID(headers interface{ Get(string) string }) (uuid.UUID, error) {
	raw := strings.TrimSpace(headers.Get(userIDHeader))
	if raw == "" {
		return uuid.Nil, errors.New("missing " + userIDHeader + " header")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid %s header: %w", userIDHeader, err)
	}
	return id, nil
}

// firstUserMessage returns the first user-role message in the request.
// Used to derive a conversation title when minting a new conversation.
func firstUserMessage(msgs []*augurv2.ChatMessage) string {
	for _, m := range msgs {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			return m.Content
		}
	}
	return ""
}

// StreamChat implements streaming RAG chat
func (h *Handler) StreamChat(
	ctx context.Context,
	req *connect.Request[augurv2.StreamChatRequest],
	stream *connect.ServerStream[augurv2.StreamChatResponse],
) error {
	userID, err := extractUserID(req.Header())
	if err != nil {
		h.logger.Warn("augur stream chat rejected", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Extract last user message as query and build conversation history
	var query string
	var conversationHistory []domain.Message
	for i := len(req.Msg.Messages) - 1; i >= 0; i-- {
		if req.Msg.Messages[i].Role == "user" && query == "" {
			query = req.Msg.Messages[i].Content
		}
	}

	if query == "" {
		h.logger.Warn("no user message found in request")
		return connect.NewError(connect.CodeInvalidArgument, nil)
	}

	// Build conversation history (all messages except the last user message)
	// Limit to last 6 messages (3 turns) for efficiency
	allMsgs := req.Msg.Messages
	if len(allMsgs) > 1 {
		historyMsgs := allMsgs[:len(allMsgs)-1] // Exclude last message (the query)
		start := 0
		if len(historyMsgs) > 6 {
			start = len(historyMsgs) - 6
		}
		for _, msg := range historyMsgs[start:] {
			conversationHistory = append(conversationHistory, domain.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Resolve persisted conversation row. Zero UUID = new conversation.
	var requestedConvID uuid.UUID
	if raw := strings.TrimSpace(req.Msg.ConversationId); raw != "" {
		parsed, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return connect.NewError(connect.CodeInvalidArgument, parseErr)
		}
		requestedConvID = parsed
	}

	firstMsg := firstUserMessage(req.Msg.Messages)
	if firstMsg == "" {
		firstMsg = query
	}
	conv, err := h.conversationUsecase.EnsureConversation(ctx, userID, requestedConvID, firstMsg)
	if err != nil {
		h.logger.Error("failed to ensure conversation", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeInternal, err)
	}

	// Persist the user's current turn. If this fails we refuse to stream —
	// we never want the LLM answer to outlive a lost user turn.
	if err := h.conversationUsecase.AppendUserTurn(ctx, conv.ID, query); err != nil {
		h.logger.Error("failed to persist user turn", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeInternal, err)
	}

	h.logger.Info("starting augur stream chat",
		slog.String("query", query),
		slog.Int("history_turns", len(conversationHistory)),
		slog.String("conversation_id", conv.ID.String()))

	// Derive thread ID for the in-memory ConversationStore (separate from
	// persisted conversation id — it feeds RAG context continuity, not history).
	threadID := deriveThreadID(req.Msg.Messages)

	// Build input for AnswerWithRAGUsecase
	locale := detectLocale(query)
	input := usecase.AnswerWithRAGInput{
		Query:               query,
		UserID:              threadID,
		Locale:              locale,
		ConversationHistory: conversationHistory,
	}

	// Stream answer using AnswerWithRAGUsecase
	events := h.answerUsecase.Stream(ctx, input)

	// Emit a leading meta event so the client can learn the persisted
	// conversation id before any content deltas arrive.
	if err := stream.Send(&augurv2.StreamChatResponse{
		Kind: "meta",
		Payload: &augurv2.StreamChatResponse_Meta{
			Meta: &augurv2.MetaPayload{
				ConversationId: conv.ID.String(),
			},
		},
	}); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	// Process stream events and convert to Connect-RPC events
	for event := range events {
		select {
		case <-ctx.Done():
			h.logger.Info("stream chat cancelled by client")
			return nil
		default:
		}

		protoEvent, shouldContinue, donePayload := h.convertStreamEvent(event)
		if protoEvent != nil {
			// Echo the persisted id on every meta event the usecase emits.
			if meta := protoEvent.GetMeta(); meta != nil {
				meta.ConversationId = conv.ID.String()
			}
			if err := stream.Send(protoEvent); err != nil {
				h.logger.Error("failed to send event", slog.String("error", err.Error()))
				return connect.NewError(connect.CodeInternal, err)
			}
		}

		// Persist the assistant turn whenever the terminal Done event carries
		// non-empty content. This works for clean success, for partial-success
		// fallback (deltas streamed before the strategy gave up), and is
		// correctly skipped for hard failures (Answer == "") and clarification
		// (no assistant answer to keep).
		if donePayload != nil && strings.TrimSpace(donePayload.Answer) != "" {
			persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			citations := citationsFromProto(donePayload.Citations)
			if err := h.conversationUsecase.AppendAssistantTurn(persistCtx, conv.ID, donePayload.Answer, citations); err != nil {
				h.logger.Error("failed to persist assistant turn", slog.String("error", err.Error()))
			}
			cancel()
		}

		if !shouldContinue {
			break
		}
	}

	h.logger.Info("augur stream chat completed")
	return nil
}

// convertStreamEvent converts usecase.StreamEvent to augurv2.StreamChatResponse.
// The third return value is the DonePayload when the event is a terminal done
// event, so the caller can persist the assistant turn.
func (h *Handler) convertStreamEvent(event usecase.StreamEvent) (*augurv2.StreamChatResponse, bool, *augurv2.DonePayload) {
	switch event.Kind {
	case usecase.StreamEventKindDelta:
		delta, ok := event.Payload.(string)
		if !ok {
			return nil, true, nil
		}
		return &augurv2.StreamChatResponse{
			Kind: "delta",
			Payload: &augurv2.StreamChatResponse_Delta{
				Delta: sanitizeUTF8(delta),
			},
		}, true, nil

	case usecase.StreamEventKindMeta:
		meta, ok := event.Payload.(usecase.StreamMeta)
		if !ok {
			return nil, true, nil
		}
		citations := h.convertContextsToCitations(meta.Contexts)
		return &augurv2.StreamChatResponse{
			Kind: "meta",
			Payload: &augurv2.StreamChatResponse_Meta{
				Meta: &augurv2.MetaPayload{
					Citations: citations,
				},
			},
		}, true, nil

	case usecase.StreamEventKindDone:
		output, ok := event.Payload.(*usecase.AnswerWithRAGOutput)
		if !ok {
			return nil, false, nil
		}
		citations := h.convertCitationsToProtoCitations(output.Citations)
		done := &augurv2.DonePayload{
			Answer:    sanitizeUTF8(output.Answer),
			Citations: citations,
			Intent:    output.Debug.IntentType,
			Strategy:  output.Debug.StrategyUsed,
		}
		return &augurv2.StreamChatResponse{
			Kind: "done",
			Payload: &augurv2.StreamChatResponse_Done{
				Done: done,
			},
		}, false, done

	case usecase.StreamEventKindFallback:
		reason, _ := event.Payload.(string)
		return &augurv2.StreamChatResponse{
			Kind: "fallback",
			Payload: &augurv2.StreamChatResponse_FallbackCode{
				FallbackCode: sanitizeUTF8(reason),
			},
		}, false, nil

	case usecase.StreamEventKindError:
		errMsg, _ := event.Payload.(string)
		return &augurv2.StreamChatResponse{
			Kind: "error",
			Payload: &augurv2.StreamChatResponse_ErrorMessage{
				ErrorMessage: sanitizeUTF8(errMsg),
			},
		}, false, nil

	case usecase.StreamEventKindThinking:
		thinking, ok := event.Payload.(string)
		if !ok {
			return nil, true, nil
		}
		return &augurv2.StreamChatResponse{
			Kind: "thinking",
			Payload: &augurv2.StreamChatResponse_ThinkingDelta{
				ThinkingDelta: sanitizeUTF8(thinking),
			},
		}, true, nil

	case usecase.StreamEventKindHeartbeat:
		return &augurv2.StreamChatResponse{
			Kind: "heartbeat",
			Payload: &augurv2.StreamChatResponse_Delta{
				Delta: "",
			},
		}, true, nil

	case usecase.StreamEventKindClarification:
		clarification, ok := event.Payload.(usecase.StreamClarification)
		if !ok {
			return nil, true, nil
		}
		return &augurv2.StreamChatResponse{
			Kind: "clarification",
			Payload: &augurv2.StreamChatResponse_Delta{
				Delta: sanitizeUTF8(clarification.Message),
			},
		}, false, nil

	case usecase.StreamEventKindProgress:
		progress, ok := event.Payload.(string)
		if !ok {
			return nil, true, nil
		}
		return &augurv2.StreamChatResponse{
			Kind: "progress",
			Payload: &augurv2.StreamChatResponse_Delta{
				Delta: sanitizeUTF8(progress),
			},
		}, true, nil

	default:
		h.logger.Warn("unknown stream event kind", slog.String("kind", string(event.Kind)))
		return nil, true, nil
	}
}

// convertContextsToCitations converts usecase.ContextItem slice to augurv2.Citation slice
func (h *Handler) convertContextsToCitations(contexts []usecase.ContextItem) []*augurv2.Citation {
	citations := make([]*augurv2.Citation, 0, len(contexts))
	for _, ctx := range contexts {
		citations = append(citations, &augurv2.Citation{
			Url:         sanitizeUTF8(ctx.URL),
			Title:       sanitizeUTF8(ctx.Title),
			PublishedAt: sanitizeUTF8(ctx.PublishedAt),
		})
	}
	return citations
}

// convertCitationsToProtoCitations converts usecase.Citation slice to augurv2.Citation slice
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

// citationsFromProto converts proto citations into domain form for persistence.
func citationsFromProto(cs []*augurv2.Citation) []domain.AugurCitation {
	if len(cs) == 0 {
		return nil
	}
	out := make([]domain.AugurCitation, 0, len(cs))
	for _, c := range cs {
		out = append(out, domain.AugurCitation{
			URL:         c.Url,
			Title:       c.Title,
			PublishedAt: c.PublishedAt,
		})
	}
	return out
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

	contexts := make([]*augurv2.ContextItem, 0, len(output.Contexts))
	for _, c := range output.Contexts {
		contexts = append(contexts, &augurv2.ContextItem{
			Url:         sanitizeUTF8(c.URL),
			Title:       sanitizeUTF8(c.Title),
			PublishedAt: sanitizeUTF8(c.PublishedAt),
			Score:       c.Score,
		})
	}

	limit := int(req.Msg.Limit)
	if limit > 0 && limit < len(contexts) {
		contexts = contexts[:limit]
	}

	return connect.NewResponse(&augurv2.RetrieveContextResponse{
		Contexts: contexts,
	}), nil
}

// ListConversations returns the caller's chat history index (most recent first).
func (h *Handler) ListConversations(
	ctx context.Context,
	req *connect.Request[augurv2.ListConversationsRequest],
) (*connect.Response[augurv2.ListConversationsResponse], error) {
	userID, err := extractUserID(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	limit := int(req.Msg.PageSize)
	var afterActivity *time.Time
	var afterID *uuid.UUID
	if token := strings.TrimSpace(req.Msg.PageToken); token != "" {
		ts, id, ok := decodePageToken(token)
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
		}
		afterActivity = &ts
		afterID = &id
	}

	summaries, err := h.conversationUsecase.ListConversations(ctx, userID, limit, afterActivity, afterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &augurv2.ListConversationsResponse{
		Conversations: make([]*augurv2.ConversationSummary, 0, len(summaries)),
	}
	for _, s := range summaries {
		resp.Conversations = append(resp.Conversations, &augurv2.ConversationSummary{
			Id:                 s.ID.String(),
			Title:              sanitizeUTF8(s.Title),
			CreatedAt:          timestamppb.New(s.CreatedAt),
			LastActivityAt:     timestamppb.New(s.LastActivityAt),
			LastMessagePreview: sanitizeUTF8(s.LastMessagePreview),
			MessageCount:       int32(s.MessageCount),
		})
	}
	if len(summaries) > 0 && len(summaries) == limit {
		last := summaries[len(summaries)-1]
		resp.NextPageToken = encodePageToken(last.LastActivityAt, last.ID)
	}
	return connect.NewResponse(resp), nil
}

// GetConversation returns every message of a single conversation.
func (h *Handler) GetConversation(
	ctx context.Context,
	req *connect.Request[augurv2.GetConversationRequest],
) (*connect.Response[augurv2.GetConversationResponse], error) {
	userID, err := extractUserID(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	convID, err := uuid.Parse(strings.TrimSpace(req.Msg.Id))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	conv, msgs, err := h.conversationUsecase.GetConversation(ctx, userID, convID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if conv == nil {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}

	resp := &augurv2.GetConversationResponse{
		Id:        conv.ID.String(),
		Title:     sanitizeUTF8(conv.Title),
		CreatedAt: timestamppb.New(conv.CreatedAt),
		Messages:  make([]*augurv2.ChatMessage, 0, len(msgs)),
	}
	for _, m := range msgs {
		protoCitations := make([]*augurv2.Citation, 0, len(m.Citations))
		for _, c := range m.Citations {
			protoCitations = append(protoCitations, &augurv2.Citation{
				Url:         c.URL,
				Title:       c.Title,
				PublishedAt: c.PublishedAt,
			})
		}
		resp.Messages = append(resp.Messages, &augurv2.ChatMessage{
			Role:      m.Role,
			Content:   sanitizeUTF8(m.Content),
			CreatedAt: timestamppb.New(m.CreatedAt),
			Citations: protoCitations,
		})
	}
	return connect.NewResponse(resp), nil
}

// DeleteConversation removes a conversation and its messages.
func (h *Handler) DeleteConversation(
	ctx context.Context,
	req *connect.Request[augurv2.DeleteConversationRequest],
) (*connect.Response[augurv2.DeleteConversationResponse], error) {
	userID, err := extractUserID(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	convID, err := uuid.Parse(strings.TrimSpace(req.Msg.Id))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.conversationUsecase.DeleteConversation(ctx, userID, convID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&augurv2.DeleteConversationResponse{}), nil
}

// encodePageToken builds an opaque keyset-pagination token for (last_activity_at, id).
func encodePageToken(last time.Time, id uuid.UUID) string {
	return fmt.Sprintf("%d|%s", last.UnixNano(), id.String())
}

func decodePageToken(token string) (time.Time, uuid.UUID, bool) {
	parts := strings.SplitN(token, "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, false
	}
	ns, err := parseInt64(parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	return time.Unix(0, ns).UTC(), id, true
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// detectLocale determines the response language based on query content.
// Uses Unicode range heuristics: if Japanese characters (Hiragana, Katakana, CJK)
// make up a significant portion, the locale is "ja"; otherwise "en".
// deriveThreadID generates a deterministic thread ID from the first user message
// in the conversation. Same conversation always maps to the same thread ID.
func deriveThreadID(messages []*augurv2.ChatMessage) string {
	for _, msg := range messages {
		if msg.Role == "user" && msg.Content != "" {
			hash := sha256.Sum256([]byte(msg.Content))
			return fmt.Sprintf("thread-%x", hash[:8])
		}
	}
	hash := sha256.Sum256(nil)
	return fmt.Sprintf("thread-%x", hash[:8])
}

func detectLocale(query string) string {
	if query == "" {
		return "ja"
	}
	jaCount := 0
	total := 0
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			total++
			if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
				jaCount++
			}
		}
	}
	if total == 0 {
		return "ja"
	}
	if float64(jaCount)/float64(total) > 0.3 {
		return "ja"
	}
	return "en"
}
