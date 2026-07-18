package usecase_test

import (
	"context"
	"testing"

	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGeneralStrategy_Name(t *testing.T) {
	strategy := usecase.NewGeneralStrategy(nil)
	assert.Equal(t, "general", strategy.Name())
}

func TestGeneralStrategy_DelegatesToRetrieve(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	strategy := usecase.NewGeneralStrategy(mockRetrieve)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: chunkID, ChunkText: "general content", Score: 0.7, Title: "News"},
		},
	}, nil)

	out, err := strategy.Retrieve(context.Background(),
		usecase.RetrieveContextInput{Query: "latest news"},
		usecase.QueryIntent{IntentType: usecase.IntentGeneral, UserQuestion: "latest news"},
	)
	assert.NoError(t, err)
	assert.Len(t, out.Contexts, 1)
	mockRetrieve.AssertExpectations(t)
}
