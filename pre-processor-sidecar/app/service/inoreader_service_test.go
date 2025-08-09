package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/models"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestInoreaderService_RefreshTokenIfNeeded(t *testing.T) {
	tests := map[string]struct {
		currentToken       *models.OAuth2Token
		mockSetup          func(*mocks.MockInoreaderClient)
		expectError        bool
		expectTokenRefresh bool
	}{
		"token_not_expired_no_refresh": {
			currentToken: &models.OAuth2Token{
				AccessToken:  "valid_token",
				RefreshToken: "refresh_token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(10 * time.Minute),
				IssuedAt:     time.Now().Add(-50 * time.Minute),
			},
			mockSetup: func(client *mocks.MockInoreaderClient) {
				// No calls expected
			},
			expectError:        false,
			expectTokenRefresh: false,
		},
		"token_needs_refresh_success": {
			currentToken: &models.OAuth2Token{
				AccessToken:  "expired_token",
				RefreshToken: "refresh_token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(2 * time.Minute), // Within 5-minute buffer
				IssuedAt:     time.Now().Add(-58 * time.Minute),
			},
			mockSetup: func(client *mocks.MockInoreaderClient) {
				response := &models.InoreaderTokenResponse{
					AccessToken:  "new_access_token",
					TokenType:    "Bearer",
					ExpiresIn:    3600,
					RefreshToken: "new_refresh_token",
					Scope:        "read",
				}
				client.EXPECT().RefreshToken(gomock.Any(), "refresh_token").Return(response, nil)
			},
			expectError:        false,
			expectTokenRefresh: true,
		},
		"token_refresh_failure": {
			currentToken: &models.OAuth2Token{
				AccessToken:  "expired_token",
				RefreshToken: "invalid_refresh_token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(-5 * time.Minute), // Expired
				IssuedAt:     time.Now().Add(-65 * time.Minute),
			},
			mockSetup: func(client *mocks.MockInoreaderClient) {
				client.EXPECT().RefreshToken(gomock.Any(), "invalid_refresh_token").
					Return(nil, fmt.Errorf("OAuth2 refresh token failed with status 400"))
			},
			expectError:        true,
			expectTokenRefresh: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockInoreaderClient := mocks.NewMockInoreaderClient(ctrl)
			tc.mockSetup(mockInoreaderClient)

			service := NewInoreaderService(mockInoreaderClient, nil, nil)
			service.SetCurrentToken(tc.currentToken)

			ctx := context.Background()
			err := service.RefreshTokenIfNeeded(ctx)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tc.expectTokenRefresh {
					// Token should be updated
					assert.Equal(t, "new_access_token", service.currentToken.AccessToken)
					assert.Equal(t, "Bearer", service.currentToken.TokenType)
				}
			}
		})
	}
}

