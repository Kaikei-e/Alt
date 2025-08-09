package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/mocks"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// Use the generated mock from mocks package

func TestSubscriptionSyncService_SyncSubscriptions(t *testing.T) {
	tests := map[string]struct {
		inoreaderResponse    []*models.Subscription
		existingSubscriptions []*models.Subscription
		mockSetup            func(*mocks.MockOAuth2Driver, *mocks.MockSubscriptionRepository)
		expectError          bool
		expectedCreated      int
		expectedUpdated      int
		expectedDeleted      int
	}{
		"successful_sync_new_subscriptions": {
			inoreaderResponse: []*models.Subscription{
				{
					InoreaderID: "feed/http://example.com/rss",
					FeedURL:     "http://example.com/rss",
					Title:       "Example Tech News",
					Category:    "Tech",
				},
				{
					InoreaderID: "feed/http://blog.example.org/feed",
					FeedURL:     "http://blog.example.org/feed", 
					Title:       "Development Blog",
					Category:    "Development",
				},
			},
			existingSubscriptions: []*models.Subscription{},
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, repo *mocks.MockSubscriptionRepository) {
				// Mock Inoreader API call
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(gomock.Any(), gomock.Any(), "/subscription/list", nil).
					Return(map[string]interface{}{
						"subscriptions": []interface{}{
							map[string]interface{}{
								"id":    "feed/http://example.com/rss",
								"url":   "http://example.com/rss",
								"title": "Example Tech News",
								"categories": []interface{}{
									map[string]interface{}{
										"id":    "user/12345/label/Tech",
										"label": "Tech",
									},
								},
							},
							map[string]interface{}{
								"id":    "feed/http://blog.example.org/feed",
								"url":   "http://blog.example.org/feed",
								"title": "Development Blog",
								"categories": []interface{}{
									map[string]interface{}{
										"id":    "user/12345/label/Development",
										"label": "Development",
									},
								},
							},
						},
					}, nil)

				// Mock repository calls for new subscriptions
				repo.EXPECT().
					FindByInoreaderID(gomock.Any(), "feed/http://example.com/rss").
					Return(nil, fmt.Errorf("not found"))
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
				
				repo.EXPECT().
					FindByInoreaderID(gomock.Any(), "feed/http://blog.example.org/feed").
					Return(nil, fmt.Errorf("not found"))
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:     false,
			expectedCreated: 2,
			expectedUpdated: 0,
			expectedDeleted: 0,
		},
		"sync_with_updates": {
			inoreaderResponse: []*models.Subscription{
				{
					InoreaderID: "feed/http://example.com/rss",
					FeedURL:     "http://example.com/rss",
					Title:       "Updated Tech News", // Title changed
					Category:    "Technology",        // Category changed
				},
			},
			existingSubscriptions: []*models.Subscription{
				{
					InoreaderID: "feed/http://example.com/rss",
					FeedURL:     "http://example.com/rss",
					Title:       "Example Tech News",
					Category:    "Tech",
				},
			},
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, repo *mocks.MockSubscriptionRepository) {
				// Mock Inoreader API call
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(gomock.Any(), gomock.Any(), "/subscription/list", nil).
					Return(map[string]interface{}{
						"subscriptions": []interface{}{
							map[string]interface{}{
								"id":    "feed/http://example.com/rss",
								"url":   "http://example.com/rss",
								"title": "Updated Tech News",
								"categories": []interface{}{
									map[string]interface{}{
										"id":    "user/12345/label/Technology",
										"label": "Technology",
									},
								},
							},
						},
					}, nil)

				// Mock repository calls for existing subscription
				existingSub := &models.Subscription{
					InoreaderID: "feed/http://example.com/rss",
					FeedURL:     "http://example.com/rss",
					Title:       "Example Tech News",
					Category:    "Tech",
				}
				repo.EXPECT().
					FindByInoreaderID(gomock.Any(), "feed/http://example.com/rss").
					Return(existingSub, nil)
				repo.EXPECT().
					Update(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:     false,
			expectedCreated: 0,
			expectedUpdated: 1,
			expectedDeleted: 0,
		},
		"api_rate_limit_error": {
			inoreaderResponse: []*models.Subscription{},
			existingSubscriptions: []*models.Subscription{},
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, repo *mocks.MockSubscriptionRepository) {
				// Mock rate limit exceeded
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(gomock.Any(), gomock.Any(), "/subscription/list", nil).
					Return(nil, fmt.Errorf("API rate limit exceeded (Zone 1: 100/100)"))
			},
			expectError:     true,
			expectedCreated: 0,
			expectedUpdated: 0,
			expectedDeleted: 0,
		},
		"oauth2_token_expired": {
			inoreaderResponse: []*models.Subscription{},
			existingSubscriptions: []*models.Subscription{},
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, repo *mocks.MockSubscriptionRepository) {
				// Mock token expired error
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(gomock.Any(), gomock.Any(), "/subscription/list", nil).
					Return(nil, fmt.Errorf("authentication failed: token may be expired or invalid"))
			},
			expectError:     true,
			expectedCreated: 0,
			expectedUpdated: 0,
			expectedDeleted: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOAuth2Client := mocks.NewMockOAuth2Driver(ctrl)
			mockRepo := mocks.NewMockSubscriptionRepository(ctrl)

			tc.mockSetup(mockOAuth2Client, mockRepo)

			// Create subscription sync service
			inoreaderService := NewInoreaderService(mockOAuth2Client, nil, nil)
			inoreaderService.SetCurrentToken(&models.OAuth2Token{
				AccessToken: "valid_token",
				TokenType:   "Bearer",
				ExpiresAt:   time.Now().Add(30 * time.Minute),
			})

			// Set rate limit to allow requests
			inoreaderService.rateLimitInfo = &models.APIRateLimitInfo{
				Zone1Usage:     25,
				Zone1Limit:     100,
				Zone1Remaining: 75,
			}

			syncService := NewSubscriptionSyncService(inoreaderService, mockRepo, nil)

			ctx := context.Background()
			result, err := syncService.SyncSubscriptions(ctx)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.expectedCreated, result.Created)
				assert.Equal(t, tc.expectedUpdated, result.Updated)
				assert.Equal(t, tc.expectedDeleted, result.Deleted)
			}
		})
	}
}

