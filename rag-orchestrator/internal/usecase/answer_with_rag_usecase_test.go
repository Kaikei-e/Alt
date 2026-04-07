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

type blockingRetryLLMClient struct {
	streamResponse string
	retryResponse  string
	retryStarted   chan struct{}
	allowRetry     chan struct{}
}

func (c *blockingRetryLLMClient) Generate(context.Context, string, int) (*domain.LLMResponse, error) {
	panic("unused")
}

func (c *blockingRetryLLMClient) GenerateStream(context.Context, string, int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	panic("unused")
}

func (c *blockingRetryLLMClient) ChatStream(_ context.Context, _ []domain.Message, _ int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	chunkCh := make(chan domain.LLMStreamChunk, 2)
	errCh := make(chan error)
	chunkCh <- domain.LLMStreamChunk{Response: c.streamResponse, Done: false}
	chunkCh <- domain.LLMStreamChunk{Done: true}
	close(chunkCh)
	close(errCh)
	return chunkCh, errCh, nil
}

func (c *blockingRetryLLMClient) Chat(ctx context.Context, _ []domain.Message, _ int) (*domain.LLMResponse, error) {
	select {
	case <-c.retryStarted:
	default:
		close(c.retryStarted)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.allowRetry:
		return &domain.LLMResponse{Text: c.retryResponse, Done: true}, nil
	}
}

func (c *blockingRetryLLMClient) Version() string {
	return "blocking-retry"
}

type mockRetrievalStrategy struct {
	mock.Mock
	name string
}

func (m *mockRetrievalStrategy) Name() string { return m.name }

func (m *mockRetrievalStrategy) Retrieve(ctx context.Context, input usecase.RetrieveContextInput, intent usecase.QueryIntent) (*usecase.RetrieveContextOutput, error) {
	args := m.Called(ctx, input, intent)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*usecase.RetrieveContextOutput), args.Error(1)
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

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger)

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
			contains(msgs[0].Content, "800文字以上") &&
			contains(msgs[0].Content, "結論を最初に") &&
			contains(msgs[0].Content, "引用は[番号]形式")
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

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 7, 512, 6000, "alpha-v1", "ja", testLogger)

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

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger)

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
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
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
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
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
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
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
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
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

func TestPromptBuilder_ContainsGroundingInstructions(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()

	messages, err := builder.Build(usecase.PromptInput{
		Query:         "test query",
		Locale:        "ja",
		PromptVersion: "alpha-v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	content := messages[0].Content
	assert.Contains(t, content, "提供されたコンテキスト情報のみに基づいて回答すること")
	assert.Contains(t, content, "推測・捏造しないこと")
	assert.Contains(t, content, "情報が不十分な場合は、不足している点を明示すること")
}

func TestPromptBuilder_ArticleContext(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()

	messages, err := builder.Build(usecase.PromptInput{
		Query:         "What are the key points?",
		Locale:        "ja",
		PromptVersion: "alpha-v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "AI Article", ChunkText: "content"},
		},
		ArticleContext: &usecase.ArticleContext{
			ArticleID: "art-123",
			Title:     "AI Article",
			Truncated: false,
		},
	})
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	content := messages[0].Content
	assert.Contains(t, content, "記事コンテキスト")
	assert.Contains(t, content, "AI Article")
	assert.Contains(t, content, "全内容です")
	assert.NotContains(t, content, "主要な部分")
}

func TestPromptBuilder_ArticleContextTruncated(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()

	messages, err := builder.Build(usecase.PromptInput{
		Query:         "Summarize",
		Locale:        "ja",
		PromptVersion: "alpha-v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Big Article", ChunkText: "partial content"},
		},
		ArticleContext: &usecase.ArticleContext{
			ArticleID: "art-456",
			Title:     "Big Article",
			Truncated: true,
		},
	})
	assert.NoError(t, err)

	content := messages[0].Content
	assert.Contains(t, content, "主要な部分です")
	assert.NotContains(t, content, "全内容です")
}

func TestPromptBuilder_GeneralNoArticleContext(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()

	messages, err := builder.Build(usecase.PromptInput{
		Query:         "What is AI?",
		Locale:        "ja",
		PromptVersion: "alpha-v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
	})
	assert.NoError(t, err)

	content := messages[0].Content
	assert.NotContains(t, content, "記事コンテキスト")
}

