package retrieve_context_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/rag_integration_port"
	"context"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock mocks
type MockSearchFeedPort struct {
	mock.Mock
}

func (m *MockSearchFeedPort) SearchFeeds(ctx context.Context, query string) ([]domain.SearchArticleHit, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchArticleHit), args.Error(1)
}

func (m *MockSearchFeedPort) SearchFeedsWithPagination(ctx context.Context, query string, offset, limit int) ([]domain.SearchArticleHit, int, error) {
	args := m.Called(ctx, query, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int), args.Error(2)
	}
	return args.Get(0).([]domain.SearchArticleHit), args.Get(1).(int), args.Error(2)
}

type MockRagIntegrationPort struct {
	mock.Mock
}

func (m *MockRagIntegrationPort) RetrieveContext(ctx context.Context, query string, candidateIDs []string, userID string) ([]rag_integration_port.RagContext, error) {
	args := m.Called(ctx, query, candidateIDs, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]rag_integration_port.RagContext), args.Error(1)
}

func (m *MockRagIntegrationPort) UpsertArticle(ctx context.Context, input rag_integration_port.UpsertArticleInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *MockRagIntegrationPort) Answer(ctx context.Context, input rag_integration_port.AnswerInput) (<-chan string, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan string), args.Error(1)
}

func TestRetrieveContextUsecase_Execute(t *testing.T) {
	mockSearch := new(MockSearchFeedPort)
	mockRag := new(MockRagIntegrationPort)
	usecase := NewRetrieveContextUsecase(mockSearch, mockRag)

	ctx := context.Background()
	query := "test query"
	userID := "user-123"

	// Mock Meilisearch response
	mockSearch.On("SearchFeedsWithPagination", ctx, query, 0, 50).Return([]domain.SearchArticleHit{
		{ID: "article-1"},
		{ID: "article-2"},
	}, 2, nil)

	// Mock RAG response
	expectedContexts := []rag_integration_port.RagContext{
		{ChunkText: "text1", Score: 0.9},
	}
	mockRag.On("RetrieveContext", ctx, query, []string{"article-1", "article-2"}, userID).Return(expectedContexts, nil)

	// Execute
	contexts, err := usecase.Execute(ctx, query, userID)

	// Verify
	assert.NoError(t, err)
	assert.Len(t, contexts, 1)
	assert.Equal(t, "text1", contexts[0].ChunkText)

	mockSearch.AssertExpectations(t)
	mockRag.AssertExpectations(t)
}

func TestRetrieveContextUsecase_Execute_RejectsBlankUserID(t *testing.T) {
	mockSearch := new(MockSearchFeedPort)
	mockRag := new(MockRagIntegrationPort)
	usecase := NewRetrieveContextUsecase(mockSearch, mockRag)

	ctx := context.Background()
	query := "test query"

	testCases := []struct {
		name   string
		userID string
	}{
		{name: "empty user_id", userID: ""},
		{name: "whitespace user_id", userID: "   \t\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			contexts, err := usecase.Execute(ctx, query, tc.userID)
			assert.Error(t, err)
			assert.Nil(t, contexts)
			assert.ErrorIs(t, err, rag_integration_port.ErrMissingUserID)
		})
	}
}
