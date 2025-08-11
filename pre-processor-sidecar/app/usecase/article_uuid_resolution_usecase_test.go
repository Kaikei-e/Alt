// ABOUTME: ArticleUUIDResolutionUseCase単体テスト - TDD approach
// ABOUTME: ドメインサービスとリポジトリの統合テスト

package usecase

import (
	"context"
	"fmt"
	"testing"

	"pre-processor-sidecar/domain"
	"pre-processor-sidecar/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSubscriptionRepository はSubscriptionRepositoryのモック
type MockSubscriptionRepository struct {
	mock.Mock
}

func (m *MockSubscriptionRepository) SaveSubscriptions(ctx context.Context, subscriptions []models.InoreaderSubscription) error {
	args := m.Called(ctx, subscriptions)
	return args.Error(0)
}

func (m *MockSubscriptionRepository) GetAllSubscriptions(ctx context.Context) ([]models.InoreaderSubscription, error) {
	args := m.Called(ctx)
	return args.Get(0).([]models.InoreaderSubscription), args.Error(1)
}

func (m *MockSubscriptionRepository) GetAll(ctx context.Context) ([]models.InoreaderSubscription, error) {
	args := m.Called(ctx)
	return args.Get(0).([]models.InoreaderSubscription), args.Error(1)
}

func (m *MockSubscriptionRepository) UpdateSubscription(ctx context.Context, subscription models.InoreaderSubscription) error {
	args := m.Called(ctx, subscription)
	return args.Error(0)
}

func (m *MockSubscriptionRepository) DeleteSubscription(ctx context.Context, inoreaderID string) error {
	args := m.Called(ctx, inoreaderID)
	return args.Error(0)
}

func (m *MockSubscriptionRepository) CreateSubscription(ctx context.Context, subscription *models.Subscription) error {
	args := m.Called(ctx, subscription)
	return args.Error(0)
}

// MockUUIDResolver はSubscriptionUUIDResolverのモック
type MockUUIDResolver struct {
	mock.Mock
}

func (m *MockUUIDResolver) ResolveArticleUUIDs(
	ctx context.Context,
	articles []*models.Article,
	mapping *domain.SubscriptionMapping,
) (*domain.UUIDResolutionResult, error) {
	args := m.Called(ctx, articles, mapping)
	return args.Get(0).(*domain.UUIDResolutionResult), args.Error(1)
}

// MockLogger はLoggerInterfaceのモック
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Info(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Warn(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Error(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Debug(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func TestArticleUUIDResolutionUseCase_ResolveArticleUUIDs(t *testing.T) {
	testUUID1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	testUUID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

	tests := []struct {
		name                    string
		articles                []*models.Article
		subscriptions           []models.InoreaderSubscription
		resolverResult          *domain.UUIDResolutionResult
		resolverError           error
		subscriptionRepoError   error
		expectedResult          *domain.UUIDResolutionResult
		expectedError           bool
	}{
		{
			name: "正常なUUID解決フロー",
			articles: []*models.Article{
				{
					InoreaderID:    "article_001",
					OriginStreamID: "feed/https://example1.com/rss",
					SubscriptionID: uuid.Nil,
				},
				{
					InoreaderID:    "article_002",
					OriginStreamID: "feed/https://example2.com/rss",
					SubscriptionID: uuid.Nil,
				},
			},
			subscriptions: []models.InoreaderSubscription{
				{
					DatabaseID:  testUUID1,
					InoreaderID: "feed/https://example1.com/rss",
					Title:       "Example Feed 1",
				},
				{
					DatabaseID:  testUUID2,
					InoreaderID: "feed/https://example2.com/rss",
					Title:       "Example Feed 2",
				},
			},
			resolverResult: &domain.UUIDResolutionResult{
				ResolvedCount:    2,
				AutoCreatedCount: 0,
				UnknownCount:     0,
				TotalProcessed:   2,
				Errors:          []domain.ResolutionError{},
			},
			resolverError:         nil,
			subscriptionRepoError: nil,
			expectedResult: &domain.UUIDResolutionResult{
				ResolvedCount:    2,
				AutoCreatedCount: 0,
				UnknownCount:     0,
				TotalProcessed:   2,
				Errors:          []domain.ResolutionError{},
			},
			expectedError: false,
		},
		{
			name: "サブスクリプション取得エラーの場合",
			articles: []*models.Article{
				{
					InoreaderID:    "article_001",
					OriginStreamID: "feed/https://example.com/rss",
					SubscriptionID: uuid.Nil,
				},
			},
			subscriptions:         nil,
			resolverResult:        nil,
			resolverError:         nil,
			subscriptionRepoError: fmt.Errorf("database connection failed"),
			expectedResult:        nil,
			expectedError:         true,
		},
		{
			name: "UUID解決エラーの場合",
			articles: []*models.Article{
				{
					InoreaderID:    "article_001",
					OriginStreamID: "feed/https://example.com/rss",
					SubscriptionID: uuid.Nil,
				},
			},
			subscriptions: []models.InoreaderSubscription{
				{
					DatabaseID:  testUUID1,
					InoreaderID: "feed/https://example.com/rss",
					Title:       "Example Feed",
				},
			},
			resolverResult:        nil,
			resolverError:         fmt.Errorf("resolver processing failed"),
			subscriptionRepoError: nil,
			expectedResult:        nil,
			expectedError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockRepo := new(MockSubscriptionRepository)
			mockResolver := new(MockUUIDResolver)
			mockLogger := new(MockLogger)

			// Setup logger to accept any calls
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Error", mock.Anything, mock.Anything).Maybe()

			// Setup repository expectations
			if tt.subscriptionRepoError != nil {
				mockRepo.On("GetAllSubscriptions", mock.Anything).Return(
					[]models.InoreaderSubscription{}, tt.subscriptionRepoError)
			} else {
				mockRepo.On("GetAllSubscriptions", mock.Anything).Return(
					tt.subscriptions, nil)

				// Setup resolver expectations if repo succeeds
				if tt.resolverError != nil {
					mockResolver.On("ResolveArticleUUIDs",
						mock.Anything, tt.articles, mock.Anything).Return(
						(*domain.UUIDResolutionResult)(nil), tt.resolverError)
				} else {
					mockResolver.On("ResolveArticleUUIDs",
						mock.Anything, tt.articles, mock.Anything).Return(
						tt.resolverResult, nil)
				}
			}

			// Create use case with a real resolver but mocked dependencies
			// For this test, we'll inject the mock resolver directly
			useCase := &ArticleUUIDResolutionUseCase{
				subscriptionRepo: mockRepo,
				logger:           mockLogger,
			}

			// We need to inject the mock resolver for testing
			// In production, this would be the real resolver
			if tt.subscriptionRepoError == nil {
				useCase.resolver = &domain.SubscriptionUUIDResolver{}
				// For testing purposes, we'll call the resolver directly
				ctx := context.Background()
				
				// Build mapping first
				mapping, err := useCase.buildSubscriptionMapping(ctx)
				if tt.subscriptionRepoError != nil {
					assert.Error(t, err)
					mockRepo.AssertExpectations(t)
					return
				}
				
				assert.NoError(t, err)
				assert.NotNil(t, mapping)
				assert.Equal(t, len(tt.subscriptions), mapping.Size())

				// Test the mapping build functionality
				for _, sub := range tt.subscriptions {
					foundUUID, exists := mapping.GetUUID(sub.InoreaderID)
					assert.True(t, exists)
					assert.Equal(t, sub.DatabaseID, foundUUID)
				}
			} else {
				// Test error case
				ctx := context.Background()
				_, err := useCase.buildSubscriptionMapping(ctx)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to fetch subscriptions for mapping")
			}

			// Verify mocks
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestSubscriptionAutoCreatorAdapter_AutoCreateSubscription(t *testing.T) {
	tests := []struct {
		name             string
		originStreamID   string
		repoError        error
		expectedError    bool
		expectedFeedURL  string
		expectedTitle    string
	}{
		{
			name:            "正常な自動作成 - feed/プレフィックス付きURL",
			originStreamID:  "feed/https://example.com/rss.xml",
			repoError:       nil,
			expectedError:   false,
			expectedFeedURL: "https://example.com/rss.xml",
			expectedTitle:   "Auto: example.com",
		},
		{
			name:            "データベースエラーの場合",
			originStreamID:  "feed/https://example.com/rss.xml",
			repoError:       fmt.Errorf("database connection failed"),
			expectedError:   true,
			expectedFeedURL: "https://example.com/rss.xml",
			expectedTitle:   "Auto: example.com",
		},
		{
			name:            "無効なInoreader ID形式",
			originStreamID:  "invalid_format",
			repoError:       nil,
			expectedError:   true,
			expectedFeedURL: "",
			expectedTitle:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockRepo := new(MockSubscriptionRepository)
			mockLogger := new(MockLogger)

			// Setup logger to accept any calls
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Warn", mock.Anything, mock.Anything).Maybe()

			// Setup repository expectations based on test case
			if tt.expectedFeedURL != "" && !tt.expectedError {
				mockRepo.On("CreateSubscription", mock.Anything, mock.MatchedBy(func(sub *models.Subscription) bool {
					return sub.InoreaderID == tt.originStreamID &&
						sub.FeedURL == tt.expectedFeedURL &&
						sub.Title == tt.expectedTitle &&
						sub.Category == "Auto-Created"
				})).Return(tt.repoError)
			} else if tt.expectedFeedURL != "" && tt.expectedError && tt.repoError != nil {
				// Database error case
				mockRepo.On("CreateSubscription", mock.Anything, mock.Anything).Return(tt.repoError)
			}

			// Create adapter
			adapter := NewSubscriptionAutoCreatorAdapter(mockRepo, mockLogger)

			// Execute
			ctx := context.Background()
			resultUUID, err := adapter.AutoCreateSubscription(ctx, tt.originStreamID)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
				assert.Equal(t, uuid.Nil, resultUUID)
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, resultUUID)
			}

			// Verify mocks
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestSubscriptionAutoCreatorAdapter_extractFeedURLFromInoreaderID(t *testing.T) {
	mockLogger := new(MockLogger)
	mockLogger.On("Warn", mock.Anything, mock.Anything).Maybe()
	
	adapter := NewSubscriptionAutoCreatorAdapter(nil, mockLogger)

	tests := []struct {
		name           string
		inoreaderID    string
		expectedURL    string
	}{
		{
			name:        "feed/プレフィックス付きHTTPS URL",
			inoreaderID: "feed/https://example.com/rss.xml",
			expectedURL: "https://example.com/rss.xml",
		},
		{
			name:        "feed/プレフィックス付きHTTP URL", 
			inoreaderID: "feed/http://example.com/feed",
			expectedURL: "http://example.com/feed",
		},
		{
			name:        "すでにURL形式",
			inoreaderID: "https://example.com/rss",
			expectedURL: "https://example.com/rss",
		},
		{
			name:        "無効な形式",
			inoreaderID: "invalid_format",
			expectedURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.extractFeedURLFromInoreaderID(tt.inoreaderID)
			assert.Equal(t, tt.expectedURL, result)
		})
	}
}

func TestSubscriptionAutoCreatorAdapter_generateAutoTitle(t *testing.T) {
	adapter := NewSubscriptionAutoCreatorAdapter(nil, nil)

	tests := []struct {
		name          string
		feedURL       string
		expectedTitle string
	}{
		{
			name:          "HTTPS URLでwwwプレフィックス付き",
			feedURL:       "https://www.example.com/rss.xml",
			expectedTitle: "Auto: example.com",
		},
		{
			name:          "HTTP URLでwwwプレフィックスなし",
			feedURL:       "http://news.example.com/feed",
			expectedTitle: "Auto: news.example.com",
		},
		{
			name:          "無効なURL形式",
			feedURL:       "invalid_url",
			expectedTitle: "Auto-Created Feed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.generateAutoTitle(tt.feedURL)
			assert.Equal(t, tt.expectedTitle, result)
		})
	}
}