func TestExecute_ArticleScopedQuery_UsesNormalizedQuery(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()

	// The mock should receive the NORMALIZED query (UserQuestion), not the raw query
	mockRetrieve.On("Execute", mock.Anything, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		// For a general query going through generalStrategy, the query should be the UserQuestion
		return input.Query == "What are the key improvements?"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk about improvements",
				URL:             "http://example.com",
				Title:           "Example",
				PublishedAt:     "2025-12-25T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}, nil)

	llmResponse := `{"answer":"The key improvements are...","citations":[{"chunk_id":"1","reason":"relevant"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	// Use article-scoped query format — but no article_scoped strategy registered,
	// so it falls through to generalStrategy
	rawQuery := "Regarding the article: OpenAI GPT-5 [articleId: abc-123]\n\nQuestion:\nWhat are the key improvements?"
	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: rawQuery})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	// Strategy should be "general" since no article_scoped strategy was registered
	// and it will first try article_scoped (not found), fall through to general
}

func TestExecute_DebugIncludesStrategy(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Test chunk", Score: 0.9, DocumentVersion: 1},
		},
	}, nil)

	llmResponse := `{"answer":"Answer text","citations":[{"chunk_id":"1","reason":"r"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "general question"})
	assert.NoError(t, err)
	assert.Equal(t, "general", output.Debug.StrategyUsed)
}

func TestStream_DebugIncludesStrategy(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Test chunk", URL: "http://example.com", Title: "Test", PublishedAt: "2025-12-25T00:00:00Z", Score: 0.9, DocumentVersion: 1},
		},
	}, nil)

	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)
	llmResponse := `{"answer":"Hello","citations":[{"chunk_id":"` + chunkID.String() + `","reason":"r"}],"fallback":false,"reason":""}`
	chunkChan <- domain.LLMStreamChunk{Response: llmResponse, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil)

	events := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test query"})

	var metaEvent *usecase.StreamMeta
	for event := range events {
		if event.Kind == usecase.StreamEventKindMeta {
			meta := event.Payload.(usecase.StreamMeta)
			metaEvent = &meta
		}
	}

	assert.NotNil(t, metaEvent)
	assert.Equal(t, "general", metaEvent.Debug.StrategyUsed)
}

func TestExecute_ArticleScopedFollowUp_InheritsScopeFromHistory(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
	)

	chunkID := uuid.New()
	articleStrategy.
		On("Retrieve", mock.Anything, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
			return input.Query == "What is the impact?"
		}), mock.MatchedBy(func(intent usecase.QueryIntent) bool {
			return intent.IntentType == usecase.IntentArticleScoped &&
				intent.ArticleID == "art-123" &&
				intent.ArticleTitle == "OpenAI GPT-5" &&
				intent.UserQuestion == "What is the impact?"
		})).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{
					ChunkID:         chunkID,
					ChunkText:       "Impact details",
					Title:           "OpenAI GPT-5",
					Score:           1.0,
					DocumentVersion: 1,
				},
			},
		}, nil)

	// General strategy (re-retrieval) for follow-up augmentation
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: uuid.New(), ChunkText: "Related context from global index", Title: "Related", Score: 0.7, DocumentVersion: 1},
		},
	}, nil)

	llmResponse := `{"answer":"Impact summary","citations":[{"chunk_id":"1","reason":"relevant"}],"fallback":false,"reason":""}`
	// Multi-turn produces actual chat turns: 2 history messages + 1 current user message
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		if len(msgs) < 3 {
			return false
		}
		lastMsg := msgs[len(msgs)-1]
		return lastMsg.Role == "user" &&
			contains(lastMsg.Content, "What is the impact?")
	}), mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{
		Query: "What is the impact?",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "Regarding the article: OpenAI GPT-5 [articleId: art-123]\n\nQuestion:\nWhat changed?"},
			{Role: "assistant", Content: "It improved several areas."},
		},
	})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Equal(t, "article_scoped+general", output.Debug.StrategyUsed)
}