func TestSubscriptionSyncService_IsSubscriptionChanged(t *testing.T) {
	tests := map[string]struct {
		existing *models.Subscription
		incoming *models.Subscription
		expected bool
	}{
		"no_changes": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			expected: false,
		},
		"title_changed": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Updated Tech News",
				Category:    "Technology",
			},
			expected: true,
		},
		"category_changed": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Development",
			},
			expected: true,
		},
		"url_changed": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/feed.xml",
				Title:       "Tech News", 
				Category:    "Technology",
			},
			expected: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			syncService := &SubscriptionSyncService{}

			result := syncService.IsSubscriptionChanged(tc.existing, tc.incoming)

			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSubscriptionSyncService_GetSyncStats(t *testing.T) {
	syncService := NewSubscriptionSyncService(nil, nil, nil)

	// Initialize sync stats
	syncService.lastSyncTime = time.Now().Add(-30 * time.Minute)
	syncService.syncStats = &SubscriptionSyncStats{
		LastSyncTime:     syncService.lastSyncTime,
		TotalSyncs:       5,
		SuccessfulSyncs:  4,
		FailedSyncs:     1,
		Created:         10,
		Updated:         3,
		Deleted:         2,
		LastError:       "Test error",
	}

	stats := syncService.GetSyncStats()

	assert.Equal(t, int64(5), stats.TotalSyncs)
	assert.Equal(t, int64(4), stats.SuccessfulSyncs)
	assert.Equal(t, int64(1), stats.FailedSyncs)
	assert.Equal(t, 10, stats.Created)
	assert.Equal(t, 3, stats.Updated)
	assert.Equal(t, 2, stats.Deleted)
	assert.Equal(t, "Test error", stats.LastError)
	assert.Equal(t, syncService.lastSyncTime, stats.LastSyncTime)
}

// Tests use proper gomock generated mocks