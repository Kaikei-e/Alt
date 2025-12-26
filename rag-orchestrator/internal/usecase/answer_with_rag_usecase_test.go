package usecase_test

import (
	"context"
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

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 5, 512, "alpha-v1", "ja")

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

	// Stage 1: Citations Response
	citationsResponse := `{
  "quotes": [{"chunk_id":"` + chunkID.String() + `","quote":"quote text"}],
  "citations": [{"chunk_id":"` + chunkID.String() + `","url":"http://example.com","title":"Example","score":0.9}],
  "answer": "",
  "fallback": false
}`
	// Stage 2: Answer Response
	answerResponse := `{
  "answer": "Hello world",
  "fallback": false,
  "reason": ""
}`

	// Expect Stage 1 Call (we can use MatchedBy to distinguish, or just order)
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		// Check for Stage 1 characteristic (e.g. system prompt instruction)
		return len(msgs) > 0 && msgs[0].Role == "system" && contains(msgs[0].Content, "Extract exact quotes")
	}), mock.Anything).Return(&domain.LLMResponse{Text: citationsResponse, Done: true}, nil)

	// Expect Stage 2 Call
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		// Check for Stage 2 characteristic
		return len(msgs) > 0 && msgs[0].Role == "system" && contains(msgs[0].Content, "Answer the query")
	}), mock.Anything).Return(&domain.LLMResponse{Text: answerResponse, Done: true}, nil)

	output, err := uc.Execute(ctx, usecase.AnswerWithRAGInput{Query: "query"})
	assert.NoError(t, err)
	assert.False(t, output.Fallback)
	assert.Equal(t, "Hello world", output.Answer)
	assert.Len(t, output.Citations, 1)
	assert.Equal(t, chunkID.String(), output.Citations[0].ChunkID)
}

func TestAnswerWithRAG_Fallback(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 5, 512, "alpha-v1", "ja")

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk",
				Score:           0.5,
				DocumentVersion: 1,
			},
		},
	}, nil)

	// Test Fallback
	// Stage 1 might succeed or fail?
	// Case 1: Stage 1 returns insufficient evidence?
	// The prompt says "return ... fallback:true".

	// Stage 1 response
	stage1Resp := `{
		"quotes": [],
		"citations": [],
		"fallback": false
	}`
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		return len(msgs) > 0 && msgs[0].Role == "system" && contains(msgs[0].Content, "Extract exact quotes")
	}), mock.Anything).Return(&domain.LLMResponse{Text: stage1Resp, Done: true}, nil)

	// Stage 2 response (Fallback)
	fallbackResponse := `{
  "answer": "",
  "fallback": true,
  "reason": "insufficient evidence"
}`
	mockLLM.On("Chat", mock.Anything, mock.MatchedBy(func(msgs []domain.Message) bool {
		return len(msgs) > 0 && msgs[0].Role == "system" && contains(msgs[0].Content, "Answer the query")
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
