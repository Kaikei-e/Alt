package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockRetrieveContextUsecase struct {
	mock.Mock
}

func (m *mockRetrieveContextUsecase) Execute(ctx context.Context, input usecase.RetrieveContextInput) (*usecase.RetrieveContextOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*usecase.RetrieveContextOutput), args.Error(1)
}

type mockLLMClient struct {
	mock.Mock
}

func (m *mockLLMClient) Generate(ctx context.Context, prompt string, maxTokens int) (*domain.LLMResponse, error) {
	args := m.Called(ctx, prompt, maxTokens)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.LLMResponse), args.Error(1)
}

func (m *mockLLMClient) GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	args := m.Called(ctx, prompt, maxTokens)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(<-chan domain.LLMStreamChunk), args.Get(1).(<-chan error), args.Error(2)
}

func (m *mockLLMClient) Version() string {
	return "mock"
}

func (m *mockLLMClient) Chat(ctx context.Context, messages []domain.Message, maxTokens int) (*domain.LLMResponse, error) {
	args := m.Called(ctx, messages, maxTokens)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.LLMResponse), args.Error(1)
}

func (m *mockLLMClient) ChatStream(ctx context.Context, messages []domain.Message, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	args := m.Called(ctx, messages, maxTokens)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(<-chan domain.LLMStreamChunk), args.Get(1).(<-chan error), args.Error(2)
}

func TestAnswerWithRAG_Success(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 10, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk",
				URL:             "http://example.com",
				Title:           "Example",
				PublishedAt:     "2025-12-25T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}, nil)

	// Single Phase Response
	llmResponse := `{
  "answer": "Hello world [chunk_1]",
  "citations": [{"chunk_id":"` + chunkID.String() + `","reason":"relevant"}],
  "fallback": false,
  "reason": ""
}`

	// Expect Single Call (Gemma 3: instructions merged into user message)
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		return len(msgs) == 1 && msgs[0].Role == "user" &&
			contains(msgs[0].Content, "リサーチアナリスト") &&
			contains(msgs[0].Content, "500文字以上")
	}), mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "query"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Equal(t, "Hello world [chunk_1]", output.Answer)
	assert.Len(t, output.Citations, 1)
	assert.Equal(t, chunkID.String(), output.Citations[0].ChunkID)
}

func TestAnswerWithRAG_Fallback(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 7, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk",
				Score:           0.6,
				DocumentVersion: 1,
			},
		},
	}, nil)

	// Fallback Response
	fallbackResponse := `{
  "answer": "",
  "citations": [],
  "fallback": true,
  "reason": "insufficient evidence"
}`
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		return len(msgs) == 1 && msgs[0].Role == "user" &&
			contains(msgs[0].Content, "リサーチアナリスト")
	}), mock.Anything).Return(&domain.LLMResponse{Text: fallbackResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "query"})
	assert.NoError(t, err)
	assert.True(t, output.Fallback)
	assert.Equal(t, "insufficient evidence", output.Reason)
	assert.Equal(t, usecase.FallbackLLMFallback, output.FallbackCategory)
	assert.Len(t, output.Citations, 0)
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestStream_SendsThinkingEventFirst verifies that the Stream method sends an
// immediate thinking event before the retrieval phase to prevent Cloudflare 524
// timeout errors (60-second idle timeout on streaming connections).
func TestStream_SendsThinkingEventFirst(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 10, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk",
				URL:             "http://example.com",
				Title:           "Example",
				PublishedAt:     "2025-12-25T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}, nil)

	// Prepare streaming response
	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)

	llmResponse := `{"answer": "Hello world [chunk_1]", "citations": [{"chunk_id":"` + chunkID.String() + `","reason":"relevant"}], "fallback": false, "reason": ""}`
	chunkChan <- domain.LLMStreamChunk{Response: llmResponse, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil)

	// Execute Stream
	eventChan := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test query"})

	// Collect all events
	var events []usecase.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Assert: first event must be thinking (Cloudflare 524 prevention)
	assert.GreaterOrEqual(t, len(events), 2, "should have at least thinking and meta events")
	assert.Equal(t, usecase.StreamEventKindThinking, events[0].Kind, "first event should be thinking for Cloudflare 524 prevention")
	assert.Equal(t, "", events[0].Payload, "initial thinking event should be empty (signals processing started)")

	// Assert: meta event should come after thinking
	var metaIdx int
	for i, e := range events {
		if e.Kind == usecase.StreamEventKindMeta {
			metaIdx = i
			break
		}
	}
	assert.Greater(t, metaIdx, 0, "meta event should come after thinking event")
}

