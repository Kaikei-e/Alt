package augur

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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

// tenantIDHeader carries the caller's tenant uuid. alt-backend extracts it
// from the JWT claim set and forwards it alongside X-Alt-User-Id. Wave 4-A
// (ADR-000853) made this header required on the augur emit path so
// knowledge_events.tenant_id is never persisted as the zero uuid — Surface
// Planner v2's resolver scopes its inputs by tenant_id physically.
const tenantIDHeader = "X-Alt-Tenant-Id"

// uuidv7Re matches RFC 9562 UUIDv7: version nibble == 7, variant bits 10.
var uuidv7Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

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
	eventEmitter        usecase.KnowledgeEventEmitter
	logger              *slog.Logger
}

// Ensure Handler implements the interface
var _ augurv2connect.AugurServiceHandler = (*Handler)(nil)

// NewHandler creates a new AugurService handler. eventEmitter publishes
// augur.conversation_linked.v1 into knowledge-sovereign so Knowledge
// Loop's Surface Planner v2 can pick up the linkage. Pass
// usecase.NoopKnowledgeEventEmitter{} when emit is intentionally disabled
// (tests, or production until alt-deploy services.yaml registers
// rag-orchestrator as a knowledge-sovereign pacticipant).
func NewHandler(
	answerUsecase usecase.AnswerWithRAGUsecase,
	retrieveUsecase usecase.RetrieveContextUsecase,
	conversationUsecase usecase.AugurConversationUsecase,
	eventEmitter usecase.KnowledgeEventEmitter,
	logger *slog.Logger,
) *Handler {
	if eventEmitter == nil {
		eventEmitter = usecase.NoopKnowledgeEventEmitter{}
	}
	return &Handler{
		answerUsecase:       answerUsecase,
		retrieveUsecase:     retrieveUsecase,
		conversationUsecase: conversationUsecase,
		eventEmitter:        eventEmitter,
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

// extractTenantID mirrors extractUserID for the tenant scope. It is used on
// emit paths (Wave 4-A): knowledge_events rows MUST carry a non-zero
// tenant_id, otherwise multi-tenant projector queries would surface
// cross-tenant evidence. Empty or malformed values become Unauthenticated
// rather than InvalidArgument because the missing tenant signals a broken
// trust boundary on the alt-backend hop, not a bad user payload.
func extractTenantID(headers interface{ Get(string) string }) (uuid.UUID, error) {
	raw := strings.TrimSpace(headers.Get(tenantIDHeader))
	if raw == "" {
		return uuid.Nil, errors.New("missing " + tenantIDHeader + " header")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid %s header: %w", tenantIDHeader, err)
	}
	if id == uuid.Nil {
		return uuid.Nil, errors.New(tenantIDHeader + " header is the zero uuid")
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
	// Detach persistence writes from the request ctx. When the Knowledge Home
	// AskSheet closes while a stream is in flight the client-side abort
	// propagates into the handler; we must not let that orphan the conversation
	// or the user turn. AppendAssistantTurn uses the same pattern on the
	// completion path.
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer persistCancel()

	conv, err := h.conversationUsecase.EnsureConversation(persistCtx, userID, requestedConvID, firstMsg)
	if err != nil {
		h.logger.Error("failed to ensure conversation", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeInternal, err)
	}

	if err := h.conversationUsecase.AppendUserTurn(persistCtx, conv.ID, query); err != nil {
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

	// Buffer the streaming assistant content so a mid-stream client abort still
	// produces an assistant turn. Knowledge Home's AskSheet auto-aborts on
	// close; without this buffer the conversation row survived with zero
	// messages, violating the append-only invariant that every conversation
	// carries at least the turns the user saw.
	var (
		assistantBuffer    strings.Builder
		lastCitations      []domain.AugurCitation
		authoritativeSaved bool
	)

	defer func() {
		if authoritativeSaved {
			return
		}
		if strings.TrimSpace(assistantBuffer.String()) == "" {
			return
		}
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		if err := h.conversationUsecase.AppendAssistantTurn(flushCtx, conv.ID, assistantBuffer.String(), lastCitations); err != nil {
			h.logger.Error("failed to flush partial assistant turn", slog.String("error", err.Error()))
		}
	}()

loop:
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("stream chat cancelled by client")
			return nil
		case event, ok := <-events:
			if !ok {
				break loop
			}

			if event.Kind == usecase.StreamEventKindDelta {
				if delta, ok := event.Payload.(string); ok {
					assistantBuffer.WriteString(sanitizeUTF8(delta))
				}
			}

			protoEvent, shouldContinue, donePayload := h.convertStreamEvent(event)
			if protoEvent != nil {
				// Echo the persisted id on every meta event the usecase emits.
				if meta := protoEvent.GetMeta(); meta != nil {
					meta.ConversationId = conv.ID.String()
					if len(meta.Citations) > 0 {
						lastCitations = citationsFromProto(meta.Citations)
					}
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
				flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
				citations := citationsFromProto(donePayload.Citations)
				if err := h.conversationUsecase.AppendAssistantTurn(flushCtx, conv.ID, donePayload.Answer, citations); err != nil {
					h.logger.Error("failed to persist assistant turn", slog.String("error", err.Error()))
				}
				flushCancel()
				authoritativeSaved = true
			}

			if !shouldContinue {
				break loop
			}
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

// CreateAugurSessionFromLoopEntry provisions a new conversation seeded with a
// Knowledge Loop entry's Why + evidence. Caller chain is alt-frontend-sv BFF
// → alt-backend → rag-orchestrator; the BFF resolves the entry through
// sovereign and passes the enriched payload through. Server is trusted and
// does NOT re-verify sovereign. See ADR-000836.
func (h *Handler) CreateAugurSessionFromLoopEntry(
	ctx context.Context,
	req *connect.Request[augurv2.CreateAugurSessionFromLoopEntryRequest],
) (*connect.Response[augurv2.CreateAugurSessionFromLoopEntryResponse], error) {
	userID, err := extractUserID(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	tenantID, err := extractTenantID(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	m := req.Msg
	if !uuidv7Re.MatchString(strings.ToLower(strings.TrimSpace(m.ClientHandshakeId))) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("client_handshake_id must be UUIDv7"))
	}
	if strings.TrimSpace(m.EntryKey) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("entry_key required"))
	}
	why := strings.TrimSpace(m.WhyText)
	if why == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("why_text required"))
	}
	if utf8.RuneCountInString(why) > 512 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("why_text exceeds 512 runes"))
	}
	if len(m.EvidenceRefs) > 8 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("evidence_refs exceeds 8 entries"))
	}

	refs := make([]domain.AugurCitation, 0, len(m.EvidenceRefs))
	for _, r := range m.EvidenceRefs {
		refs = append(refs, domain.AugurCitation{
			URL:   sanitizeUTF8(r.RefId),
			Title: sanitizeUTF8(r.Label),
		})
	}

	conv, err := h.conversationUsecase.CreateSessionFromLoopEntry(ctx, usecase.CreateSessionFromLoopEntryInput{
		UserID:       userID,
		EntryKey:     sanitizeUTF8(m.EntryKey),
		LensModeID:   sanitizeUTF8(m.LensModeId),
		WhyText:      sanitizeUTF8(why),
		EvidenceRefs: refs,
	})
	if err != nil {
		h.logger.Error("create augur loop session failed", slog.String("error", err.Error()))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Emit augur.conversation_linked.v1 into knowledge-sovereign so the
	// Knowledge Loop Surface Planner v2 resolver can credit this entry
	// with HasAugurLink and route it to the Continue plane on the next
	// projector tick. Best-effort warn-and-continue: emit failure must
	// NOT fail the conversation creation. The linked_at timestamp is the
	// persisted conversation's CreatedAt — payload-resident, event-time
	// pure (canonical contract §6.4.1, ADR-000853 / ADR-000854).
	emitInput := usecase.AugurConversationLinkedInput{
		UserID:         userID,
		TenantID:       tenantID,
		EntryKey:       sanitizeUTF8(m.EntryKey),
		LensModeID:     sanitizeUTF8(m.LensModeId),
		ConversationID: conv.ID,
		LinkedAt:       conv.CreatedAt.UnixMilli(),
	}
	if emitErr := h.eventEmitter.EmitAugurConversationLinked(ctx, emitInput); emitErr != nil {
		h.logger.Warn("augur.conversation_linked.v1 emit failed (non-fatal)",
			slog.String("error", emitErr.Error()),
			slog.String("entry_key", emitInput.EntryKey),
			slog.String("conversation_id", emitInput.ConversationID.String()),
		)
	}

	return connect.NewResponse(&augurv2.CreateAugurSessionFromLoopEntryResponse{
		ConversationId: conv.ID.String(),
	}), nil
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
