package fetch_random_subscription_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	logger.InitLogger()
}

// MockFetchRandomSubscriptionPort is a mock implementation for testing
type MockFetchRandomSubscriptionPort struct {
	mock.Mock
}

func (m *MockFetchRandomSubscriptionPort) FetchRandomSubscription(ctx context.Context) (*domain.Feed, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Feed), args.Error(1)
}

func TestFetchRandomSubscriptionUsecase_Execute_Success(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchRandomSubscriptionPort)
	usecase := NewFetchRandomSubscriptionUsecase(mockPort)

	expectedFeed := &domain.Feed{
		ID:          uuid.New(),
		Title:       "Test Feed",
		Description: "A test feed description",
		Link:        "https://example.com",
	}

	mockPort.On("FetchRandomSubscription", mock.Anything).Return(expectedFeed, nil)

	// Act
	result, err := usecase.Execute(context.Background())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedFeed.ID, result.ID)
	assert.Equal(t, expectedFeed.Title, result.Title)
	assert.Equal(t, expectedFeed.Description, result.Description)
	assert.Equal(t, expectedFeed.Link, result.Link)
	mockPort.AssertExpectations(t)
}

func TestFetchRandomSubscriptionUsecase_Execute_NoFeeds(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchRandomSubscriptionPort)
	usecase := NewFetchRandomSubscriptionUsecase(mockPort)

	mockPort.On("FetchRandomSubscription", mock.Anything).Return(nil, ErrNoSubscriptions)

	// Act
	result, err := usecase.Execute(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, ErrNoSubscriptions))
	mockPort.AssertExpectations(t)
}

func TestFetchRandomSubscriptionUsecase_Execute_DatabaseError(t *testing.T) {
	// Arrange
	mockPort := new(MockFetchRandomSubscriptionPort)
	usecase := NewFetchRandomSubscriptionUsecase(mockPort)

	dbError := errors.New("database connection failed")
	mockPort.On("FetchRandomSubscription", mock.Anything).Return(nil, dbError)

	// Act
	result, err := usecase.Execute(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database connection failed")
	mockPort.AssertExpectations(t)
}
