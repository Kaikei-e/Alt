// ABOUTME: SubscriptionUUIDResolver単体テスト - TDDのREDフェーズ
// ABOUTME: テーブル駆動テストによる包括的なテストカバレッジ

package domain

import (
	"context"
	"fmt"
	"testing"

	"pre-processor-sidecar/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSubscriptionAutoCreator はSubscriptionAutoCreatorのモック
type MockSubscriptionAutoCreator struct {
	mock.Mock
}

func (m *MockSubscriptionAutoCreator) AutoCreateSubscription(
	ctx context.Context,
	originStreamID string,
) (uuid.UUID, error) {
	args := m.Called(ctx, originStreamID)
	return args.Get(0).(uuid.UUID), args.Error(1)
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

// Test UUIDs for consistency
var (
	testUUID1 = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	testUUID2 = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	testUUID3 = uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
)

func TestSubscriptionUUIDResolver_ResolveArticleUUIDs(t *testing.T) {
	tests := []struct {
		name                string
		articles            []*models.Article
		existingMapping     map[string]uuid.UUID
		autoCreateSetup     func(*MockSubscriptionAutoCreator)
		expectedResult      *UUIDResolutionResult
		expectedError       error
		validateArticleUUIDs func(t *testing.T, articles []*models.Article)
	}{
		{
			name: "全記事のUUID解決が成功する場合",
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
			existingMapping: map[string]uuid.UUID{
				"feed/https://example1.com/rss": testUUID1,
				"feed/https://example2.com/rss": testUUID2,
			},
			autoCreateSetup: func(mock *MockSubscriptionAutoCreator) {
				// auto creation is not called in this scenario
			},
			expectedResult: &UUIDResolutionResult{
				ResolvedCount:    2,
				AutoCreatedCount: 0,
				UnknownCount:     0,
				TotalProcessed:   2,
				Errors:          []ResolutionError{},
			},
			expectedError: nil,
			validateArticleUUIDs: func(t *testing.T, articles []*models.Article) {
				assert.Equal(t, testUUID1, articles[0].SubscriptionID)
				assert.Equal(t, testUUID2, articles[1].SubscriptionID)
				// 一時フィールドがクリアされていることを確認
				assert.Empty(t, articles[0].OriginStreamID)
				assert.Empty(t, articles[1].OriginStreamID)
			},
		},
		{
			name: "未知サブスクリプションの自動作成が成功する場合",
			articles: []*models.Article{
				{
					InoreaderID:    "article_003",
					OriginStreamID: "feed/https://unknown.com/rss",
					SubscriptionID: uuid.Nil,
				},
			},
			existingMapping: map[string]uuid.UUID{},
			autoCreateSetup: func(mockAutoCreator *MockSubscriptionAutoCreator) {
				mockAutoCreator.On("AutoCreateSubscription", 
					mock.Anything, 
					"feed/https://unknown.com/rss").Return(testUUID3, nil)
			},
			expectedResult: &UUIDResolutionResult{
				ResolvedCount:    0,
				AutoCreatedCount: 1,
				UnknownCount:     0,
				TotalProcessed:   1,
				Errors:          []ResolutionError{},
			},
			expectedError: nil,
			validateArticleUUIDs: func(t *testing.T, articles []*models.Article) {
				assert.Equal(t, testUUID3, articles[0].SubscriptionID)
				assert.Empty(t, articles[0].OriginStreamID)
			},
		},
		{
			name: "空のOriginStreamIDでエラーハンドリングが正常に動作する場合",
			articles: []*models.Article{
				{
					InoreaderID:    "article_004",
					OriginStreamID: "", // 空の場合
					SubscriptionID: uuid.Nil,
				},
				{
					InoreaderID:    "article_005",
					OriginStreamID: "feed/https://valid.com/rss",
					SubscriptionID: uuid.Nil,
				},
			},
			existingMapping: map[string]uuid.UUID{
				"feed/https://valid.com/rss": testUUID1,
			},
			autoCreateSetup: func(mock *MockSubscriptionAutoCreator) {
				// auto creation is not called for empty stream ID
			},
			expectedResult: &UUIDResolutionResult{
				ResolvedCount:    1,
				AutoCreatedCount: 0,
				UnknownCount:     1,
				TotalProcessed:   2,
				Errors: []ResolutionError{
					{
						ArticleInoreaderID: "article_004",
						OriginStreamID:     "",
						ErrorMessage:       ErrEmptyOriginStreamID.Error(),
						ErrorCode:          "VALIDATION_ERROR",
					},
				},
			},
			expectedError: nil,
			validateArticleUUIDs: func(t *testing.T, articles []*models.Article) {
				// 最初の記事は失敗（uuid.Nil）
				assert.Equal(t, uuid.Nil, articles[0].SubscriptionID)
				// 2番目の記事は成功
				assert.Equal(t, testUUID1, articles[1].SubscriptionID)
				// 両方の一時フィールドがクリアされている
				assert.Empty(t, articles[0].OriginStreamID)
				assert.Empty(t, articles[1].OriginStreamID)
			},
		},
		{
			name: "自動作成が失敗した場合のエラーハンドリング",
			articles: []*models.Article{
				{
					InoreaderID:    "article_006",
					OriginStreamID: "feed/https://fail-create.com/rss",
					SubscriptionID: uuid.Nil,
				},
			},
			existingMapping: map[string]uuid.UUID{},
			autoCreateSetup: func(mockAutoCreator *MockSubscriptionAutoCreator) {
				mockAutoCreator.On("AutoCreateSubscription",
					mock.Anything,
					"feed/https://fail-create.com/rss").Return(
					uuid.Nil, fmt.Errorf("database connection failed"))
			},
			expectedResult: &UUIDResolutionResult{
				ResolvedCount:    0,
				AutoCreatedCount: 0,
				UnknownCount:     1,
				TotalProcessed:   1,
				Errors: []ResolutionError{
					{
						ArticleInoreaderID: "article_006",
						OriginStreamID:     "feed/https://fail-create.com/rss",
						ErrorMessage:       "database connection failed",
						ErrorCode:          "AUTO_CREATION_ERROR",
					},
				},
			},
			expectedError: nil,
			validateArticleUUIDs: func(t *testing.T, articles []*models.Article) {
				assert.Equal(t, uuid.Nil, articles[0].SubscriptionID)
				assert.Empty(t, articles[0].OriginStreamID)
			},
		},
		{
			name: "混合シナリオ：成功・自動作成・エラーが混在する場合",
			articles: []*models.Article{
				{
					InoreaderID:    "article_007",
					OriginStreamID: "feed/https://known.com/rss",
					SubscriptionID: uuid.Nil,
				},
				{
					InoreaderID:    "article_008",
					OriginStreamID: "feed/https://autocreate.com/rss",
					SubscriptionID: uuid.Nil,
				},
				{
					InoreaderID:    "article_009",
					OriginStreamID: "",
					SubscriptionID: uuid.Nil,
				},
			},
			existingMapping: map[string]uuid.UUID{
				"feed/https://known.com/rss": testUUID1,
			},
			autoCreateSetup: func(mockAutoCreator *MockSubscriptionAutoCreator) {
				mockAutoCreator.On("AutoCreateSubscription",
					mock.Anything,
					"feed/https://autocreate.com/rss").Return(testUUID2, nil)
			},
			expectedResult: &UUIDResolutionResult{
				ResolvedCount:    1,
				AutoCreatedCount: 1,
				UnknownCount:     1,
				TotalProcessed:   3,
				Errors: []ResolutionError{
					{
						ArticleInoreaderID: "article_009",
						OriginStreamID:     "",
						ErrorMessage:       ErrEmptyOriginStreamID.Error(),
						ErrorCode:          "VALIDATION_ERROR",
					},
				},
			},
			expectedError: nil,
			validateArticleUUIDs: func(t *testing.T, articles []*models.Article) {
				assert.Equal(t, testUUID1, articles[0].SubscriptionID)
				assert.Equal(t, testUUID2, articles[1].SubscriptionID)
				assert.Equal(t, uuid.Nil, articles[2].SubscriptionID)
				// 全て一時フィールドがクリアされている
				for _, article := range articles {
					assert.Empty(t, article.OriginStreamID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockAutoCreator := new(MockSubscriptionAutoCreator)
			mockLogger := new(MockLogger)

			// Setup auto creator expectations
			tt.autoCreateSetup(mockAutoCreator)

			// Setup logger to accept any calls (not the focus of this test)
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Warn", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Error", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()

			// Create resolver
			resolver := NewSubscriptionUUIDResolver(mockAutoCreator, mockLogger)

			// Setup mapping
			mapping := NewSubscriptionMapping()
			for streamID, subscriptionUUID := range tt.existingMapping {
				mapping.SetMapping(streamID, subscriptionUUID)
			}

			// Execute
			ctx := context.Background()
			result, err := resolver.ResolveArticleUUIDs(ctx, tt.articles, mapping)

			// Assert error
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			// Assert result
			if tt.expectedResult != nil {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.ResolvedCount, result.ResolvedCount)
				assert.Equal(t, tt.expectedResult.AutoCreatedCount, result.AutoCreatedCount)
				assert.Equal(t, tt.expectedResult.UnknownCount, result.UnknownCount)
				assert.Equal(t, tt.expectedResult.TotalProcessed, result.TotalProcessed)
				assert.Equal(t, len(tt.expectedResult.Errors), len(result.Errors))

				// Assert specific errors
				for i, expectedError := range tt.expectedResult.Errors {
					assert.Equal(t, expectedError.ArticleInoreaderID, result.Errors[i].ArticleInoreaderID)
					assert.Equal(t, expectedError.OriginStreamID, result.Errors[i].OriginStreamID)
					assert.Equal(t, expectedError.ErrorMessage, result.Errors[i].ErrorMessage)
					assert.Equal(t, expectedError.ErrorCode, result.Errors[i].ErrorCode)
				}
			}

			// Custom validations
			if tt.validateArticleUUIDs != nil {
				tt.validateArticleUUIDs(t, tt.articles)
			}

			// Verify mocks
			mockAutoCreator.AssertExpectations(t)
		})
	}
}

func TestSubscriptionMapping_ThreadSafety(t *testing.T) {
	mapping := NewSubscriptionMapping()
	
	// Concurrent access test
	done := make(chan bool, 2)
	
	// Writer goroutine
	go func() {
		defer func() { done <- true }()
		for i := 0; i < 100; i++ {
			streamID := fmt.Sprintf("feed/test%d", i)
			subscriptionUUID := uuid.New()
			mapping.SetMapping(streamID, subscriptionUUID)
		}
	}()
	
	// Reader goroutine
	go func() {
		defer func() { done <- true }()
		for i := 0; i < 100; i++ {
			streamID := fmt.Sprintf("feed/test%d", i)
			_, _ = mapping.GetUUID(streamID)
		}
	}()
	
	// Wait for completion
	<-done
	<-done
	
	// Verify final state
	assert.True(t, mapping.Size() >= 0)
	assert.True(t, mapping.Size() <= 100)
}

func TestSubscriptionUUIDResolver_validateArticleForResolution(t *testing.T) {
	resolver := NewSubscriptionUUIDResolver(nil, nil)
	
	tests := []struct {
		name        string
		article     *models.Article
		expectedErr error
	}{
		{
			name:        "nil記事の場合",
			article:     nil,
			expectedErr: ErrInvalidArticle,
		},
		{
			name: "空のOriginStreamIDの場合",
			article: &models.Article{
				InoreaderID:    "test",
				OriginStreamID: "",
			},
			expectedErr: ErrEmptyOriginStreamID,
		},
		{
			name: "有効な記事の場合",
			article: &models.Article{
				InoreaderID:    "test",
				OriginStreamID: "feed/https://example.com/rss",
			},
			expectedErr: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolver.validateArticleForResolution(tt.article)
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}