package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockVectorEncoder
type MockVectorEncoder struct {
	mock.Mock
}

func (m *MockVectorEncoder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	args := m.Called(ctx, texts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]float32), args.Error(1)
}

func (m *MockVectorEncoder) Version() string {
	return "mock-v1"
}

func TestRetrieveContext_Execute_Success(t *testing.T) {
	mockChunkRepo := new(MockRagChunkRepository)
	mockDocRepo := new(MockRagDocumentRepository)
	mockEncoder := new(MockVectorEncoder)
	mockLLM := new(mockLLMClient) // Defined in answer_with_rag_usecase_test.go
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewRetrieveContextUsecase(mockChunkRepo, mockDocRepo, mockEncoder, mockLLM, nil, testLogger)

	ctx := context.Background()
	input := usecase.RetrieveContextInput{
		Query: "search query",
	}

	// Expectations
	// 1. Expand Query
	// LLM returns 3 variations
	mockLLM.On("Generate", ctx, mock.MatchedBy(func(prompt string) bool {
		return msgContains(prompt, "expert search query generator") && msgContains(prompt, "search query")
	}), 200).Return(&domain.LLMResponse{
		Text: "variation 1\nvariation 2\nvariation 3",
	}, nil)

	// 2. Encode
	// Expect original + 3 variations
	expectedQueries := []string{"search query", "variation 1", "variation 2", "variation 3"}
	// We need multiple embeddings
	queryVec := []float32{0.1, 0.2, 0.3}
	mockEncoder.On("Encode", ctx, expectedQueries).Return([][]float32{queryVec, queryVec, queryVec, queryVec}, nil)

	// 3. Search
	// Expect 4 searches
	mockChunkRepo.On("Search", ctx, queryVec, []string(nil), 50).Return([]domain.SearchResult{
		{
			Chunk: domain.RagChunk{
				ID:      uuid.New(),
				Content: "Found content",
			},
			Score:           0.95,
			ArticleID:       "art-1",
			DocumentVersion: 1,
		},
	}, nil).Times(4)

	// Execute
	output, err := uc.Execute(ctx, input)

	// Assert
	assert.NoError(t, err)
	// We might get duplicates if search returns same chunk, but we deduplicate in code
	// Since we return same chunk for all 4 searches, we expect 1 unique context
	assert.Len(t, output.Contexts, 1)
	assert.Equal(t, "Found content", output.Contexts[0].ChunkText)
	assert.Equal(t, float32(0.95), output.Contexts[0].Score)
}

func msgContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
