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

func TestFactCheckStrategy_Name(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewFactCheckStrategy(nil, logger)
	assert.Equal(t, "fact_check", strategy.Name())
}

func TestFactCheckStrategy_DelegatesToRetrieve(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewFactCheckStrategy(mockRetrieve, logger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "evidence", Score: 0.85, Title: "Claim"},
		},
	}, nil)

	out, err := strategy.Retrieve(context.Background(),
		usecase.RetrieveContextInput{Query: "本当に？"},
		usecase.QueryIntent{IntentType: usecase.IntentFactCheck, UserQuestion: "本当に？"},
	)
	assert.NoError(t, err)
	assert.Len(t, out.Contexts, 1)
	mockRetrieve.AssertExpectations(t)
}
