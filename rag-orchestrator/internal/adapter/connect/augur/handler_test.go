package augur_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"

	"rag-orchestrator/internal/adapter/connect/augur"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockAnswerWithRAGUsecase mocks the AnswerWithRAGUsecase interface
type MockAnswerWithRAGUsecase struct {
	mock.Mock
}

func (m *MockAnswerWithRAGUsecase) Execute(ctx context.Context, input usecase.AnswerWithRAGInput) (*usecase.AnswerWithRAGOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*usecase.AnswerWithRAGOutput), args.Error(1)
}

func (m *MockAnswerWithRAGUsecase) Stream(ctx context.Context, input usecase.AnswerWithRAGInput) <-chan usecase.StreamEvent {
	args := m.Called(ctx, input)
	return args.Get(0).(<-chan usecase.StreamEvent)
}

// MockRetrieveContextUsecase mocks the RetrieveContextUsecase interface
type MockRetrieveContextUsecase struct {
	mock.Mock
}

func (m *MockRetrieveContextUsecase) Execute(ctx context.Context, input usecase.RetrieveContextInput) (*usecase.RetrieveContextOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*usecase.RetrieveContextOutput), args.Error(1)
}

func TestHandler_RetrieveContext_Success(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, nil, nil, logger)

	ctx := context.Background()
	req := connect.NewRequest(&augurv2.RetrieveContextRequest{
		Query: "AIについて",
		Limit: 5,
	})

	mockRetrieve.On("Execute", ctx, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "AIについて"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				URL:         "https://example.com/article1",
				Title:       "AI記事1",
				PublishedAt: "2026-01-14T10:00:00Z",
				Score:       0.95,
			},
			{
				URL:         "https://example.com/article2",
				Title:       "AI記事2",
				PublishedAt: "2026-01-14T09:00:00Z",
				Score:       0.85,
			},
		},
	}, nil)

	resp, err := handler.RetrieveContext(ctx, req)

	require.NoError(t, err)
	assert.Len(t, resp.Msg.Contexts, 2)
	assert.Equal(t, "https://example.com/article1", resp.Msg.Contexts[0].Url)
	assert.Equal(t, "AI記事1", resp.Msg.Contexts[0].Title)
	assert.Equal(t, float32(0.95), resp.Msg.Contexts[0].Score)
}

func TestHandler_RetrieveContext_EmptyQuery(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, nil, nil, logger)

	ctx := context.Background()
	req := connect.NewRequest(&augurv2.RetrieveContextRequest{
		Query: "",
	})

	_, err := handler.RetrieveContext(ctx, req)

	require.Error(t, err)
	connectErr := err.(*connect.Error)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestHandler_RetrieveContext_WithLimit(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, nil, nil, logger)

	ctx := context.Background()
	req := connect.NewRequest(&augurv2.RetrieveContextRequest{
		Query: "AIについて",
		Limit: 1, // Limit to 1 result
	})

	mockRetrieve.On("Execute", ctx, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "AIについて"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				URL:         "https://example.com/article1",
				Title:       "AI記事1",
				PublishedAt: "2026-01-14T10:00:00Z",
				Score:       0.95,
			},
			{
				URL:         "https://example.com/article2",
				Title:       "AI記事2",
				PublishedAt: "2026-01-14T09:00:00Z",
				Score:       0.85,
			},
		},
	}, nil)

	resp, err := handler.RetrieveContext(ctx, req)

	require.NoError(t, err)
	// Should be limited to 1 result
	assert.Len(t, resp.Msg.Contexts, 1)
	assert.Equal(t, "https://example.com/article1", resp.Msg.Contexts[0].Url)
}

func TestNewHandler(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, nil, nil, logger)

	assert.NotNil(t, handler)
}

// MockAugurConversationUsecase mocks the AugurConversationUsecase interface.
type MockAugurConversationUsecase struct {
	mock.Mock
}

func (m *MockAugurConversationUsecase) EnsureConversation(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, firstUserMessage string) (*domain.AugurConversation, error) {
	args := m.Called(ctx, userID, conversationID, firstUserMessage)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AugurConversation), args.Error(1)
}

func (m *MockAugurConversationUsecase) AppendUserTurn(ctx context.Context, conversationID uuid.UUID, content string) error {
	return m.Called(ctx, conversationID, content).Error(0)
}

func (m *MockAugurConversationUsecase) AppendAssistantTurn(ctx context.Context, conversationID uuid.UUID, content string, citations []domain.AugurCitation, relatedCitations []domain.AugurCitation) error {
	return m.Called(ctx, conversationID, content, citations, relatedCitations).Error(0)
}

