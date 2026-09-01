package augur_test

import (
	"context"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fallbackStreamHarness struct {
	events chan usecase.StreamEvent
	conv   *domain.AugurConversation
	mocks  *MockAugurConversationUsecase
	client augurv2connect.AugurServiceClient
	userID uuid.UUID
	stop   func()
}

func newFallbackStreamHarness(t *testing.T) *fallbackStreamHarness {
	t.Helper()

	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	mockConv := new(MockAugurConversationUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	userID := uuid.New()
	conv := &domain.AugurConversation{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     "fallback",
		CreatedAt: time.Now().UTC(),
	}

	events := make(chan usecase.StreamEvent, 8)
	mockConv.On("EnsureConversation", mock.Anything, userID, uuid.Nil, mock.AnythingOfType("string")).Return(conv, nil)
	mockConv.On("AppendUserTurn", mock.Anything, conv.ID, mock.AnythingOfType("string")).Return(nil)
	mockAnswer.On("Stream", mock.Anything, mock.Anything).Return((<-chan usecase.StreamEvent)(events))

	handler := augur.NewHandler(mockAnswer, mockRetrieve, mockConv, nil, logger)
	mux := http.NewServeMux()
	path, connectHandler := augurv2connect.NewAugurServiceHandler(handler)
	mux.Handle(path, connectHandler)
	server := httptest.NewServer(mux)

	return &fallbackStreamHarness{
		events: events,
		conv:   conv,
		mocks:  mockConv,
		client: augurv2connect.NewAugurServiceClient(server.Client(), server.URL),
		userID: userID,
		stop:   server.Close,
	}
}

func (h *fallbackStreamHarness) request() *connect.Request[augurv2.StreamChatRequest] {
	req := connect.NewRequest(&augurv2.StreamChatRequest{
		Messages: []*augurv2.ChatMessage{{Role: "user", Content: "なぜ物流が滞っているのか"}},
	})
	req.Header().Set("X-Alt-User-Id", h.userID.String())
	return req
}

// A fallback event used to end the read loop before the terminal Done arrived,
// so the answer the user actually saw was never written to augur_messages: the
// conversation kept the user turn and lost the assistant one. Every stored
// quality figure is computed over the rows that survive, so the worst answers
// were the ones systematically excluded.
func TestStreamChat_PersistsFallbackAnswerWithMarker(t *testing.T) {
	h := newFallbackStreamHarness(t)
	defer h.stop()

	var (
		mu       sync.Mutex
		content  string
		code     string
		reason   string
		called   = make(chan struct{})
		callOnce sync.Once
	)
	h.mocks.On("AppendFallbackAssistantTurn",
		mock.Anything, h.conv.ID, mock.AnythingOfType("string"),
		mock.Anything, mock.Anything,
		mock.AnythingOfType("string"), mock.AnythingOfType("string"),
	).Run(func(args mock.Arguments) {
		mu.Lock()
		content, code, reason = args.String(2), args.String(5), args.String(6)
		mu.Unlock()
		callOnce.Do(func() { close(called) })
	}).Return(nil)

	stream, err := h.client.StreamChat(context.Background(), h.request())
	require.NoError(t, err)

	h.events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDelta, Payload: "途中まで書いた回答"}
	h.events <- usecase.StreamEvent{Kind: usecase.StreamEventKindFallback, Payload: "answer_declined"}
	h.events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDone, Payload: &usecase.AnswerWithRAGOutput{
		Answer:           "途中まで書いた回答",
		Fallback:         true,
		Reason:           "answer quality insufficient: low keyword coverage",
		FallbackCategory: usecase.FallbackShortUnderGrounded,
	}}
	close(h.events)

	for stream.Receive() {
	}

	select {
	case <-called:
	case <-time.After(3 * time.Second):
		t.Fatal("a fallback answer must still be persisted as an assistant turn")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "途中まで書いた回答", content)
	assert.Equal(t, string(usecase.FallbackShortUnderGrounded), code)
	assert.Contains(t, reason, "low keyword coverage")
	h.mocks.AssertNotCalled(t, "AppendAssistantTurn",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The client contract is unchanged: a fallback is still terminal for the
// caller. Draining to the Done event is a server-side persistence concern and
// must not leak an extra "done" frame onto the wire after the fallback.
func TestStreamChat_FallbackStaysTerminalForTheClient(t *testing.T) {
	h := newFallbackStreamHarness(t)
	defer h.stop()

	persisted := make(chan struct{})
	var once sync.Once
	h.mocks.On("AppendFallbackAssistantTurn",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything,
	).Run(func(mock.Arguments) { once.Do(func() { close(persisted) }) }).Return(nil)

	stream, err := h.client.StreamChat(context.Background(), h.request())
	require.NoError(t, err)

	h.events <- usecase.StreamEvent{Kind: usecase.StreamEventKindFallback, Payload: "answer_declined"}
	h.events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDone, Payload: &usecase.AnswerWithRAGOutput{
		Answer:   "見つかった範囲での回答",
		Fallback: true,
		Reason:   "declined",
	}}
	close(h.events)

	var kinds []string
	for stream.Receive() {
		kinds = append(kinds, stream.Msg().Kind)
	}

	select {
	case <-persisted:
	case <-time.After(3 * time.Second):
		t.Fatal("fallback answer was not persisted")
	}

	require.NotEmpty(t, kinds)
	assert.Equal(t, "fallback", kinds[len(kinds)-1],
		"fallback must remain the last frame the client sees")
	assert.NotContains(t, kinds[1:], "done")
}

// A fallback with no text at all has nothing to show in history; writing an
// empty assistant bubble would be worse than the hole. It must still be
// counted and named in the log rather than disappearing.
func TestStreamChat_EmptyFallback_IsNotPersistedAsBlankTurn(t *testing.T) {
	h := newFallbackStreamHarness(t)
	defer h.stop()

	h.mocks.On("AppendFallbackAssistantTurn",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything,
	).Return(nil)

	stream, err := h.client.StreamChat(context.Background(), h.request())
	require.NoError(t, err)

	h.events <- usecase.StreamEvent{Kind: usecase.StreamEventKindFallback, Payload: "llm_no_output"}
	h.events <- usecase.StreamEvent{Kind: usecase.StreamEventKindDone, Payload: &usecase.AnswerWithRAGOutput{
		Answer:   "",
		Fallback: true,
		Reason:   "llm stream produced no data",
	}}
	close(h.events)

	for stream.Receive() {
	}
	time.Sleep(200 * time.Millisecond)

	h.mocks.AssertNotCalled(t, "AppendFallbackAssistantTurn",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything)
}