func TestExecute_ArticleScopedFollowUp_FromHistory_ClassifiesCritiqueAndPreservesDebug(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}
	classifier := usecase.NewQueryClassifier(nil, 0)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
		usecase.WithQueryClassifier(classifier),
	)

	chunkID := uuid.New()
	articleStrategy.
		On("Retrieve", mock.Anything, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
			return input.Query == "反論はある？"
		}), mock.MatchedBy(func(intent usecase.QueryIntent) bool {
			return intent.IntentType == usecase.IntentArticleScoped &&
				intent.SubIntentType == usecase.SubIntentCritique &&
				intent.ArticleID == "art-critique" &&
				intent.UserQuestion == "反論はある？"
		})).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{
					ChunkID:         chunkID,
					ChunkText:       "Article claim and evidence",
					Title:           "Test Article",
					Score:           1.0,
					DocumentVersion: 1,
				},
			},
		}, nil)

	// With the subintent-gated retrieval policy, critique subintent without a
	// quality assessor will NOT trigger general re-retrieval — stays article-only.

	llmResponse := `{"answer":"批判的な回答","citations":[{"chunk_id":"1","reason":"relevant"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		if len(msgs) < 3 {
			return false
		}
		lastMsg := msgs[len(msgs)-1]
		return lastMsg.Role == "user" &&
			contains(lastMsg.Content, "反論はある？") &&
			contains(lastMsg.Content, "弱点") &&
			contains(lastMsg.Content, "反証")
	}), mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{
		Query: "反論はある？",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "Regarding the article: Test Article [articleId: art-critique]\n\nQuestion:\n要点は？"},
			{Role: "assistant", Content: "記事の要点は..."},
		},
	})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Equal(t, "article_scoped", output.Debug.StrategyUsed)
	assert.Equal(t, "critique", output.Debug.SubIntentType)
	// General strategy should NOT be called — no quality assessor configured
	mockRetrieve.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
}

func TestExecute_ArticleScopedFollowUp_ReRetrievesFromGlobalIndex(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
	)

	chunkID1 := uuid.New()
	chunkID2 := uuid.New()

	// Article-scoped strategy returns article's own chunks
	articleStrategy.
		On("Retrieve", mock.Anything, mock.Anything, mock.Anything).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{ChunkID: chunkID1, ChunkText: "Article chunk about fuel crisis", Title: "Fuel Crisis", Score: 1.0, DocumentVersion: 1},
			},
		}, nil)

	// General strategy (mockRetrieve) should be called for follow-up re-retrieval
	// and return ADDITIONAL context from the global index
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID2, ChunkText: "Related article about Middle East geopolitics", Title: "Geopolitics", Score: 0.8, DocumentVersion: 1},
		},
	}, nil)

	llmResponse := `{"answer":"Combined answer","citations":[{"chunk_id":"1","reason":"r"},{"chunk_id":"2","reason":"r"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{
		Query: "Why did this crisis happen?",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "Regarding the article: Fuel Crisis [articleId: art-fuel]\n\nQuestion:\nSummary?"},
			{Role: "assistant", Content: "The article discusses a fuel crisis in Australia."},
		},
	})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)

	// General strategy should have been called for re-retrieval (agentic pattern)
	mockRetrieve.AssertCalled(t, "Execute", mock.Anything, mock.Anything)

	// Output should contain contexts from BOTH article and general retrieval
	assert.Greater(t, len(output.Contexts), 1,
		"follow-up should include contexts from both article and global index")
}

func TestExecute_FallbackDebugPreservesStrategyUsed(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Test chunk", Score: 0.9, DocumentVersion: 1},
		},
	}, nil)

	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "general question"})
	assert.NoError(t, err)
	assert.True(t, output.Fallback)
	assert.Equal(t, "general", output.Debug.StrategyUsed)
}

func TestExecute_ArticleScopedMaxChunksMarksPromptAsTruncated(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 1, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
	)

	chunkID1 := uuid.New()
	chunkID2 := uuid.New()
	articleStrategy.
		On("Retrieve", mock.Anything, mock.Anything, mock.MatchedBy(func(intent usecase.QueryIntent) bool {
			return intent.ArticleID == "art-123"
		})).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{ChunkID: chunkID1, ChunkText: "Chunk 1", Title: "Large Article", Score: 1.0, DocumentVersion: 1},
				{ChunkID: chunkID2, ChunkText: "Chunk 2", Title: "Large Article", Score: 1.0, DocumentVersion: 1},
			},
		}, nil)

	llmResponse := `{"answer":"Partial summary","citations":[{"chunk_id":"1","reason":"relevant"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		return len(msgs) == 1 &&
			contains(msgs[0].Content, "主要な部分です") &&
			!contains(msgs[0].Content, "全内容です")
	}), mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{
		Query: "Regarding the article: Large Article [articleId: art-123]\n\nQuestion:\nSummarize it.",
	})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Len(t, output.Contexts, 1)
}

func TestPromptBuilder_MultiTurnCoreferenceInstruction(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()

	messages, err := builder.Build(usecase.PromptInput{
		Query:         "それについて詳しく教えて",
		Locale:        "ja",
		PromptVersion: "alpha-v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "content"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "AIについて教えて"},
			{Role: "assistant", Content: "AIとは人工知能のことです。"},
		},
	})
	assert.NoError(t, err)

	// Multi-turn: 2 history messages + 1 current user message = 3
	assert.Len(t, messages, 3)

	// Past turns should be actual chat messages
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "assistant", messages[1].Role)

	// Last message is the follow-up with instructions
	lastMsg := messages[2]
	assert.Contains(t, lastMsg.Content, "繰り返さない",
		"follow-up should instruct not to repeat")
}

// --- Phase 1: Retrieval Quality Gate integration tests ---

type mockQueryExpander struct {
	mock.Mock
}

func (m *mockQueryExpander) ExpandQuery(ctx context.Context, query string, jaCount, enCount int) ([]string, error) {
	args := m.Called(ctx, query, jaCount, enCount)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockQueryExpander) ExpandQueryWithHistory(ctx context.Context, query string, history []domain.Message, jaCount, enCount int) ([]string, error) {
	args := m.Called(ctx, query, history, jaCount, enCount)
	return args.Get(0).([]string), args.Error(1)
}

func TestQualityGate_GoodQuality_NoRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	assessor := usecase.NewRetrievalQualityAssessor(0.5, 0.25, 1)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithQualityAssessor(assessor, nil),
	)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Good chunk", Score: 0.9, RerankScore: 0.9, Title: "T", URL: "http://x.com", PublishedAt: "2025-01-01T00:00:00Z", DocumentVersion: 1},
		},
	}, nil)

	llmResponse := `{"answer": "Answer here", "citations": [], "fallback": false, "reason": ""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "test"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Equal(t, "good", output.Debug.RetrievalQuality)
	assert.Equal(t, 0, output.Debug.RetryCount)
}

