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

func TestTopicDeepDiveStrategy_Name(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewTopicDeepDiveStrategy(nil, logger)
	assert.Equal(t, "topic_deep_dive", strategy.Name())
}

func TestTopicDeepDiveStrategy_DelegatesToRetrieve(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	strategy := usecase.NewTopicDeepDiveStrategy(mockRetrieve, logger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "deep dive content", Score: 0.9, Title: "Topic"},
		},
	}, nil)

	out, err := strategy.Retrieve(context.Background(),
		usecase.RetrieveContextInput{Query: "詳しく教えて"},
		usecase.QueryIntent{IntentType: usecase.IntentTopicDeepDive, UserQuestion: "詳しく教えて"},
	)
	assert.NoError(t, err)
	assert.Len(t, out.Contexts, 1)
	mockRetrieve.AssertExpectations(t)
}
