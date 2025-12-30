package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
	"strings"

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

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 10, 512, "alpha-v1", "ja", testLogger)

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

	// Expect Single Call
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		// Check for specific instruction (updated prompt)
		return len(msgs) > 0 && msgs[0].Role == "system" &&
			contains(msgs[0].Content, "synthesizing information from the provided context documents") &&
			contains(msgs[0].Content, "Value the information in the documents regardless of their language")
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

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 7, 512, "alpha-v1", "ja", testLogger)

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
		return len(msgs) > 0 && msgs[0].Role == "system" &&
			contains(msgs[0].Content, "synthesizing information from the provided context documents") &&
			contains(msgs[0].Content, "Value the information in the documents regardless of their language")
	}), mock.Anything).Return(&domain.LLMResponse{Text: fallbackResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "query"})
	assert.NoError(t, err)
	assert.True(t, output.Fallback)
	assert.Equal(t, "insufficient evidence", output.Reason)
	assert.Len(t, output.Citations, 0)
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