func TestQualityGate_InsufficientQuality_ReturnsFallback(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	assessor := usecase.NewRetrievalQualityAssessor(0.5, 0.25, 1)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithQualityAssessor(assessor, nil),
	)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Bad chunk", Score: 0.05, RerankScore: 0.05, Title: "T", URL: "http://x.com", PublishedAt: "2025-01-01T00:00:00Z", DocumentVersion: 1},
		},
	}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "test"})
	assert.NoError(t, err)
	assert.True(t, output.Fallback)
	assert.Contains(t, output.Reason, "retrieval quality insufficient")
}

func TestQualityGate_MarginalQuality_TriggersRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockExpander := new(mockQueryExpander)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	assessor := usecase.NewRetrievalQualityAssessor(0.5, 0.25, 1)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithQualityAssessor(assessor, mockExpander),
	)

	chunkID := uuid.New()
	// First retrieval returns marginal quality
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Marginal chunk", Score: 0.3, RerankScore: 0.3, Title: "T", URL: "http://x.com", PublishedAt: "2025-01-01T00:00:00Z", DocumentVersion: 1},
		},
	}, nil).Once()

	// Query expander returns a rewritten query
	mockExpander.On("ExpandQueryWithHistory", mock.Anything, mock.Anything, mock.Anything, 2, 2).
		Return([]string{"expanded query"}, nil)

	chunkID2 := uuid.New()
	// Second retrieval (retry) returns good quality via general strategy
	// Note: the retry calls generalStrategy which internally calls RetrieveContextUsecase.Execute
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID2, ChunkText: "Good chunk", Score: 0.8, RerankScore: 0.8, Title: "T2", URL: "http://y.com", PublishedAt: "2025-01-01T00:00:00Z", DocumentVersion: 1},
		},
	}, nil).Once()

	llmResponse := `{"answer": "Better answer", "citations": [], "fallback": false, "reason": ""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "test"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Equal(t, 1, output.Debug.RetryCount)
	assert.Contains(t, output.Debug.StrategyUsed, "retried")
	mockExpander.AssertCalled(t, "ExpandQueryWithHistory", mock.Anything, mock.Anything, mock.Anything, 2, 2)
}

func TestExecute_ShortAnswer_TriggersCorrectiveRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(20),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.2, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()

	shortResponse := `{"answer":"短い","citations":[{"chunk_id":"1","reason":"short"}],"fallback":false,"reason":""}`
	longResponse := `{"answer":"` + strings.Repeat("世界的な物流混乱は、供給制約、輸送遅延、港湾処理の逼迫が重なって発生しました。背景には需要の急回復と在庫の偏在があり、構造要因として物流網の集中と代替輸送能力の不足が影響しています。結果として、納期の長期化、価格上昇、在庫再編の必要性が生じました。", 3) + `","citations":[{"chunk_id":"1","reason":"retry"},{"chunk_id":"1","reason":"supporting detail"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: shortResponse, Done: true}, nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: longResponse, Done: true}, nil).Once()

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "供給制約")
	assert.Len(t, output.Citations, 2)
}