func (m *MockAugurConversationUsecase) ListConversations(ctx context.Context, userID uuid.UUID, limit int, afterActivity *time.Time, afterID *uuid.UUID) ([]domain.AugurConversationSummary, error) {
	args := m.Called(ctx, userID, limit, afterActivity, afterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.AugurConversationSummary), args.Error(1)
}

func (m *MockAugurConversationUsecase) GetConversation(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*domain.AugurConversation, []domain.AugurMessage, error) {
	args := m.Called(ctx, userID, conversationID)
	var conv *domain.AugurConversation
	if v := args.Get(0); v != nil {
		conv = v.(*domain.AugurConversation)
	}
	var msgs []domain.AugurMessage
	if v := args.Get(1); v != nil {
		msgs = v.([]domain.AugurMessage)
	}
	return conv, msgs, args.Error(2)
}

func (m *MockAugurConversationUsecase) DeleteConversation(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error {
	return m.Called(ctx, userID, conversationID).Error(0)
}

func (m *MockAugurConversationUsecase) CreateSessionFromLoopEntry(ctx context.Context, input usecase.CreateSessionFromLoopEntryInput) (*domain.AugurConversation, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AugurConversation), args.Error(1)
}

// TestStreamChat_ClientAbortAfterDeltas_FlushesPartialAssistantTurn asserts the
// write-path contract after the Knowledge Home → AskSheet regression: when the
// client aborts a streaming chat mid-flight after deltas have been emitted, the
// server must (a) still have persisted the user turn and (b) flush the
// accumulated partial assistant content via AppendAssistantTurn using a context
// that is decoupled from the now-canceled request context. Prior to the fix,
// the conversation row survived but both turns were lost because every write
// rode the request ctx.
func TestStreamChat_ClientAbortAfterDeltas_FlushesPartialAssistantTurn(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	mockConv := new(MockAugurConversationUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	userID := uuid.New()
	convID := uuid.New()
	conv := &domain.AugurConversation{
		ID:        convID,
		UserID:    userID,
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}

	events := make(chan usecase.StreamEvent, 8)
	var ensureDone <-chan struct{}
	var userTurnDone <-chan struct{}
	var assistantDone <-chan struct{}
	var assistantContent string
	assistantCalled := make(chan struct{})
	var once sync.Once

	mockConv.On("EnsureConversation", mock.Anything, userID, uuid.Nil, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			ensureDone = args.Get(0).(context.Context).Done()
		}).Return(conv, nil)
	mockConv.On("AppendUserTurn", mock.Anything, convID, "test query").
		Run(func(args mock.Arguments) {
			userTurnDone = args.Get(0).(context.Context).Done()
		}).Return(nil)
	mockConv.On("AppendAssistantTurn", mock.Anything, convID, mock.AnythingOfType("string"), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			assistantDone = args.Get(0).(context.Context).Done()
			assistantContent = args.String(2)
			once.Do(func() { close(assistantCalled) })
		}).Return(nil)
	mockAnswer.On("Stream", mock.Anything, mock.Anything).Return((<-chan usecase.StreamEvent)(events))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, mockConv, nil, logger)

	mux := http.NewServeMux()
	path, connectHandler := augurv2connect.NewAugurServiceHandler(handler)
	mux.Handle(path, connectHandler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := augurv2connect.NewAugurServiceClient(server.Client(), server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := connect.NewRequest(&augurv2.StreamChatRequest{
		Messages: []*augurv2.ChatMessage{{Role: "user", Content: "test query"}},
	})
	req.Header().Set("X-Alt-User-Id", userID.String())

	stream, err := client.StreamChat(ctx, req)
	require.NoError(t, err)

	// Server emits the leading meta event automatically. Push two delta chunks
	// so the buffered partial answer has something worth persisting.
	events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDelta, Payload: "Partial "}
	events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDelta, Payload: "answer"}

	// Drain meta + two deltas on the client side to make sure they reached us
	// before we abort — otherwise a cancel before the first flush is a no-op.
	require.True(t, stream.Receive(), "expected meta event")
	require.True(t, stream.Receive(), "expected first delta")
	require.True(t, stream.Receive(), "expected second delta")

	cancel()

	select {
	case <-assistantCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("AppendAssistantTurn was never invoked after client abort")
	}

	assert.Equal(t, "Partial answer", assistantContent, "partial deltas must be flushed as the assistant turn")
	// Done-channel identity proves the ctx handed to persistence is not the
	// request ctx. If it were, the fix never landed and a new class of aborts
	// will keep orphaning conversations.
	require.NotNil(t, ensureDone)
	require.NotNil(t, userTurnDone)
	require.NotNil(t, assistantDone)
	assert.NotEqual(t, ctx.Done(), ensureDone, "EnsureConversation ctx must not be tied to the request ctx")
	assert.NotEqual(t, ctx.Done(), userTurnDone, "AppendUserTurn ctx must not be tied to the request ctx")
	assert.NotEqual(t, ctx.Done(), assistantDone, "AppendAssistantTurn partial-flush ctx must be detached from the request ctx")

	close(events)
}

