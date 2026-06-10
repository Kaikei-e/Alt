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
		// Partial-flush path: only meta-derived preview citations are known
		// here. Related citations are computed at done-event time, so a
		// partial turn persists with an empty related set rather than a
		// stale guess.
		if err := h.conversationUsecase.AppendAssistantTurn(flushCtx, conv.ID, assistantBuffer.String(), lastCitations, nil); err != nil {
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
				related := citationsFromProto(donePayload.RelatedCitations)
				if err := h.conversationUsecase.AppendAssistantTurn(flushCtx, conv.ID, donePayload.Answer, citations, related); err != nil {
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
		related := h.convertCitationsToProtoCitations(output.RelatedCitations)
		done := &augurv2.DonePayload{
			Answer:           sanitizeUTF8(output.Answer),
			Citations:        citations,
			Intent:           output.Debug.IntentType,
			Strategy:         output.Debug.StrategyUsed,
			RelatedCitations: related,
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

// uuidLikeRe matches any 8-4-4-4-12 hex pattern (canonical UUID, any version).
// Used as a defensive filter so a citation Title never carries a bare UUID
// that would leak the internal identifier into the UI's visible text.
var uuidLikeRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// sanitizeCitationTitle strips a Title that is just a UUID (the historical
// label-fallback bug from ADR-926). Empty result lets the FE fall back to the
// URL's domain name or "Untitled source" — never the raw UUID.
func sanitizeCitationTitle(title string) string {
	trimmed := strings.TrimSpace(sanitizeUTF8(title))
	if trimmed == "" || uuidLikeRe.MatchString(trimmed) {
		return ""
	}
	return trimmed
}

// classifyCitation infers the Citation discriminator from the upstream
// usecase Citation. An ArticleID that parses as a UUID makes this an ARTICLE
// citation routed to /articles/<ref_id> on the FE; otherwise an https URL
// makes it a WEB citation; otherwise UNSPECIFIED, which the FE renders as a
// disabled span. The classifier deliberately ignores chunk-level UUIDs in
// ChunkID — only stable article IDs are eligible to become ref_id.
func classifyCitation(c usecase.Citation) (augurv2.CitationKind, string) {
	if c.ArticleID != "" {
		if _, err := uuid.Parse(c.ArticleID); err == nil {
			return augurv2.CitationKind_CITATION_KIND_ARTICLE, c.ArticleID
		}
	}
	if isHTTPURL(c.URL) {
		return augurv2.CitationKind_CITATION_KIND_WEB, ""
	}
	return augurv2.CitationKind_CITATION_KIND_UNSPECIFIED, ""
}

// isHTTPURL accepts only http(s) URLs so a stray UUID parked in c.URL cannot
// quietly become a WEB citation that the FE would then try to render.
func isHTTPURL(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://")
}

// convertContextsToCitations converts usecase.ContextItem slice to augurv2.Citation slice.
// This feeds the meta event, which is a preview surfaced before the LLM has
// committed to its citations. Related citations are deliberately NOT computed
// here; the done event is the single authoritative source for both lists.
func (h *Handler) convertContextsToCitations(contexts []usecase.ContextItem) []*augurv2.Citation {
	citations := make([]*augurv2.Citation, 0, len(contexts))
	for _, ctx := range contexts {
		kind := augurv2.CitationKind_CITATION_KIND_UNSPECIFIED
		refID := ""
		if ctx.ArticleID != "" {
			if _, err := uuid.Parse(ctx.ArticleID); err == nil {
				kind = augurv2.CitationKind_CITATION_KIND_ARTICLE
				refID = ctx.ArticleID
			}
		}
		if kind == augurv2.CitationKind_CITATION_KIND_UNSPECIFIED && isHTTPURL(ctx.URL) {
			kind = augurv2.CitationKind_CITATION_KIND_WEB
		}
		citations = append(citations, &augurv2.Citation{
			Url:         sanitizeUTF8(ctx.URL),
			Title:       sanitizeCitationTitle(ctx.Title),
			PublishedAt: sanitizeUTF8(ctx.PublishedAt),
			Kind:        kind,
			RefId:       refID,
		})
	}
	return citations
}

// convertCitationsToProtoCitations converts usecase.Citation slice (the LLM's
// final, grounded citations) into the wire form. Kind / RefId are inferred
// from the ArticleID propagated through the retrieval pipeline so the UI can
// route to /articles/<ref_id> instead of the legacy disabled-span fallback.
func (h *Handler) convertCitationsToProtoCitations(citations []usecase.Citation) []*augurv2.Citation {
	result := make([]*augurv2.Citation, 0, len(citations))
	for _, c := range citations {
		kind, refID := classifyCitation(c)
		result = append(result, &augurv2.Citation{
			Url:         sanitizeUTF8(c.URL),
			Title:       sanitizeCitationTitle(c.Title),
			PublishedAt: sanitizeUTF8(c.PublishedAt),
			Kind:        kind,
			RefId:       refID,
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
			Kind:        domainCitationKind(c.Kind),
			RefID:       c.RefId,
		})
	}
	return out
}

// domainCitationKind translates the wire-format enum into the domain-layer
// string so the domain package does not need to import the generated proto.
func domainCitationKind(k augurv2.CitationKind) domain.CitationKind {
	switch k {
	case augurv2.CitationKind_CITATION_KIND_WEB:
		return domain.CitationKindWeb
	case augurv2.CitationKind_CITATION_KIND_ARTICLE:
		return domain.CitationKindArticle
	case augurv2.CitationKind_CITATION_KIND_SUMMARY:
		return domain.CitationKindSummary
	default:
		return domain.CitationKindUnspecified
	}
}

// protoCitationKind is the inverse of domainCitationKind, used when reading
// stored citations back out for the client.
func protoCitationKind(k domain.CitationKind) augurv2.CitationKind {
	switch k {
	case domain.CitationKindWeb:
		return augurv2.CitationKind_CITATION_KIND_WEB
	case domain.CitationKindArticle:
		return augurv2.CitationKind_CITATION_KIND_ARTICLE
	case domain.CitationKindSummary:
		return augurv2.CitationKind_CITATION_KIND_SUMMARY
	default:
		return augurv2.CitationKind_CITATION_KIND_UNSPECIFIED
	}
}

// domainCitationsToProto rebuilds a wire-format slice from persisted citations
// so GetConversation read paths return the kind / ref_id discriminator the FE
// expects without having to re-classify by hand. Title is passed through the
// same UUID-only filter that the write path applies.
func domainCitationsToProto(cs []domain.AugurCitation) []*augurv2.Citation {
	out := make([]*augurv2.Citation, 0, len(cs))
	for _, c := range cs {
		out = append(out, &augurv2.Citation{
			Url:         sanitizeUTF8(c.URL),
			Title:       sanitizeCitationTitle(c.Title),
			PublishedAt: sanitizeUTF8(c.PublishedAt),
			Kind:        protoCitationKind(c.Kind),
			RefId:       sanitizeUTF8(c.RefID),
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
		protoCitations := domainCitationsToProto(m.Citations)
		protoRelated := domainCitationsToProto(m.RelatedCitations)
		resp.Messages = append(resp.Messages, &augurv2.ChatMessage{
			Role:             m.Role,
			Content:          sanitizeUTF8(m.Content),
			CreatedAt:        timestamppb.New(m.CreatedAt),
			Citations:        protoCitations,
			RelatedCitations: protoRelated,
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