func TestStream_ShortAnswer_TriggersCorrectiveRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(20),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(10*time.Millisecond),
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.2, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()

	shortResponse := `{"answer":"短い","citations":[{"chunk_id":"1","reason":"short"}],"fallback":false,"reason":""}`
	longResponse := `{"answer":"世界的な物流混乱は、供給制約、輸送遅延、港湾処理の逼迫が重なって発生した可能性が高いです。","citations":[{"chunk_id":"1","reason":"retry"}],"fallback":false,"reason":""}`

	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)
	chunkChan <- domain.LLMStreamChunk{Response: shortResponse, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: longResponse, Done: true}, nil).Once()

	events := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？"})

	var gotAnswer string
	var doneSeen bool
	for event := range events {
		switch event.Kind {
		case usecase.StreamEventKindDelta:
			gotAnswer = event.Payload.(string)
		case usecase.StreamEventKindDone:
			doneSeen = true
		}
	}

	assert.True(t, doneSeen)
	assert.Contains(t, gotAnswer, "供給制約")
}

func TestExecute_ShortAnswer_RetryStillShortButGroundedReturnsAnswer(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(80),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.2, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()

	shortResponse := `{"answer":"短い","citations":[{"chunk_id":"1","reason":"short"}],"fallback":false,"reason":""}`
	stillShortButGrounded := `{"answer":"供給制約と輸送遅延が重なり、価格上昇と供給不安が拡大しました。現時点では背景要因の切り分けがなお必要です。","citations":[{"chunk_id":"1","reason":"retry"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: shortResponse, Done: true}, nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: stillShortButGrounded, Done: true}, nil).Once()

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "供給制約")
	assert.Equal(t, 1, output.Debug.RetryCount)
}

func TestStream_ShortAnswer_RetryStillShortButGroundedReturnsAnswer(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(80),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(10*time.Millisecond),
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.2, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()

	shortResponse := `{"answer":"短い","citations":[{"chunk_id":"1","reason":"short"}],"fallback":false,"reason":""}`
	stillShortButGrounded := `{"answer":"供給制約と輸送遅延が重なり、価格上昇と供給不安が拡大しました。現時点では背景要因の切り分けがなお必要です。","citations":[{"chunk_id":"1","reason":"retry"}],"fallback":false,"reason":""}`

	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)
	chunkChan <- domain.LLMStreamChunk{Response: shortResponse, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: stillShortButGrounded, Done: true}, nil).Once()

	events := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？"})

	var gotAnswer string
	var fallbackSeen bool
	for event := range events {
		switch event.Kind {
		case usecase.StreamEventKindDelta:
			gotAnswer = event.Payload.(string)
		case usecase.StreamEventKindFallback:
			fallbackSeen = true
		}
	}

	assert.False(t, fallbackSeen)
	assert.Contains(t, gotAnswer, "供給制約")
}

func TestExecute_CausalShortGroundedAnswer_DoesNotRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(500),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
	)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()

	groundedButShort := `{"answer":"` + strings.Repeat("供給制約と輸送遅延、保管能力の逼迫が重なり、市場の不安定化が進みました。", 4) + `","citations":[{"chunk_id":"1","reason":"grounded"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: groundedButShort, Done: true}, nil).Once()

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "供給制約")
	assert.Equal(t, 0, output.Debug.RetryCount)
	mockLLM.AssertNumberOfCalls(t, "Chat", 1)
}

func TestStream_CausalShortGroundedAnswer_DoesNotRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(500),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(10*time.Millisecond),
	)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()

	groundedButShort := `{"answer":"` + strings.Repeat("供給制約と輸送遅延、保管能力の逼迫が重なり、市場の不安定化が進みました。", 4) + `","citations":[{"chunk_id":"1","reason":"grounded"}],"fallback":false,"reason":""}`
	chunkChan := make(chan domain.LLMStreamChunk, 2)
	errChan := make(chan error)
	chunkChan <- domain.LLMStreamChunk{Response: groundedButShort, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil).Once()

	events := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？"})

	var gotAnswer string
	for event := range events {
		if event.Kind == usecase.StreamEventKindDelta {
			gotAnswer = event.Payload.(string)
		}
	}

	assert.Contains(t, gotAnswer, "供給制約")
	mockLLM.AssertNumberOfCalls(t, "Chat", 0)
}

