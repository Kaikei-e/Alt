package fetch_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/errors"
	"alt/utils/logger"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestFetchSingleFeedUsecase_Execute(t *testing.T) {
	// Initialize logger to prevent nil pointer issues
	logger.InitLogger()

	// GoMockコントローラを初期化
	// このコントローラは、テスト終了時にMockオブジェクトの期待値を検証する
	ctrl := gomock.NewController(t)
	defer ctrl.Finish() // テスト終了時にVerify()を呼び出すことを保証
	ctx := context.Background()

	// 成功時の期待されるRSSFeedデータ
	mockSuccessFeed := &domain.RSSFeed{
		Title:         "Test Feed Title",
		Description:   "Description of test feed",
		Link:          "http://example.com/feed",
		FeedLink:      "http://example.com/feed.xml",
		Links:         []string{"http://example.com/article1", "http://example.com/article2"},
		Updated:       "2025-05-30T10:00:00Z",
		UpdatedParsed: time.Date(2025, time.May, 30, 10, 0, 0, 0, time.UTC),
		Language:      "en",
		Image: domain.RSSFeedImage{
			URL:   "http://example.com/image.png",
			Title: "Feed Image",
		},
		Generator: "Test Generator",
		Items: []domain.FeedItem{
			{
				Title:       "Test Article 1",
				Description: "Description 1",
				Link:        "http://example.com/article1",
			},
			{
				Title:       "Test Article 2",
				Description: "Description 2",
				Link:        "http://example.com/article2",
			},
		},
	}

	// Empty feed for edge case testing
	mockEmptyFeed := &domain.RSSFeed{
		Title:       "Empty Feed",
		Description: "Feed with no items",
		Link:        "http://example.com/empty",
		FeedLink:    "http://example.com/empty.xml",
		Items:       []domain.FeedItem{},
	}

	tests := []struct {
		name      string
		mockSetup func(*mocks.MockFetchSingleFeedPort)
		want      *domain.RSSFeed
		wantErr   bool
		errorType string
	}{
		{
			name: "success_with_multiple_items",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(mockSuccessFeed, nil).Times(1)
			},
			want:    mockSuccessFeed,
			wantErr: false,
		},
		{
			name: "success_with_empty_feed",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(mockEmptyFeed, nil).Times(1)
			},
			want:    mockEmptyFeed,
			wantErr: false,
		},
		{
			name: "port_returns_database_error",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				dbErr := errors.DatabaseError(
					"no feeds found",
					fmt.Errorf("table does not exist"),
					map[string]interface{}{
						"gateway": "FetchSingleFeedGateway",
					},
				)
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(nil, dbErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "AppError",
		},
		{
			name: "port_returns_validation_error",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				validationErr := errors.ValidationError("invalid feed URL", map[string]interface{}{
					"url": "invalid-url",
				})
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(nil, validationErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "AppError",
		},
		{
			name: "port_returns_timeout_error",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				timeoutErr := errors.TimeoutError("request timed out", fmt.Errorf("context deadline exceeded"), map[string]interface{}{
					"timeout": "30s",
				})
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(nil, timeoutErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "AppError",
		},
		{
			name: "port_returns_rate_limit_error",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				rateLimitErr := errors.RateLimitError("rate limit exceeded", fmt.Errorf("too many requests"), map[string]interface{}{
					"host":        "example.com",
					"retry_after": "60s",
				})
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(nil, rateLimitErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "AppError",
		},
		{
			name: "port_returns_generic_error",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				genericErr := fmt.Errorf("database connection failed")
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(nil, genericErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "UnknownError",
		},
		{
			name: "context_cancellation",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				contextErr := errors.TimeoutError("context cancelled", fmt.Errorf("context canceled"), map[string]interface{}{
					"reason": "context.Canceled",
				})
				mockPort.EXPECT().FetchSingleFeed(gomock.Any()).Return(nil, contextErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "AppError",
		},
		{
			name: "network_error",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				networkErr := errors.ExternalAPIError("network connection failed", fmt.Errorf("connection refused"), map[string]interface{}{
					"host": "example.com",
					"port": 80,
				})
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(nil, networkErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "AppError",
		},
		{
			name: "parse_error",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				parseErr := errors.ValidationError("invalid RSS format", map[string]interface{}{
					"content_type": "text/html",
					"expected":     "application/rss+xml",
				})
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(nil, parseErr).Times(1)
			},
			want:      nil,
			wantErr:   true,
			errorType: "AppError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPort := mocks.NewMockFetchSingleFeedPort(ctrl)
			tt.mockSetup(mockPort)

			usecase := NewFetchSingleFeedUsecase(mockPort)
			got, err := usecase.Execute(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("FetchSingleFeedUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				// Verify error type when expected
				if tt.errorType != "" {
					switch tt.errorType {
					case "AppError":
						if _, ok := err.(*errors.AppError); !ok {
							t.Errorf("Expected AppError, got %T", err)
						}
					case "UnknownError":
						if appErr, ok := err.(*errors.AppError); ok {
							if appErr.Code != errors.ErrCodeUnknown {
								t.Errorf("Expected UnknownError code, got %v", appErr.Code)
							}
						} else {
							t.Errorf("Expected AppError with UnknownError code, got %T", err)
						}
					}
				}

				// Verify usecase context is added to AppError
				if appErr, ok := err.(*errors.AppError); ok {
					if appErr.Context == nil {
						t.Error("Expected context to be added to AppError")
					} else {
						if usecase, exists := appErr.Context["usecase"]; !exists || usecase != "FetchSingleFeedUsecase" {
							t.Error("Expected usecase context to be set")
						}
						if operation, exists := appErr.Context["operation"]; !exists || operation != "Execute" {
							t.Error("Expected operation context to be set")
						}
					}
				}
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchSingleFeedUsecase.Execute() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchSingleFeedUsecase_Execute_ContextPropagation(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test with context values
	ctx := context.WithValue(context.Background(), "request_id", "test-123")
	ctx = context.WithValue(ctx, "user_id", "user-456")

	mockPort := mocks.NewMockFetchSingleFeedPort(ctrl)

	// Verify that context is passed through correctly
	mockPort.EXPECT().FetchSingleFeed(ctx).DoAndReturn(func(passedCtx context.Context) (*domain.RSSFeed, error) {
		// Verify context values are propagated
		if requestID := passedCtx.Value("request_id"); requestID != "test-123" {
			t.Errorf("Expected request_id 'test-123', got %v", requestID)
		}
		if userID := passedCtx.Value("user_id"); userID != "user-456" {
			t.Errorf("Expected user_id 'user-456', got %v", userID)
		}

		return &domain.RSSFeed{
			Title: "Context Test Feed",
			Link:  "http://example.com/context-test",
		}, nil
	}).Times(1)

	usecase := NewFetchSingleFeedUsecase(mockPort)
	_, err := usecase.Execute(ctx)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestFetchSingleFeedUsecase_Execute_EdgeCases(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	tests := []struct {
		name      string
		mockSetup func(*mocks.MockFetchSingleFeedPort)
		validate  func(*testing.T, *domain.RSSFeed, error)
	}{
		{
			name: "feed_with_special_characters",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				specialFeed := &domain.RSSFeed{
					Title:       "Feed with 特殊文字 and émojis 🚀",
					Description: "Feed containing <script>alert('xss')</script> and other special chars",
					Link:        "http://example.com/special-feed",
					FeedLink:    "http://example.com/special-feed.xml",
				}
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(specialFeed, nil).Times(1)
			},
			validate: func(t *testing.T, feed *domain.RSSFeed, err error) {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if feed == nil {
					t.Error("Expected feed, got nil")
					return
				}
				if feed.Title != "Feed with 特殊文字 and émojis 🚀" {
					t.Errorf("Special characters not preserved in title")
				}
			},
		},
		{
			name: "feed_with_very_long_title",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				longTitle := "This is a very long feed title that might cause issues with some systems because it exceeds normal length expectations and contains a lot of text that should be handled gracefully by the system without causing any truncation or other issues"
				longFeed := &domain.RSSFeed{
					Title:    longTitle,
					Link:     "http://example.com/long-title-feed",
					FeedLink: "http://example.com/long-title-feed.xml",
				}
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(longFeed, nil).Times(1)
			},
			validate: func(t *testing.T, feed *domain.RSSFeed, err error) {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if feed == nil {
					t.Error("Expected feed, got nil")
					return
				}
				if len(feed.Title) < 100 {
					t.Error("Long title was truncated unexpectedly")
				}
			},
		},
		{
			name: "feed_with_null_fields",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				minimalFeed := &domain.RSSFeed{
					Title: "Minimal Feed",
					Link:  "http://example.com/minimal",
					// Other fields left as zero values
				}
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(minimalFeed, nil).Times(1)
			},
			validate: func(t *testing.T, feed *domain.RSSFeed, err error) {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if feed == nil {
					t.Error("Expected feed, got nil")
					return
				}
				// Should handle minimal feeds gracefully
				if feed.Title != "Minimal Feed" {
					t.Error("Minimal feed title not preserved")
				}
			},
		},
		{
			name: "feed_with_large_number_of_items",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				// Create feed with many items
				items := make([]domain.FeedItem, 1000)
				for i := 0; i < 1000; i++ {
					items[i] = domain.FeedItem{
						Title:       fmt.Sprintf("Item %d", i+1),
						Description: fmt.Sprintf("Description for item %d", i+1),
						Link:        fmt.Sprintf("http://example.com/item%d", i+1),
					}
				}

				largeFeed := &domain.RSSFeed{
					Title:    "Large Feed",
					Link:     "http://example.com/large-feed",
					FeedLink: "http://example.com/large-feed.xml",
					Items:    items,
				}
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(largeFeed, nil).Times(1)
			},
			validate: func(t *testing.T, feed *domain.RSSFeed, err error) {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if feed == nil {
					t.Error("Expected feed, got nil")
					return
				}
				if len(feed.Items) != 1000 {
					t.Errorf("Expected 1000 items, got %d", len(feed.Items))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPort := mocks.NewMockFetchSingleFeedPort(ctrl)
			tt.mockSetup(mockPort)

			usecase := NewFetchSingleFeedUsecase(mockPort)
			feed, err := usecase.Execute(ctx)

			tt.validate(t, feed, err)
		})
	}
}