// TestStream_HeartbeatDuringSlowBuildPrompt verifies that heartbeat events are
// emitted at regular intervals when buildPrompt (retrieval + reranking) takes a
// long time. This prevents Cloudflare's 30-second Proxy Write Timeout from
// killing the connection when no data flows through the tunnel.
func TestStream_HeartbeatDuringSlowBuildPrompt(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Use a short heartbeat interval for testing (100ms instead of 5s)
	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(100*time.Millisecond),
	)

	chunkID := uuid.New()
	// Mock retrieval that takes 350ms (should trigger ~3 heartbeats at 100ms interval)
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			time.Sleep(350 * time.Millisecond)
		}).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{
					ChunkID:         chunkID,
					ChunkText:       "Test chunk",
					URL:             "http://example.com",
					Title:           "Example",
					PublishedAt:     "2025-12-25T00:00:00Z",
					Score:           0.9,
					DocumentVersion: 1,
				},
			},
		}, nil)

	// Prepare streaming response
	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)

	llmResponse := `{"answer": "Hello world [chunk_1]", "citations": [{"chunk_id":"` + chunkID.String() + `","reason":"relevant"}], "fallback": false, "reason": ""}`
	chunkChan <- domain.LLMStreamChunk{Response: llmResponse, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil)

	// Execute Stream
	eventChan := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test query"})

	// Collect all events
	var events []usecase.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Count heartbeat events
	heartbeatCount := 0
	for _, e := range events {
		if e.Kind == usecase.StreamEventKindHeartbeat {
			heartbeatCount++
		}
	}

	// With 350ms delay and 100ms interval, expect at least 2 heartbeats
	assert.GreaterOrEqual(t, heartbeatCount, 2,
		"should emit heartbeats during slow buildPrompt (got %d)", heartbeatCount)

	// Verify heartbeat payload is empty string
	for _, e := range events {
		if e.Kind == usecase.StreamEventKindHeartbeat {
			assert.Equal(t, "", e.Payload, "heartbeat payload should be empty")
		}
	}

	// Verify stream still completes successfully with done event
	lastEvent := events[len(events)-1]
	assert.Equal(t, usecase.StreamEventKindDone, lastEvent.Kind, "stream should complete with done event")
}

// TestStream_NoHeartbeatWhenBuildPromptFast verifies that no heartbeat events
// are emitted when buildPrompt completes quickly (under the heartbeat interval).
func TestStream_NoHeartbeatWhenBuildPromptFast(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Use a long heartbeat interval so no heartbeats fire during fast buildPrompt
	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(10*time.Second),
	)

	chunkID := uuid.New()
	// Mock instant retrieval - no delay
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk",
				URL:             "http://example.com",
				Title:           "Example",
				PublishedAt:     "2025-12-25T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}, nil)

	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)

	llmResponse := `{"answer": "Hello world [chunk_1]", "citations": [{"chunk_id":"` + chunkID.String() + `","reason":"relevant"}], "fallback": false, "reason": ""}`
	chunkChan <- domain.LLMStreamChunk{Response: llmResponse, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil)

	eventChan := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test query"})

	var events []usecase.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Count heartbeat events - should be zero
	heartbeatCount := 0
	for _, e := range events {
		if e.Kind == usecase.StreamEventKindHeartbeat {
			heartbeatCount++
		}
	}

	assert.Equal(t, 0, heartbeatCount, "should not emit heartbeats when buildPrompt is fast")
}

