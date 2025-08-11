package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// Use the generated mocks from mocks package

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

func TestArticleFetchService_FetchArticles(t *testing.T) {
	tests := map[string]struct {
		streamID              string
		maxArticles           int
		mockInoreaderResponse []map[string]interface{}
		mockContinuationToken string
		mockSyncState         *models.SyncState
		mockSetup             func(*mocks.MockOAuth2Driver, *mocks.MockArticleRepository, *mocks.MockSyncStateRepository, *mocks.MockSubscriptionRepository)
		expectError           bool
		expectedArticleCount  int
		expectedContinuation  string
	}{
		"successful_first_fetch": {
			streamID:    "user/-/state/com.google/reading-list",
			maxArticles: 100,
			mockInoreaderResponse: []map[string]interface{}{
				{
					"id":        "tag:google.com,2005:reader/item/feed/http://example.com/rss#article1",
					"title":     "First Article",
					"author":    "John Doe",
					"published": float64(1672531200), // Unix timestamp
					"alternate": []interface{}{
						map[string]interface{}{
							"href": "http://example.com/article1",
							"type": "text/html",
						},
					},
					"origin": map[string]interface{}{
						"streamId": "feed/http://example.com/rss",
						"title":    "Example Feed",
					},
				},
				{
					"id":        "tag:google.com,2005:reader/item/feed/http://example.com/rss#article2",
					"title":     "Second Article",
					"author":    "Jane Smith",
					"published": float64(1672617600),
					"alternate": []interface{}{
						map[string]interface{}{
							"href": "http://example.com/article2",
							"type": "text/html",
						},
					},
					"origin": map[string]interface{}{
						"streamId": "feed/http://example.com/rss",
						"title":    "Example Feed",
					},
				},
			},
			mockContinuationToken: "next_page_token_123",
			mockSyncState:         nil, // No existing sync state
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, articleRepo *mocks.MockArticleRepository, syncRepo *mocks.MockSyncStateRepository, subscriptionRepo *mocks.MockSubscriptionRepository) {
				// Setup subscription mock
				setupSubscriptionMock(subscriptionRepo)

				// Mock Inoreader API call
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(
						gomock.Any(),
						"valid_token",
						"/stream/contents/user%2F-%2Fstate%2Fcom.google%2Freading-list",
						map[string]string{
							"output": "json",
							"n":      "100",
						},
					).
					Return(map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"id":        "tag:google.com,2005:reader/item/feed/http://example.com/rss#article1",
								"title":     "First Article",
								"author":    "John Doe",
								"published": float64(1672531200),
								"alternate": []interface{}{
									map[string]interface{}{
										"href": "http://example.com/article1",
										"type": "text/html",
									},
								},
								"origin": map[string]interface{}{
									"streamId": "feed/http://example.com/rss",
									"title":    "Example Feed",
								},
							},
							map[string]interface{}{
								"id":        "tag:google.com,2005:reader/item/feed/http://example.com/rss#article2",
								"title":     "Second Article",
								"author":    "Jane Smith",
								"published": float64(1672617600),
								"alternate": []interface{}{
									map[string]interface{}{
										"href": "http://example.com/article2",
										"type": "text/html",
									},
								},
								"origin": map[string]interface{}{
									"streamId": "feed/http://example.com/rss",
									"title":    "Example Feed",
								},
							},
						},
						"continuation": "next_page_token_123",
					}, nil)

				// Mock sync state repository calls (no existing state)
				syncRepo.EXPECT().
					FindByStreamID(gomock.Any(), "user/-/state/com.google/reading-list").
					Return(nil, fmt.Errorf("not found"))

				// Mock article repository calls for new articles
				articleRepo.EXPECT().
					FindByInoreaderID(gomock.Any(), "tag:google.com,2005:reader/item/feed/http://example.com/rss#article1").
					Return(nil, fmt.Errorf("not found"))
				articleRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)

				articleRepo.EXPECT().
					FindByInoreaderID(gomock.Any(), "tag:google.com,2005:reader/item/feed/http://example.com/rss#article2").
					Return(nil, fmt.Errorf("not found"))
				articleRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)

				// Mock sync state creation with continuation token
				syncRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:          false,
			expectedArticleCount: 2,
			expectedContinuation: "next_page_token_123",
		},
		"fetch_with_continuation_token": {
			streamID:    "user/-/state/com.google/reading-list",
			maxArticles: 100,
			mockInoreaderResponse: []map[string]interface{}{
				{
					"id":        "tag:google.com,2005:reader/item/feed/http://example.com/rss#article3",
					"title":     "Third Article",
					"author":    "Bob Wilson",
					"published": float64(1672704000),
					"alternate": []interface{}{
						map[string]interface{}{
							"href": "http://example.com/article3",
							"type": "text/html",
						},
					},
					"origin": map[string]interface{}{
						"streamId": "feed/http://example.com/rss",
						"title":    "Example Feed",
					},
				},
			},
			mockContinuationToken: "", // Last page
			mockSyncState: &models.SyncState{
				StreamID:          "user/-/state/com.google/reading-list",
				ContinuationToken: "existing_token_456",
				LastSync:          time.Now().Add(-15 * time.Minute),
			},
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, articleRepo *mocks.MockArticleRepository, syncRepo *mocks.MockSyncStateRepository, subscriptionRepo *mocks.MockSubscriptionRepository) {
				// Setup subscription mock
				setupSubscriptionMock(subscriptionRepo)

				// Mock Inoreader API call with continuation token
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(
						gomock.Any(),
						"valid_token",
						"/stream/contents/user%2F-%2Fstate%2Fcom.google%2Freading-list",
						map[string]string{
							"output": "json",
							"n":      "100",
							"c":      "existing_token_456",
						},
					).
					Return(map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"id":        "tag:google.com,2005:reader/item/feed/http://example.com/rss#article3",
								"title":     "Third Article",
								"author":    "Bob Wilson",
								"published": float64(1672704000),
								"alternate": []interface{}{
									map[string]interface{}{
										"href": "http://example.com/article3",
										"type": "text/html",
									},
								},
								"origin": map[string]interface{}{
									"streamId": "feed/http://example.com/rss",
									"title":    "Example Feed",
								},
							},
						},
						// No continuation token = last page
					}, nil)

				// Mock sync state repository calls (existing state)
				syncRepo.EXPECT().
					FindByStreamID(gomock.Any(), "user/-/state/com.google/reading-list").
					Return(&models.SyncState{
						StreamID:          "user/-/state/com.google/reading-list",
						ContinuationToken: "existing_token_456",
						LastSync:          time.Now().Add(-15 * time.Minute),
					}, nil)

				// Mock article repository calls
				articleRepo.EXPECT().
					FindByInoreaderID(gomock.Any(), "tag:google.com,2005:reader/item/feed/http://example.com/rss#article3").
					Return(nil, fmt.Errorf("not found"))
				articleRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)

				// Mock sync state update (continuation token cleared)
				syncRepo.EXPECT().
					Update(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:          false,
			expectedArticleCount: 1,
			expectedContinuation: "", // Last page
		},
		"api_rate_limit_error": {
			streamID:      "user/-/state/com.google/reading-list",
			maxArticles:   100,
			mockSyncState: nil,
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, articleRepo *mocks.MockArticleRepository, syncRepo *mocks.MockSyncStateRepository, subscriptionRepo *mocks.MockSubscriptionRepository) {
				// Setup subscription mock
				setupSubscriptionMock(subscriptionRepo)

				// Mock rate limit exceeded
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(
						gomock.Any(),
						"valid_token",
						"/stream/contents/user%2F-%2Fstate%2Fcom.google%2Freading-list",
						map[string]string{
							"output": "json",
							"n":      "100",
						},
					).
					Return(nil, fmt.Errorf("API rate limit exceeded (Zone 1: 100/100)"))

				// Mock sync state repository calls
				syncRepo.EXPECT().
					FindByStreamID(gomock.Any(), "user/-/state/com.google/reading-list").
					Return(nil, fmt.Errorf("not found"))
			},
			expectError:          true,
			expectedArticleCount: 0,
			expectedContinuation: "",
		},
		"duplicate_article_skip": {
			streamID:      "user/-/state/com.google/reading-list",
			maxArticles:   100,
			mockSyncState: nil,
			mockSetup: func(oauth2Client *mocks.MockOAuth2Driver, articleRepo *mocks.MockArticleRepository, syncRepo *mocks.MockSyncStateRepository, subscriptionRepo *mocks.MockSubscriptionRepository) {
				// Setup subscription mock
				setupSubscriptionMock(subscriptionRepo)

				// Mock Inoreader API call
				oauth2Client.EXPECT().
					MakeAuthenticatedRequest(
						gomock.Any(),
						"valid_token",
						"/stream/contents/user%2F-%2Fstate%2Fcom.google%2Freading-list",
						map[string]string{
							"output": "json",
							"n":      "100",
						},
					).
					Return(map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"id":        "tag:google.com,2005:reader/item/feed/http://example.com/rss#existing",
								"title":     "Existing Article",
								"author":    "Author Name",
								"published": float64(1672531200),
								"alternate": []interface{}{
									map[string]interface{}{
										"href": "http://example.com/existing",
										"type": "text/html",
									},
								},
								"origin": map[string]interface{}{
									"streamId": "feed/http://example.com/rss",
									"title":    "Example Feed",
								},
							},
						},
					}, nil)

				// Mock sync state repository
				syncRepo.EXPECT().
					FindByStreamID(gomock.Any(), "user/-/state/com.google/reading-list").
					Return(nil, fmt.Errorf("not found"))

				// Mock article repository - article already exists
				existingArticle := &models.Article{
					InoreaderID: "tag:google.com,2005:reader/item/feed/http://example.com/rss#existing",
					Title:       "Existing Article",
					ArticleURL:  "http://example.com/existing",
				}
				articleRepo.EXPECT().
					FindByInoreaderID(gomock.Any(), "tag:google.com,2005:reader/item/feed/http://example.com/rss#existing").
					Return(existingArticle, nil)
				// No Create call expected since article exists

				// Mock sync state creation
				syncRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:          false,
			expectedArticleCount: 0, // No new articles created
			expectedContinuation: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOAuth2Client := mocks.NewMockOAuth2Driver(ctrl)
			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockSyncRepo := mocks.NewMockSyncStateRepository(ctrl)
			mockSubscriptionRepo := mocks.NewMockSubscriptionRepository(ctrl)

			tc.mockSetup(mockOAuth2Client, mockArticleRepo, mockSyncRepo, mockSubscriptionRepo)

			// Create Inoreader service using client wrapper over OAuth2Driver mock
			inoreaderClient := NewInoreaderClient(mockOAuth2Client, nil)
			inoreaderService := NewInoreaderService(inoreaderClient, nil, nil, nil)
			// Token management is now handled by SimpleTokenService

			// Set rate limit to allow requests
			inoreaderService.rateLimitInfo = &models.APIRateLimitInfo{
				Zone1Usage:     25,
				Zone1Limit:     100,
				Zone1Remaining: 75,
			}

			// Create article fetch service
			articleService := NewArticleFetchService(
				inoreaderService,
				mockArticleRepo,
				mockSyncRepo,
				mockSubscriptionRepo,
				nil,
			)

			ctx := context.Background()
			result, err := articleService.FetchArticles(ctx, tc.streamID, tc.maxArticles)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.expectedArticleCount, result.NewArticles)
				assert.Equal(t, tc.expectedContinuation, result.ContinuationToken)
			}
		})
	}
}

