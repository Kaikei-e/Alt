package fetch_articles_by_tag_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	logger.InitLogger()
}

// MockFetchArticlesByTagPort is a mock implementation for testing
type MockFetchArticlesByTagPort struct {
	mock.Mock
}

func (m *MockFetchArticlesByTagPort) FetchArticlesByTag(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	args := m.Called(ctx, tagID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.TagTrailArticle), args.Error(1)
}

func (m *MockFetchArticlesByTagPort) FetchArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	args := m.Called(ctx, tagName, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.TagTrailArticle), args.Error(1)
}

func TestFetchArticlesByTagUsecase_Execute_Success(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagID := uuid.New().String()
	now := time.Now()
	expectedArticles := []*domain.TagTrailArticle{
		{
			ID:          uuid.New().String(),
			Title:       "Test Article 1",
			Link:        "https://example.com/article1",
			PublishedAt: now,
			FeedTitle:   "Test Feed",
		},
		{
			ID:          uuid.New().String(),
			Title:       "Test Article 2",
			Link:        "https://example.com/article2",
			PublishedAt: now.Add(-1 * time.Hour),
			FeedTitle:   "Test Feed",
		},
	}

	mockPort.On("FetchArticlesByTag", mock.Anything, tagID, (*time.Time)(nil), 20).Return(expectedArticles, nil)

	// Act
	result, err := usecase.Execute(context.Background(), tagID, nil, 20)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)
	assert.Equal(t, expectedArticles[0].Title, result[0].Title)
	mockPort.AssertExpectations(t)
}

func TestFetchArticlesByTagUsecase_Execute_WithCursor(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagID := uuid.New().String()
	cursor := time.Now().Add(-2 * time.Hour)
	expectedArticles := []*domain.TagTrailArticle{
		{
			ID:          uuid.New().String(),
			Title:       "Older Article",
			Link:        "https://example.com/older",
			PublishedAt: cursor.Add(-1 * time.Hour),
			FeedTitle:   "Test Feed",
		},
	}

	mockPort.On("FetchArticlesByTag", mock.Anything, tagID, &cursor, 10).Return(expectedArticles, nil)

	// Act
	result, err := usecase.Execute(context.Background(), tagID, &cursor, 10)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 1)
	mockPort.AssertExpectations(t)
}

func TestFetchArticlesByTagUsecase_Execute_EmptyTagID(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	// Act
	result, err := usecase.Execute(context.Background(), "", nil, 20)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tag_id must not be empty")
}

func TestFetchArticlesByTagUsecase_Execute_InvalidLimit(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagID := uuid.New().String()

	tests := []struct {
		name  string
		limit int
	}{
		{"zero limit", 0},
		{"negative limit", -1},
		{"too large limit", 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := usecase.Execute(context.Background(), tagID, nil, tt.limit)
			assert.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestFetchArticlesByTagUsecase_Execute_DatabaseError(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagID := uuid.New().String()
	dbError := errors.New("database connection failed")
	mockPort.On("FetchArticlesByTag", mock.Anything, tagID, (*time.Time)(nil), 20).Return(nil, dbError)

	// Act
	result, err := usecase.Execute(context.Background(), tagID, nil, 20)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database connection failed")
	mockPort.AssertExpectations(t)
}

func TestFetchArticlesByTagUsecase_Execute_EmptyResult(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagID := uuid.New().String()
	mockPort.On("FetchArticlesByTag", mock.Anything, tagID, (*time.Time)(nil), 20).Return([]*domain.TagTrailArticle{}, nil)

	// Act
	result, err := usecase.Execute(context.Background(), tagID, nil, 20)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 0)
	mockPort.AssertExpectations(t)
}

func TestFetchArticlesByTagUsecase_ExecuteByTagName_Success(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagName := "golang"
	now := time.Now()
	expectedArticles := []*domain.TagTrailArticle{
		{
			ID:          uuid.New().String(),
			Title:       "Go Article 1",
			Link:        "https://example.com/go1",
			PublishedAt: now,
			FeedTitle:   "Go Blog",
		},
		{
			ID:          uuid.New().String(),
			Title:       "Go Article 2",
			Link:        "https://other.com/go2",
			PublishedAt: now.Add(-1 * time.Hour),
			FeedTitle:   "Another Go Blog",
		},
	}

	mockPort.On("FetchArticlesByTagName", mock.Anything, tagName, (*time.Time)(nil), 20).Return(expectedArticles, nil)

	// Act
	result, err := usecase.ExecuteByTagName(context.Background(), tagName, nil, 20)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)
	assert.Equal(t, expectedArticles[0].Title, result[0].Title)
	mockPort.AssertExpectations(t)
}

func TestFetchArticlesByTagUsecase_ExecuteByTagName_EmptyTagName(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	// Act
	result, err := usecase.ExecuteByTagName(context.Background(), "", nil, 20)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tag_name must not be empty")
}

func TestFetchArticlesByTagUsecase_ExecuteByTagName_InvalidLimit(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagName := "golang"

	tests := []struct {
		name  string
		limit int
	}{
		{"zero limit", 0},
		{"negative limit", -1},
		{"too large limit", 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := usecase.ExecuteByTagName(context.Background(), tagName, nil, tt.limit)
			assert.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestFetchArticlesByTagUsecase_ExecuteByTagName_WithCursor(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticlesByTagPort)
	usecase := NewFetchArticlesByTagUsecase(mockPort)

	tagName := "python"
	cursor := time.Now().Add(-2 * time.Hour)
	expectedArticles := []*domain.TagTrailArticle{
		{
			ID:          uuid.New().String(),
			Title:       "Older Python Article",
			Link:        "https://example.com/python",
			PublishedAt: cursor.Add(-1 * time.Hour),
			FeedTitle:   "Python Blog",
		},
	}

	mockPort.On("FetchArticlesByTagName", mock.Anything, tagName, &cursor, 10).Return(expectedArticles, nil)

	// Act
	result, err := usecase.ExecuteByTagName(context.Background(), tagName, &cursor, 10)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 1)
	mockPort.AssertExpectations(t)
}