// TestStream_HeartbeatDuringChatStreamSetup verifies that heartbeat events are
// emitted while waiting for ChatStream() to establish a connection to Ollama.
// ChatStream() can block for seconds when the LLM is loading or busy.
func TestStream_HeartbeatDuringChatStreamSetup(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(100*time.Millisecond),
	)

	chunkID := uuid.New()
	// Instant retrieval
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk",
				URL:             "http://example.com",
				Title:           "Example",
				PublishedAt:     "2025-12-25T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}, nil)

	// ChatStream blocks for 350ms simulating slow Ollama connection
	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)

	llmResponse := `{"answer": "Hello world [chunk_1]", "citations": [{"chunk_id":"` + chunkID.String() + `","reason":"relevant"}], "fallback": false, "reason": ""}`

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			time.Sleep(350 * time.Millisecond) // Simulate slow connection setup
			chunkChan <- domain.LLMStreamChunk{Response: llmResponse, Done: false}
			chunkChan <- domain.LLMStreamChunk{Done: true}
			close(chunkChan)
			close(errChan)
		}).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil)

	eventChan := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test query"})

	var events []usecase.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Count heartbeat events
	heartbeatCount := 0
	for _, e := range events {
		if e.Kind == usecase.StreamEventKindHeartbeat {
			heartbeatCount++
		}
	}

	// With 350ms ChatStream delay and 100ms interval, expect at least 2 heartbeats
	assert.GreaterOrEqual(t, heartbeatCount, 2,
		"should emit heartbeats during slow ChatStream setup (got %d)", heartbeatCount)

	// Verify stream completes
	lastEvent := events[len(events)-1]
	assert.Equal(t, usecase.StreamEventKindDone, lastEvent.Kind, "stream should complete with done event")
}

// TestStream_HeartbeatDuringLLMStreaming verifies that heartbeat events are
// emitted when there are long gaps between LLM chunks (e.g., during thinking
// to generation phase transitions).
func TestStream_HeartbeatDuringLLMStreaming(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(100*time.Millisecond),
	)

	chunkID := uuid.New()
	// Instant retrieval
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk",
				URL:             "http://example.com",
				Title:           "Example",
				PublishedAt:     "2025-12-25T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}, nil)

	// Simulate LLM streaming with a long gap between chunks
	chunkChan := make(chan domain.LLMStreamChunk)
	errChan := make(chan error)

	llmResponse := `{"answer": "Hello world [chunk_1]", "citations": [{"chunk_id":"` + chunkID.String() + `","reason":"relevant"}], "fallback": false, "reason": ""}`

	go func() {
		// Send thinking chunk, then pause 350ms (simulating thinking->generation transition)
		chunkChan <- domain.LLMStreamChunk{Thinking: "Let me analyze...", Done: false}
		time.Sleep(350 * time.Millisecond) // Long gap — should trigger heartbeats
		chunkChan <- domain.LLMStreamChunk{Response: llmResponse, Done: false}
		chunkChan <- domain.LLMStreamChunk{Done: true}
		close(chunkChan)
		close(errChan)
	}()

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil)

	eventChan := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test query"})

	var events []usecase.StreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Count heartbeat events
	heartbeatCount := 0
	for _, e := range events {
		if e.Kind == usecase.StreamEventKindHeartbeat {
			heartbeatCount++
		}
	}

	// With 350ms gap and 100ms interval, expect at least 2 heartbeats during LLM streaming
	assert.GreaterOrEqual(t, heartbeatCount, 2,
		"should emit heartbeats during LLM streaming gaps (got %d)", heartbeatCount)

	// Verify stream completes
	lastEvent := events[len(events)-1]
	assert.Equal(t, usecase.StreamEventKindDone, lastEvent.Kind, "stream should complete with done event")
}