func TestInoreaderService_FetchSubscriptions(t *testing.T) {
	tests := map[string]struct {
		mockResponse     map[string]interface{}
		expectedCount    int
		expectError      bool
		rateLimitHeaders map[string]string
	}{
		"successful_subscription_fetch": {
			mockResponse: map[string]interface{}{
				"subscriptions": []interface{}{
					map[string]interface{}{
						"id":    "feed/http://example.com/rss",
						"title": "Example Tech News",
						"categories": []interface{}{
							map[string]interface{}{
								"id":    "user/12345/label/Tech",
								"label": "Tech",
							},
						},
						"url":     "http://example.com/rss",
						"htmlUrl": "http://example.com",
						"iconUrl": "http://example.com/favicon.ico",
					},
					map[string]interface{}{
						"id":    "feed/http://blog.example.org/feed",
						"title": "Development Blog",
						"categories": []interface{}{
							map[string]interface{}{
								"id":    "user/12345/label/Development",
								"label": "Development",
							},
						},
						"url":     "http://blog.example.org/feed",
						"htmlUrl": "http://blog.example.org",
					},
				},
			},
			expectedCount: 2,
			expectError:   false,
			rateLimitHeaders: map[string]string{
				"X-Reader-Zone1-Usage": "25",
				"X-Reader-Zone1-Limit": "100",
			},
		},
		"empty_subscriptions_list": {
			mockResponse: map[string]interface{}{
				"subscriptions": []interface{}{},
			},
			expectedCount: 0,
			expectError:   false,
		},
		"api_rate_limit_exceeded": {
			mockResponse:  nil,
			expectedCount: 0,
			expectError:   true,
			rateLimitHeaders: map[string]string{
				"X-Reader-Zone1-Usage": "100",
				"X-Reader-Zone1-Limit": "100",
			},
		},
		"unauthorized_token_expired": {
			mockResponse:  nil,
			expectedCount: 0,
			expectError:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockInoreaderClient := mocks.NewMockInoreaderClient(ctrl)

			// Setup expectations
			if tc.expectError && tc.rateLimitHeaders != nil {
				mockInoreaderClient.EXPECT().
					FetchSubscriptionList(gomock.Any(), "valid_token").
					Return(nil, fmt.Errorf("API rate limit exceeded (Zone 1: 100/100)"))
			} else if tc.expectError {
				mockInoreaderClient.EXPECT().
					FetchSubscriptionList(gomock.Any(), "valid_token").
					Return(nil, fmt.Errorf("authentication failed: token may be expired or invalid"))
			} else {
				mockInoreaderClient.EXPECT().
					FetchSubscriptionList(gomock.Any(), "valid_token").
					Return(tc.mockResponse, nil)
				if tc.expectedCount > 0 {
					mockInoreaderClient.EXPECT().
						ParseSubscriptionsResponse(tc.mockResponse).
						Return([]*models.Subscription{
							{InoreaderID: "feed/http://example.com/rss", FeedURL: "http://example.com/rss", Title: "Example Tech News", Category: "Tech"},
							{InoreaderID: "feed/http://blog.example.org/feed", FeedURL: "http://blog.example.org/feed", Title: "Development Blog", Category: "Development"},
						}, nil)
				} else {
					mockInoreaderClient.EXPECT().
						ParseSubscriptionsResponse(tc.mockResponse).
						Return([]*models.Subscription{}, nil)
				}
			}

			service := NewInoreaderService(mockInoreaderClient, nil, nil)
			service.SetCurrentToken(&models.OAuth2Token{
				AccessToken: "valid_token",
				TokenType:   "Bearer",
				ExpiresAt:   time.Now().Add(30 * time.Minute),
			})

			// Set up rate limit to allow requests
			service.rateLimitInfo = &models.APIRateLimitInfo{
				Zone1Usage:     25,
				Zone1Limit:     100,
				Zone1Remaining: 75,
			}

			ctx := context.Background()
			subscriptions, err := service.FetchSubscriptions(ctx)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, subscriptions)
			} else {
				assert.NoError(t, err)
				assert.Len(t, subscriptions, tc.expectedCount)

				if tc.expectedCount > 0 {
					// Verify first subscription structure
					sub := subscriptions[0]
					assert.Equal(t, "feed/http://example.com/rss", sub.InoreaderID)
					assert.Equal(t, "http://example.com/rss", sub.FeedURL)
					assert.Equal(t, "Example Tech News", sub.Title)
					assert.Equal(t, "Tech", sub.Category)
				}
			}
		})
	}
}

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
			service := NewInoreaderService(nil, nil, nil)
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