func TestHandler_RetrieveContext_SanitizesInvalidUTF8(t *testing.T) {
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, nil, nil, logger)

	// Build strings with invalid UTF-8: truncated 3-byte Japanese char (日 = E6 97 A5, only 2 bytes)
	invalidTitle := "記事タイトル" + string([]byte{0xe6, 0x97}) + "テスト"
	invalidURL := "https://example.com/" + string([]byte{0xc0, 0xaf}) + "path"
	invalidDate := "2026-01-01" + string([]byte{0xff})

	ctx := context.Background()
	req := connect.NewRequest(&augurv2.RetrieveContextRequest{
		Query: "テスト",
		Limit: 10,
	})

	mockRetrieve.On("Execute", ctx, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "テスト"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				URL:         invalidURL,
				Title:       invalidTitle,
				PublishedAt: invalidDate,
				Score:       0.9,
			},
		},
	}, nil)

	resp, err := handler.RetrieveContext(ctx, req)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Contexts, 1)

	c := resp.Msg.Contexts[0]
	// Invalid UTF-8 bytes must be stripped
	assert.Equal(t, "記事タイトルテスト", c.Title, "Title should have invalid UTF-8 removed")
	assert.Equal(t, "https://example.com/path", c.Url, "URL should have invalid UTF-8 removed")
	assert.Equal(t, "2026-01-01", c.PublishedAt, "PublishedAt should have invalid UTF-8 removed")
}

// fakeEventEmitter records EmitAugurConversationLinked calls so tests can
// assert wiring without a real knowledge-sovereign sovereign_client.
type fakeEventEmitter struct {
	mu    sync.Mutex
	calls []usecase.AugurConversationLinkedInput
	err   error
}

func (f *fakeEventEmitter) EmitAugurConversationLinked(_ context.Context, in usecase.AugurConversationLinkedInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	return f.err
}

func (f *fakeEventEmitter) snapshot() []usecase.AugurConversationLinkedInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]usecase.AugurConversationLinkedInput, len(f.calls))
	copy(out, f.calls)
	return out
}

