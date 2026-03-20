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

	llmResponse := `{"answer":"Impact summary","citations":[{"chunk_id":"1","reason":"relevant"}],"fallback":false,"reason":""}`
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		return len(msgs) == 1 &&
			contains(msgs[0].Content, "記事コンテキスト") &&
			contains(msgs[0].Content, "OpenAI GPT-5") &&
			contains(msgs[0].Content, "What is the impact?")
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
	assert.Equal(t, "article_scoped", output.Debug.StrategyUsed)
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
	assert.Len(t, messages, 1)

	content := messages[0].Content
	assert.Contains(t, content, "指示代名詞（「それ」「この件」等）を解決してください")
}