func TestInoreaderService_UpdateAPIUsageFromHeaders(t *testing.T) {
	tests := map[string]struct {
		endpoint      string
		headers       map[string]string
		existingUsage *models.APIUsageTracking
		mockSetup     func(*mocks.MockAPIUsageRepository)
		expectError   bool
		expectedZone1 int
		expectedZone2 int
	}{
		"new_usage_record_zone1_endpoint": {
			endpoint: "/subscription/list",
			headers: map[string]string{
				"X-Reader-Zone1-Usage":     "25",
				"X-Reader-Zone1-Limit":     "100",
				"X-Reader-Zone1-Remaining": "75",
			},
			existingUsage: nil,
			mockSetup: func(repo *mocks.MockAPIUsageRepository) {
				// No existing record found
				repo.EXPECT().
					GetTodaysUsage(gomock.Any()).
					Return(nil, fmt.Errorf("not found"))

				// Create new record
				repo.EXPECT().
					CreateUsageRecord(gomock.Any(), gomock.Any()).
					Return(nil)

				// Update record after increment
				repo.EXPECT().
					UpdateUsageRecord(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:   false,
			expectedZone1: 1,
			expectedZone2: 0,
		},
		"existing_usage_record_zone2_endpoint": {
			endpoint: "/subscription/edit",
			headers: map[string]string{
				"X-Reader-Zone2-Usage": "10",
				"X-Reader-Zone2-Limit": "100",
			},
			existingUsage: &models.APIUsageTracking{
				Zone1Requests: 5,
				Zone2Requests: 2,
				Date:          time.Now(),
			},
			mockSetup: func(repo *mocks.MockAPIUsageRepository) {
				// Return existing record
				repo.EXPECT().
					GetTodaysUsage(gomock.Any()).
					Return(&models.APIUsageTracking{
						Zone1Requests: 5,
						Zone2Requests: 2,
						Date:          time.Now(),
					}, nil)

				// Update record after increment
				repo.EXPECT().
					UpdateUsageRecord(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:   false,
			expectedZone1: 5,
			expectedZone2: 3,
		},
		"repository_not_configured": {
			endpoint: "/subscription/list",
			headers:  map[string]string{},
			mockSetup: func(repo *mocks.MockAPIUsageRepository) {
				// No expectations - repository is nil
			},
			expectError:   false,
			expectedZone1: 0,
			expectedZone2: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var mockRepo *mocks.MockAPIUsageRepository
			var service *InoreaderService

            if name == "repository_not_configured" {
                // Test with nil repository
                service = NewInoreaderService(nil, nil, nil)
            } else {
				mockRepo = mocks.NewMockAPIUsageRepository(ctrl)
				tc.mockSetup(mockRepo)
				service = NewInoreaderService(nil, mockRepo, nil)
			}

			ctx := context.Background()
			// Mock client call to fetch headers when repo configured
            if mockRepo != nil {
				mockClient := mocks.NewMockInoreaderClient(ctrl)
				service.inoreaderClient = mockClient
				service.SetCurrentToken(&models.OAuth2Token{AccessToken: "valid_token", TokenType: "Bearer", ExpiresAt: time.Now().Add(1 * time.Hour)})
				mockClient.EXPECT().
					MakeAuthenticatedRequestWithHeaders(gomock.Any(), "valid_token", tc.endpoint, gomock.Nil()).
					Return(nil, tc.headers, nil)
            } else {
                // Even with nil repo, the method will still try to fetch headers; provide a client and token
                mockClient := mocks.NewMockInoreaderClient(ctrl)
                service.inoreaderClient = mockClient
                service.SetCurrentToken(&models.OAuth2Token{AccessToken: "valid_token", TokenType: "Bearer", ExpiresAt: time.Now().Add(1 * time.Hour)})
                mockClient.EXPECT().
                    MakeAuthenticatedRequestWithHeaders(gomock.Any(), "valid_token", tc.endpoint, gomock.Nil()).
                    Return(nil, tc.headers, nil)
			}
			err := service.UpdateAPIUsageFromHeaders(ctx, tc.endpoint)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
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
				service = NewInoreaderService(nil, nil, nil)
				service.rateLimitInfo.Zone1Usage = 25
			} else {
				mockRepo := mocks.NewMockAPIUsageRepository(ctrl)
				tc.mockSetup(mockRepo)
				service = NewInoreaderService(nil, mockRepo, nil)
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
			svc := NewInoreaderService(nil, nil, nil)
			result := svc.isReadOnlyEndpoint(tc.endpoint)
			assert.Equal(t, tc.expected, result)
		})
	}
}
