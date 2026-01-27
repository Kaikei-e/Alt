package service

import (
	"context"
	"fmt"

	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/models"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

// Use the generated mocks from mocks package

// MockTokenService implements a simple token service for testing
type MockTokenService struct {
	token string
	valid bool
}

func (m *MockTokenService) GetCurrentToken(ctx context.Context) (string, error) {
	if m.token == "" {
		return "", fmt.Errorf("no token available")
	}
	return m.token, nil
}

func (m *MockTokenService) IsTokenValid(ctx context.Context) (bool, error) {
	return m.valid, nil
}

func (m *MockTokenService) RefreshToken(ctx context.Context) error {
	return nil // Mock refresh always succeeds
}

// setupSubscriptionMock sets up a standard subscription mock for testing
func setupSubscriptionMock(subscriptionRepo *mocks.MockSubscriptionRepository) {
	subscriptionRepo.EXPECT().
		GetAllSubscriptions(gomock.Any()).
		Return([]models.InoreaderSubscription{
			{
				DatabaseID:  uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
				InoreaderID: "feed/http://example.com/rss",
				URL:         "http://example.com/rss",
				Title:       "Example Feed",
			},
		}, nil).AnyTimes()
}

// Tests use proper gomock generated mocks
