package search_article_usecase

import (
	"alt/domain"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSearchIndexerPort is a mock implementation of SearchIndexerPort
type MockSearchIndexerPort struct {
	mock.Mock
}

func (m *MockSearchIndexerPort) SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error) {
	args := m.Called(ctx, query, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchIndexerArticleHit), args.Error(1)
}

func (m *MockSearchIndexerPort) SearchArticlesWithPagination(ctx context.Context, query string, userID string, offset int, limit int) ([]domain.SearchIndexerArticleHit, int64, error) {
	args := m.Called(ctx, query, userID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]domain.SearchIndexerArticleHit), args.Get(1).(int64), args.Error(2)
}

func TestSearchArticleUsecase_Execute_Success(t *testing.T) {
	// Arrange
	mockPort := new(MockSearchIndexerPort)
	usecase := NewSearchArticleUsecase(mockPort)

	userID := uuid.New()
	user := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := context.WithValue(context.Background(), domain.UserContextKey, user)

	query := "test query"
	expectedHits := []domain.SearchIndexerArticleHit{
		{
			ID:      "1",
			Title:   "Test Article 1",
			Content: "This is test content 1",
			Tags:    []string{"tag1", "tag2"},
		},
		{
			ID:      "2",
			Title:   "Test Article 2",
			Content: "This is test content 2",
			Tags:    []string{"tag3"},
		},
	}

	mockPort.On("SearchArticles", ctx, query, userID.String()).Return(expectedHits, nil)

	// Act
	results, err := usecase.Execute(ctx, query)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedHits, results)
	assert.Len(t, results, 2)
	mockPort.AssertExpectations(t)
}

func TestSearchArticleUsecase_Execute_NoUserInContext(t *testing.T) {
	// Arrange
	mockPort := new(MockSearchIndexerPort)
	usecase := NewSearchArticleUsecase(mockPort)

	ctx := context.Background() // No user in context
	query := "test query"

	// Act
	results, err := usecase.Execute(ctx, query)

	// Assert
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "authentication required")
	mockPort.AssertNotCalled(t, "SearchArticles")
}

func TestSearchArticleUsecase_Execute_SearchIndexerError(t *testing.T) {
	// Arrange
	mockPort := new(MockSearchIndexerPort)
	usecase := NewSearchArticleUsecase(mockPort)

	userID := uuid.New()
	user := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := context.WithValue(context.Background(), domain.UserContextKey, user)

	query := "test query"
	expectedError := errors.New("search indexer connection failed")

	mockPort.On("SearchArticles", ctx, query, userID.String()).Return(nil, expectedError)

	// Act
	results, err := usecase.Execute(ctx, query)

	// Assert
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "article search failed")
	mockPort.AssertExpectations(t)
}

func TestSearchArticleUsecase_Execute_EmptyResults(t *testing.T) {
	// Arrange
	mockPort := new(MockSearchIndexerPort)
	usecase := NewSearchArticleUsecase(mockPort)

	userID := uuid.New()
	user := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := context.WithValue(context.Background(), domain.UserContextKey, user)

	query := "nonexistent query"
	emptyHits := []domain.SearchIndexerArticleHit{}

	mockPort.On("SearchArticles", ctx, query, userID.String()).Return(emptyHits, nil)

	// Act
	results, err := usecase.Execute(ctx, query)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, results)
	mockPort.AssertExpectations(t)
}