// emitterFailureCount reads rag_orchestrator_knowledge_event_emitter_failure_total
// straight from the default Prometheus registry. sovereign_client keeps the
// collector unexported, so gathering the default registerer is the only way
// to assert IncEmitterFailure fired without reaching into package internals.
func emitterFailureCount(t *testing.T, eventType string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != "rag_orchestrator_knowledge_event_emitter_failure_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "event_type" && l.GetValue() == eventType {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// runStreamChatEmitScenario drives a single-turn StreamChat call through a
// real Connect-RPC httptest server (same harness shape as
// TestStreamChat_ClientAbortAfterDeltas_FlushesPartialAssistantTurn) and lets
// the caller inspect the fake emitter afterwards. citations become the Done
// event's citation list; tenantHeader is omitted entirely when empty so
// tests can exercise the "header missing" path.
func runStreamChatEmitScenario(
	t *testing.T,
	conv *domain.AugurConversation,
	requestedConvID uuid.UUID,
	tenantHeader string,
	citations []usecase.Citation,
	emitter *fakeEventEmitter,
) {
	t.Helper()

	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	mockConv := new(MockAugurConversationUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	events := make(chan usecase.StreamEvent, 1)
	mockConv.On("EnsureConversation", mock.Anything, conv.UserID, requestedConvID, mock.AnythingOfType("string")).
		Return(conv, nil)
	mockConv.On("AppendUserTurn", mock.Anything, conv.ID, "test query").Return(nil)
	mockConv.On("AppendAssistantTurn", mock.Anything, conv.ID, mock.AnythingOfType("string"), mock.Anything, mock.Anything).
		Return(nil)
	mockAnswer.On("Stream", mock.Anything, mock.Anything).Return((<-chan usecase.StreamEvent)(events))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, mockConv, emitter, logger)

	mux := http.NewServeMux()
	path, connectHandler := augurv2connect.NewAugurServiceHandler(handler)
	mux.Handle(path, connectHandler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := augurv2connect.NewAugurServiceClient(server.Client(), server.URL)

	ctx := context.Background()
	convIDField := ""
	if requestedConvID != uuid.Nil {
		convIDField = requestedConvID.String()
	}
	req := connect.NewRequest(&augurv2.StreamChatRequest{
		Messages:       []*augurv2.ChatMessage{{Role: "user", Content: "test query"}},
		ConversationId: convIDField,
	})
	req.Header().Set("X-Alt-User-Id", conv.UserID.String())
	if tenantHeader != "" {
		req.Header().Set("X-Alt-Tenant-Id", tenantHeader)
	}

	stream, err := client.StreamChat(ctx, req)
	require.NoError(t, err)

	events <- usecase.StreamEvent{
		Kind: usecase.StreamEventKindDone,
		Payload: &usecase.AnswerWithRAGOutput{
			Answer:    "the answer",
			Citations: citations,
		},
	}
	close(events)

	for stream.Receive() {
	}
	require.NoError(t, stream.Err())
}

// TestStreamChat_EmitsConversationLinked_OnNewConversationWithArticleCitation
// pins the canonical-contract §6.4.1 wiring: the first turn of a brand-new
// Augur conversation that cites a real article must publish
// augur.conversation_linked.v1 so Surface Planner v2 can compute
// augur_link_id. Before this fix h.eventEmitter was stored on Handler but
// never invoked anywhere (review HIGH finding, augur/handler.go:43).
func TestStreamChat_EmitsConversationLinked_OnNewConversationWithArticleCitation(t *testing.T) {
	emitter := &fakeEventEmitter{}
	articleID := uuid.New()
	tenantID := uuid.New()
	conv := &domain.AugurConversation{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Title:     "test",
		CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	}

	runStreamChatEmitScenario(t, conv, uuid.Nil, tenantID.String(), []usecase.Citation{
		{ArticleID: articleID.String(), Title: "an article"},
	}, emitter)

	calls := emitter.snapshot()
	require.Len(t, calls, 1, "expected exactly one augur.conversation_linked.v1 emit")
	got := calls[0]
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, conv.UserID, got.UserID)
	assert.Equal(t, conv.ID, got.ConversationID)
	assert.Equal(t, "article:"+articleID.String(), got.EntryKey)
	assert.Equal(t, "default", got.LensModeID)
	assert.Equal(t, conv.CreatedAt.UnixMilli(), got.LinkedAt)
}

func TestStreamChat_SkipsEmit_WhenNoArticleCitation(t *testing.T) {
	emitter := &fakeEventEmitter{}
	conv := &domain.AugurConversation{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}

	runStreamChatEmitScenario(t, conv, uuid.Nil, uuid.New().String(), []usecase.Citation{
		{URL: "https://example.com/x", Title: "a web source"},
	}, emitter)

	assert.Empty(t, emitter.snapshot(), "a WEB-only citation set must not trigger a conversation-linked emit")
}

func TestStreamChat_SkipsEmit_WhenTenantHeaderMissing(t *testing.T) {
	emitter := &fakeEventEmitter{}
	conv := &domain.AugurConversation{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}

	runStreamChatEmitScenario(t, conv, uuid.Nil, "", []usecase.Citation{
		{ArticleID: uuid.New().String(), Title: "an article"},
	}, emitter)

	assert.Empty(t, emitter.snapshot(), "missing X-Alt-Tenant-Id must skip emit rather than fabricate a tenant")
}

func TestStreamChat_SkipsEmit_WhenContinuingExistingConversation(t *testing.T) {
	emitter := &fakeEventEmitter{}
	conv := &domain.AugurConversation{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}

	runStreamChatEmitScenario(t, conv, uuid.New(), uuid.New().String(), []usecase.Citation{
		{ArticleID: uuid.New().String(), Title: "an article"},
	}, emitter)

	assert.Empty(t, emitter.snapshot(), "conversation-linked only fires for a newly minted conversation")
}

// TestStreamChat_IncrementsEmitterFailureMetric_OnEmitError pins the
// review's item #4 fix: the warn-and-continue path around
// EmitAugurConversationLinked must call sovereign_client.IncEmitterFailure so
// the "emit failure" Prometheus counter is not permanently zero.
func TestStreamChat_IncrementsEmitterFailureMetric_OnEmitError(t *testing.T) {
	before := emitterFailureCount(t, "augur.conversation_linked.v1")
	emitter := &fakeEventEmitter{err: errors.New("sovereign unreachable")}
	conv := &domain.AugurConversation{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}

	runStreamChatEmitScenario(t, conv, uuid.Nil, uuid.New().String(), []usecase.Citation{
		{ArticleID: uuid.New().String(), Title: "an article"},
	}, emitter)

	require.Len(t, emitter.snapshot(), 1, "emit must still be attempted even though it will fail")
	after := emitterFailureCount(t, "augur.conversation_linked.v1")
	assert.Equal(t, before+1, after, "IncEmitterFailure must fire on the warn-and-continue path")
}