func TestExecute_DetailedCausalShortGroundedAnswer_TriggersRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(500),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.95, DocumentVersion: 1},
		},
	}, nil).Once()

	groundedButShort := `{"answer":"` + strings.Repeat("供給制約と輸送遅延、保管能力の逼迫が重なり、市場の不安定化が進みました。", 4) + `","citations":[{"chunk_id":"1","reason":"grounded"}],"fallback":false,"reason":""}`
	longerRetry := `{"answer":"` + strings.Repeat("供給制約と輸送遅延が直接要因です。背景では需要の急回復と港湾処理能力の逼迫が重なりました。さらに、在庫の偏在と物流ネットワークの集中が構造要因として作用し、遅延が連鎖的に拡大しました。結果として、納期の長期化、価格上昇、代替調達コストの増加が発生し、企業の在庫戦略にも再設計が求められました。", 3) + `","citations":[{"chunk_id":"1","reason":"retry"},{"chunk_id":"1","reason":"supporting detail"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: groundedButShort, Done: true}, nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: longerRetry, Done: true}, nil).Once()

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？詳しく教えて。"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "構造要因")
	assert.Equal(t, 1, output.Debug.RetryCount)
	mockLLM.AssertNumberOfCalls(t, "Chat", 2)
}

func TestStream_DetailedCausalShortGroundedAnswer_TriggersRetry(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(500),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(10*time.Millisecond),
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.95, DocumentVersion: 1},
		},
	}, nil).Once()

	groundedButShort := `{"answer":"` + strings.Repeat("供給制約と輸送遅延、保管能力の逼迫が重なり、市場の不安定化が進みました。", 4) + `","citations":[{"chunk_id":"1","reason":"grounded"}],"fallback":false,"reason":""}`
	longerRetry := `{"answer":"` + strings.Repeat("供給制約と輸送遅延が直接要因です。背景では需要の急回復と港湾処理能力の逼迫が重なりました。さらに、在庫の偏在と物流ネットワークの集中が構造要因として作用し、遅延が連鎖的に拡大しました。結果として、納期の長期化、価格上昇、代替調達コストの増加が発生し、企業の在庫戦略にも再設計が求められました。", 3) + `","citations":[{"chunk_id":"1","reason":"retry"},{"chunk_id":"1","reason":"supporting detail"}],"fallback":false,"reason":""}`

	// Hybrid streaming: strictLongForm now uses ChatStream for initial attempt
	chunkChan := make(chan domain.LLMStreamChunk, 10)
	errChan := make(chan error)
	chunkChan <- domain.LLMStreamChunk{Response: groundedButShort, Done: false}
	chunkChan <- domain.LLMStreamChunk{Done: true}
	close(chunkChan)
	close(errChan)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkChan), (<-chan error)(errChan), nil).Once()

	// Corrective retry uses non-streaming Chat
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: longerRetry, Done: true}, nil).Once()

	events := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？詳しく教えて。"})

	var gotAnswer string
	for event := range events {
		if event.Kind == usecase.StreamEventKindDone {
			output := event.Payload.(*usecase.AnswerWithRAGOutput)
			gotAnswer = output.Answer
		}
	}

	assert.Contains(t, gotAnswer, "構造要因")
	mockLLM.AssertNumberOfCalls(t, "ChatStream", 1)
	mockLLM.AssertNumberOfCalls(t, "Chat", 1)
}

func TestStream_DetailedQuery_EmitsRefiningBeforeRetryStarts(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()
	retrievalOutput := &usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Supply bottleneck context", Title: "Supply", Score: 0.8, DocumentVersion: 1},
			{ChunkID: secondChunkID, ChunkText: "Port congestion context", Title: "Port", Score: 0.79, DocumentVersion: 1},
		},
	}
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievalOutput, nil).Twice()

	shortResponse := `{"answer":"短い説明です。","citations":[{"chunk_id":"` + firstChunkID.String() + `","reason":"initial"}],"fallback":false,"reason":""}`
	longResponse := `{"answer":"` + strings.Repeat("物流混乱は、供給制約、港湾滞留、輸送網の同期失敗が連鎖して拡大しました。", 8) + `","citations":[{"chunk_id":"` + firstChunkID.String() + `","reason":"supply"},{"chunk_id":"` + secondChunkID.String() + `","reason":"port"}],"fallback":false,"reason":""}`
	llm := &blockingRetryLLMClient{
		streamResponse: shortResponse,
		retryResponse:  longResponse,
		retryStarted:   make(chan struct{}),
		allowRetry:     make(chan struct{}),
	}

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, llm, usecase.NewOutputValidator(80),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(10*time.Millisecond),
	)

	events := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "物流混乱の背景を詳しく教えて"})

	sawRefining := false
	retryStarted := false
	retryStartedCh := llm.retryStarted
	timeout := time.After(2 * time.Second)

loop:
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for refining progress")
		case <-retryStartedCh:
			retryStarted = true
			retryStartedCh = nil
			if !sawRefining {
				t.Fatal("retry started before refining progress was emitted")
			}
			close(llm.allowRetry)
		case event, ok := <-events:
			if !ok {
				break loop
			}
			if event.Kind == usecase.StreamEventKindProgress && event.Payload == "refining" {
				sawRefining = true
			}
			if retryStarted && event.Kind == usecase.StreamEventKindDone {
				break loop
			}
		}
	}

	assert.True(t, sawRefining)
	assert.True(t, retryStarted)
}

func TestExecute_CorrectiveRetryContextDisclaimer_KeepsOriginalAnswer(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(220),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.7, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.8, DocumentVersion: 1},
		},
	}, nil).Once()

	originalShortButGrounded := `{"answer":"供給制約と輸送遅延が重なり、物流網の混乱が広がりました。背景要因として需要変動と港湾処理能力の逼迫も確認できます。","citations":[{"chunk_id":"1","reason":"grounded"}],"fallback":false,"reason":""}`
	retryContextDisclaimer := `{"answer":"提供されたコンテキストには、世界的な物流混乱に関する情報、因果分析、または関連するデータは含まれていません。","citations":[],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: originalShortButGrounded, Done: true}, nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: retryContextDisclaimer, Done: true}, nil).Once()

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "世界的な物流混乱はなぜ起きた？"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "供給制約")
	assert.NotContains(t, output.Answer, "提供されたコンテキストには")
}

