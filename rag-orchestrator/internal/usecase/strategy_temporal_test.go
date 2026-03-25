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

func TestTemporalStrategy_AppliesStrongerBoost(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewTemporalStrategy(mockRetrieve, logger)

	assert.Equal(t, "temporal", strategy.Name())

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "Recent article", Score: 0.5, Title: "T", URL: "http://x.com", PublishedAt: "2025-01-01T00:00:00Z", DocumentVersion: 1},
		},
	}, nil)

	ctx := context.Background()
	input := usecase.RetrieveContextInput{Query: "最近のAIニュース"}
	intent := usecase.QueryIntent{IntentType: usecase.IntentTemporal, UserQuestion: "最近のAIニュース"}

	output, err := strategy.Retrieve(ctx, input, intent)
	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Len(t, output.Contexts, 1)
}

func TestTemporalStrategy_Name(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewTemporalStrategy(nil, logger)
	assert.Equal(t, "temporal", strategy.Name())
}
