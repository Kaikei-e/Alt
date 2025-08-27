package service

import (
	"context"
	"fmt"
	"testing"

	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/models"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)



func TestInoreaderService_CheckAPIRateLimit(t *testing.T) {
	tests := map[string]struct {
		currentUsage    int
		dailyLimit      int
		safetyBuffer    int
		expectAllowed   bool
		expectRemaining int
	}{
		"well_within_limits": {
			currentUsage:    25,
			dailyLimit:      100,
			safetyBuffer:    10,
			expectAllowed:   true,
			expectRemaining: 65, // 100 - 25 - 10
		},
		"approaching_limit": {
			currentUsage:    85,
			dailyLimit:      100,
			safetyBuffer:    10,
			expectAllowed:   true,
			expectRemaining: 5, // 100 - 85 - 10
		},
		"exceeded_safe_limit": {
			currentUsage:    92,
			dailyLimit:      100,
			safetyBuffer:    10,
			expectAllowed:   false,
			expectRemaining: 0, // 100 - 92 - 10 = -2 -> 0
		},
		"at_absolute_limit": {
			currentUsage:    100,
			dailyLimit:      100,
			safetyBuffer:    10,
			expectAllowed:   false,
			expectRemaining: 0,
		},
		"over_absolute_limit": {
			currentUsage:    105,
			dailyLimit:      100,
			safetyBuffer:    10,
			expectAllowed:   false,
			expectRemaining: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewInoreaderService(nil, nil, nil, nil)
			service.apiDailyLimit = tc.dailyLimit
			service.safetyBuffer = tc.safetyBuffer
			service.rateLimitInfo = &models.APIRateLimitInfo{
				Zone1Usage:     tc.currentUsage,
				Zone1Limit:     tc.dailyLimit,
				Zone1Remaining: tc.dailyLimit - tc.currentUsage,
			}

			allowed, remaining := service.CheckAPIRateLimit()

			assert.Equal(t, tc.expectAllowed, allowed)
			assert.Equal(t, tc.expectRemaining, remaining)
		})
	}
}


func TestInoreaderService_GetCurrentAPIUsageInfo(t *testing.T) {
	tests := map[string]struct {
		usageRecord   *models.APIUsageTracking
		mockSetup     func(*mocks.MockAPIUsageRepository)
		expectError   bool
		expectedUsage int
	}{
		"with_usage_record": {
			usageRecord: &models.APIUsageTracking{
				Zone1Requests: 45,
				Zone2Requests: 10,
			},
			mockSetup: func(repo *mocks.MockAPIUsageRepository) {
				repo.EXPECT().
					GetTodaysUsage(gomock.Any()).
					Return(&models.APIUsageTracking{
						Zone1Requests: 45,
						Zone2Requests: 10,
					}, nil)
			},
			expectError:   false,
			expectedUsage: 45,
		},
		"no_usage_record": {
			usageRecord: nil,
			mockSetup: func(repo *mocks.MockAPIUsageRepository) {
				repo.EXPECT().
					GetTodaysUsage(gomock.Any()).
					Return(nil, fmt.Errorf("not found"))
			},
			expectError:   false,
			expectedUsage: 0,
		},
		"repository_not_configured": {
			mockSetup: func(repo *mocks.MockAPIUsageRepository) {
				// No expectations - repository is nil
			},
			expectError:   false,
			expectedUsage: 25, // From rate limit info
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var service *InoreaderService

			if name == "repository_not_configured" {
				service = NewInoreaderService(nil, nil, nil, nil)
				service.rateLimitInfo.Zone1Usage = 25
			} else {
				mockRepo := mocks.NewMockAPIUsageRepository(ctrl)
				tc.mockSetup(mockRepo)
				service = NewInoreaderService(nil, mockRepo, nil, nil)
			}

			ctx := context.Background()
			info, err := service.GetCurrentAPIUsageInfo(ctx)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, tc.expectedUsage, info.Zone1Requests)
			}
		})
	}
}

func TestInoreaderService_isReadOnlyEndpoint(t *testing.T) {
	tests := map[string]struct {
		endpoint string
		expected bool
	}{
		"subscription_list":     {"/subscription/list", true},
		"stream_contents":       {"/stream/contents/user/-/state/com.google/reading-list", true},
		"stream_items":          {"/stream/items/contents", true},
		"user_info":             {"/user-info", true},
		"subscription_edit":     {"/subscription/edit", false},
		"subscription_quickadd": {"/subscription/quickadd", false},
		"unknown_endpoint":      {"/unknown/endpoint", false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc := NewInoreaderService(nil, nil, nil, nil)
			result := svc.isReadOnlyEndpoint(tc.endpoint)
			assert.Equal(t, tc.expected, result)
		})
	}
}