func TestExecute_ShortAnswer_RetryStillShortAndUngroundedFallsBack(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(80),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Initial context", Title: "Initial", Score: 0.2, DocumentVersion: 1},
		},
	}, nil).Once()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: secondChunkID, ChunkText: "Retry context", Title: "Retry", Score: 0.9, DocumentVersion: 1},
		},
	}, nil).Once()

	shortResponse := `{"answer":"short","citations":[{"chunk_id":"1","reason":"short"}],"fallback":false,"reason":""}`
	stillShortAndUngrounded := `{"answer":"Unrelated.","citations":[{"chunk_id":"1","reason":"retry"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: shortResponse, Done: true}, nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.LLMResponse{Text: stillShortAndUngrounded, Done: true}, nil).Once()

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "What caused the global logistics disruption?"})
	assert.NoError(t, err)
	assert.True(t, output.Fallback)
	assert.Equal(t, usecase.FallbackShortUnderGrounded, output.FallbackCategory)
}

// --- Phase 2: PromptBuilder intent-aware instruction tests ---

func TestPromptBuilder_ComparisonIntent_AddsInstruction(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "RustとGoの違い",
		PromptVersion: "test",
		IntentType:    usecase.IntentComparison,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Rust", ChunkText: "Rust info", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	assert.Contains(t, messages[0].Content, "比較")
}

func TestPromptBuilder_TemporalIntent_AddsInstruction(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "最近のAIニュース",
		PromptVersion: "test",
		IntentType:    usecase.IntentTemporal,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "AI News", ChunkText: "AI info", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	assert.Contains(t, messages[0].Content, "最新")
}

func TestPromptBuilder_FactCheckIntent_AddsInstruction(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "量子コンピュータは暗号を解ける？",
		PromptVersion: "test",
		IntentType:    usecase.IntentFactCheck,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Quantum", ChunkText: "Quantum info", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	assert.Contains(t, messages[0].Content, "根拠")
}

func TestPromptBuilder_GeneralIntent_NoExtraInstruction(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "AI技術のトレンド",
		PromptVersion: "test",
		IntentType:    usecase.IntentGeneral,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "AI", ChunkText: "AI info", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	// General should NOT have comparison/temporal/factcheck-specific instructions
	assert.NotContains(t, messages[0].Content, "両者を公平に比較")
	assert.NotContains(t, messages[0].Content, "根拠と判定を構造化")
}

// --- Subintent-specific prompt template tests (Augur RAG remediation) ---

func TestPromptBuilder_SingleTurn_DetailSubIntent_DirectAnswerFormat(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "技術的な詳細をもっと教えて",
		PromptVersion: "test",
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentDetail,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Article", ChunkText: "content", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	content := messages[0].Content
	assert.Contains(t, content, "質問に直接回答")
	assert.NotContains(t, content, "## 概要\\n...\\n## 詳細\\n...\\n## まとめ")
}

func TestPromptBuilder_SingleTurn_RelatedArticlesSubIntent_ListFormat(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "関連する記事はある？",
		PromptVersion: "test",
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentRelatedArticles,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Article", ChunkText: "content", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	content := messages[0].Content
	assert.Contains(t, content, "ランク付きリスト")
}

func TestPromptBuilder_SingleTurn_EvidenceSubIntent_CitedPassages(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "根拠は？",
		PromptVersion: "test",
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentEvidence,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Article", ChunkText: "content", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	content := messages[0].Content
	assert.Contains(t, content, "引用付き")
}

func TestPromptBuilder_SingleTurn_SummaryRefreshSubIntent_Concise(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "要約して",
		PromptVersion: "test",
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentSummaryRefresh,
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Article", ChunkText: "content", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	content := messages[0].Content
	assert.NotContains(t, content, "800文字以上")
	assert.Contains(t, content, "簡潔")
}

func TestPromptBuilder_MultiTurn_DetailSubIntent_AddsGuidance(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:         "技術的な詳細をもっと教えて",
		PromptVersion: "test",
		IntentType:    usecase.IntentArticleScoped,
		SubIntentType: usecase.SubIntentDetail,
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "概要を教えて"},
			{Role: "assistant", Content: "記事の概要です。"},
		},
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Article", ChunkText: "content", Score: 0.9},
		},
	})
	assert.NoError(t, err)
	// Last message should contain detail-specific guidance
	lastMsg := messages[len(messages)-1]
	assert.Contains(t, lastMsg.Content, "技術的")
}

func TestPromptBuilder_ToolOnly_RelatedArticlesSubIntent_DoesNotRequireContextChunks(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	messages, err := builder.Build(usecase.PromptInput{
		Query:             "関連する記事はある？",
		PromptVersion:     "test",
		IntentType:        usecase.IntentArticleScoped,
		SubIntentType:     usecase.SubIntentRelatedArticles,
		Contexts:          []usecase.PromptContext{}, // Empty — tool-only path
		SupplementaryInfo: []string{"Related articles:\n- Article A\n- Article B"},
	})
	assert.NoError(t, err)
	content := messages[0].Content
	assert.Contains(t, content, "Article A")
	assert.Contains(t, content, "ランク付きリスト")
}

// --- Retrieval policy gating tests (Augur RAG remediation) ---

func TestExecute_ArticleScopedFollowUp_DetailSubIntent_SkipsGeneralReRetrieval(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
		usecase.WithQueryClassifier(usecase.NewQueryClassifier(nil, 0)),
	)

	chunkID := uuid.New()
	articleStrategy.
		On("Retrieve", mock.Anything, mock.Anything, mock.Anything).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{ChunkID: chunkID, ChunkText: "記憶の定着メカニズムの詳細", Title: "暗記のコツ", Score: 1.0, DocumentVersion: 1},
			},
		}, nil)

	llmResponse := `{"answer":"技術的な詳細の回答","citations":[{"chunk_id":"1","reason":"r"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	_, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{
		Query: "技術的な詳細をもっと教えて",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "Regarding the article: 暗記のコツ [articleId: art-memory]\n\nQuestion:\n概要を教えて"},
			{Role: "assistant", Content: "記事は暗記のコツについてです。"},
		},
	})
	assert.NoError(t, err)

	// General strategy (mockRetrieve) must NOT be called — detail subintent stays article-only
	mockRetrieve.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
}

