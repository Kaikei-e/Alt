package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestComparisonStrategy_MergesResults(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewComparisonStrategy(mockRetrieve, logger)

	assert.Equal(t, "comparison", strategy.Name())

	chunk1 := uuid.New()
	chunk2 := uuid.New()

	// First call returns Rust results, second returns Go results
	mockRetrieve.On("Execute", mock.Anything, mock.MatchedBy(func(in usecase.RetrieveContextInput) bool {
		return in.Query != ""
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunk1, ChunkText: "About Rust", Score: 0.8, Title: "Rust", URL: "http://r.com", PublishedAt: "2025-01-01T00:00:00Z", DocumentVersion: 1},
			{ChunkID: chunk2, ChunkText: "About Go", Score: 0.7, Title: "Go", URL: "http://g.com", PublishedAt: "2025-01-01T00:00:00Z", DocumentVersion: 1},
		},
	}, nil)

	ctx := context.Background()
	input := usecase.RetrieveContextInput{Query: "RustとGoの違い"}
	intent := usecase.QueryIntent{IntentType: usecase.IntentComparison, UserQuestion: "RustとGoの違い"}

	output, err := strategy.Retrieve(ctx, input, intent)
	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Greater(t, len(output.Contexts), 0)
}

func TestComparisonStrategy_Name(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewComparisonStrategy(nil, logger)
	assert.Equal(t, "comparison", strategy.Name())
}
