package usecase_test

import (
	"context"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
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

	uc := usecase.NewRetrieveContextUsecase(mockChunkRepo, mockDocRepo, mockEncoder)

	ctx := context.Background()
	input := usecase.RetrieveContextInput{
		Query: "search query",
	}

	// Expectations
	// 1. Encode
	queryVec := []float32{0.1, 0.2, 0.3}
	mockEncoder.On("Encode", ctx, []string{"search query"}).Return([][]float32{queryVec}, nil)

	// 2. Search
	mockChunkRepo.On("Search", ctx, queryVec, []string(nil), 5).Return([]domain.SearchResult{
		{
			Chunk: domain.RagChunk{
				ID:      uuid.New(),
				Content: "Found content",
			},
			Score:           0.95,
			ArticleID:       "art-1",
			DocumentVersion: 1,
		},
	}, nil)

	// Execute
	output, err := uc.Execute(ctx, input)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, output.Contexts, 1)
	assert.Equal(t, "Found content", output.Contexts[0].ChunkText)
	assert.Equal(t, float32(0.95), output.Contexts[0].Score)
}