func TestExecute_ArticleScopedFollowUp_NoneSubIntent_KeepsGeneralReRetrieval(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	articleStrategy := &mockRetrievalStrategy{name: "article_scoped"}

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0), 10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithStrategy(usecase.IntentArticleScoped, articleStrategy),
		usecase.WithQueryClassifier(usecase.NewQueryClassifier(nil, 0)),
	)

	chunkID1 := uuid.New()
	chunkID2 := uuid.New()

	articleStrategy.
		On("Retrieve", mock.Anything, mock.Anything, mock.Anything).
		Return(&usecase.RetrieveContextOutput{
			Contexts: []usecase.ContextItem{
				{ChunkID: chunkID1, ChunkText: "Article chunk", Title: "Test Article", Score: 1.0, DocumentVersion: 1},
			},
		}, nil)

	// General strategy SHOULD be called for SubIntentNone (backward compat)
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID2, ChunkText: "General chunk", Title: "General Article", Score: 0.8, DocumentVersion: 1},
		},
	}, nil)

	llmResponse := `{"answer":"回答","citations":[{"chunk_id":"1","reason":"r"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{Text: llmResponse, Done: true}, nil)

	_, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{
		// This query matches SubIntentNone — no keywords for detail/related/evidence/etc.
		Query: "もっと教えて",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "Regarding the article: Test [articleId: art-test]\n\nQuestion:\n概要を教えて"},
			{Role: "assistant", Content: "テスト記事です。"},
		},
	})
	assert.NoError(t, err)

	// General strategy SHOULD be called for backward compatibility
	mockRetrieve.AssertCalled(t, "Execute", mock.Anything, mock.Anything)
}