func TestArticleFetchService_ProcessArticleBatch(t *testing.T) {
	tests := map[string]struct {
		articles          []*models.Article
		mockSetup         func(*mocks.MockArticleRepository)
		expectError       bool
		expectedProcessed int
		expectedSkipped   int
	}{
		"process_all_new_articles": {
			articles: []*models.Article{
				{
					InoreaderID:    "article1",
					SubscriptionID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
					Title:          "Article 1",
					ArticleURL:     "http://example.com/1",
				},
				{
					InoreaderID:    "article2",
					SubscriptionID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
					Title:          "Article 2",
					ArticleURL:     "http://example.com/2",
				},
			},
			mockSetup: func(repo *mocks.MockArticleRepository) {
				// Both articles are new
				repo.EXPECT().
					FindByInoreaderID(gomock.Any(), "article1").
					Return(nil, fmt.Errorf("not found"))
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)

				repo.EXPECT().
					FindByInoreaderID(gomock.Any(), "article2").
					Return(nil, fmt.Errorf("not found"))
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError:       false,
			expectedProcessed: 2,
			expectedSkipped:   0,
		},
		"skip_existing_articles": {
			articles: []*models.Article{
				{
					InoreaderID:    "existing_article",
					SubscriptionID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
					Title:          "Existing Article",
					ArticleURL:     "http://example.com/existing",
				},
			},
			mockSetup: func(repo *mocks.MockArticleRepository) {
				// Article already exists
				existingArticle := &models.Article{
					InoreaderID: "existing_article",
					Title:       "Existing Article",
					ArticleURL:  "http://example.com/existing",
				}
				repo.EXPECT().
					FindByInoreaderID(gomock.Any(), "existing_article").
					Return(existingArticle, nil)
				// No Create call expected
			},
			expectError:       false,
			expectedProcessed: 0,
			expectedSkipped:   1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			tc.mockSetup(mockArticleRepo)

			articleService := NewArticleFetchService(nil, mockArticleRepo, nil, nil, nil)

			ctx := context.Background()
			processed, skipped, err := articleService.ProcessArticleBatch(ctx, tc.articles)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedProcessed, processed)
				assert.Equal(t, tc.expectedSkipped, skipped)
			}
		})
	}
}

// Tests use proper gomock generated mocks
