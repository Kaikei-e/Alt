package fetch_article_tags_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	logger.InitLogger()
}

// MockFetchArticleTagsPort is a mock implementation for testing
type MockFetchArticleTagsPort struct {
	mock.Mock
}

func (m *MockFetchArticleTagsPort) FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	args := m.Called(ctx, articleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.FeedTag), args.Error(1)
}

func TestFetchArticleTagsUsecase_Execute_Success(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticleTagsPort)
	usecase := NewFetchArticleTagsUsecase(mockPort)

	articleID := "test-article-id"
	now := time.Now()
	expectedTags := []*domain.FeedTag{
		{
			ID:        "tag-1",
			TagName:   "Technology",
			CreatedAt: now,
		},
		{
			ID:        "tag-2",
			TagName:   "Programming",
			CreatedAt: now,
		},
	}

	mockPort.On("FetchArticleTags", mock.Anything, articleID).Return(expectedTags, nil)

	// Act
	result, err := usecase.Execute(context.Background(), articleID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)
	assert.Equal(t, "Technology", result[0].TagName)
	assert.Equal(t, "Programming", result[1].TagName)
	mockPort.AssertExpectations(t)
}

func TestFetchArticleTagsUsecase_Execute_EmptyArticleID(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticleTagsPort)
	usecase := NewFetchArticleTagsUsecase(mockPort)

	// Act
	result, err := usecase.Execute(context.Background(), "")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "article_id must not be empty")
}

func TestFetchArticleTagsUsecase_Execute_EmptyResult(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticleTagsPort)
	usecase := NewFetchArticleTagsUsecase(mockPort)

	articleID := "test-article-id"
	mockPort.On("FetchArticleTags", mock.Anything, articleID).Return([]*domain.FeedTag{}, nil)

	// Act
	result, err := usecase.Execute(context.Background(), articleID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 0)
	mockPort.AssertExpectations(t)
}

func TestFetchArticleTagsUsecase_Execute_DatabaseError(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchArticleTagsPort)
	usecase := NewFetchArticleTagsUsecase(mockPort)

	articleID := "test-article-id"
	dbError := errors.New("database connection failed")
	mockPort.On("FetchArticleTags", mock.Anything, articleID).Return(nil, dbError)

	// Act
	result, err := usecase.Execute(context.Background(), articleID)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database connection failed")
	mockPort.AssertExpectations(t)
}
