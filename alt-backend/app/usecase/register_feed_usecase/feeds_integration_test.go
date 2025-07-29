package register_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestRegisterFeedsUsecase_Execute_IntegrationFlow(t *testing.T) {
	// Initialize logger to prevent nil pointer issues
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	// Mock dependencies
	mockRegisterFeedLinkGateway := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsGateway := mocks.NewMockRegisterFeedsPort(ctrl)
	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)

	// Test data
	rssURL := "https://example.com/feed.xml"

	// Mock feed items that should be fetched from external URL
	mockFeedItems := []*domain.FeedItem{
		{
			Title:           "Test Article 1",
			Description:     "Description for article 1",
			Link:            "https://example.com/article1",
			Published:       "2025-01-13T10:00:00Z",
			PublishedParsed: time.Date(2025, 1, 13, 10, 0, 0, 0, time.UTC),
			Author: domain.Author{
				Name: "Test Author",
			},
			Authors: []domain.Author{
				{Name: "Test Author"},
			},
			Links: []string{"https://example.com/article1"},
		},
		{
			Title:           "Test Article 2",
			Description:     "Description for article 2",
			Link:            "https://example.com/article2",
			Published:       "2025-01-13T11:00:00Z",
			PublishedParsed: time.Date(2025, 1, 13, 11, 0, 0, 0, time.UTC),
			Author: domain.Author{
				Name: "Test Author",
			},
			Authors: []domain.Author{
				{Name: "Test Author"},
			},
			Links: []string{"https://example.com/article2"},
		},
	}

	tests := []struct {
		name      string
		rssURL    string
		mockSetup func()
		wantErr   bool
		validate  func(*testing.T, error)
	}{
		{
			name:   "successful_complete_flow",
			rssURL: rssURL,
			mockSetup: func() {
				// 1. RSS feed link registration should succeed
				mockRegisterFeedLinkGateway.EXPECT().
					RegisterRSSFeedLink(ctx, rssURL).
					Return(nil).
					Times(1)

				// 2. External feed fetching should succeed
				mockFetchFeedGateway.EXPECT().
					FetchFeeds(ctx, rssURL).
					Return(mockFeedItems, nil).
					Times(1)

				// 3. Feed items should be stored in database
				mockRegisterFeedsGateway.EXPECT().
					RegisterFeeds(ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, feedItems []*domain.FeedItem) error {
						// Verify that feed items are passed correctly
						if len(feedItems) != 2 {
							return fmt.Errorf("expected 2 feed items, got %d", len(feedItems))
						}

						// Verify first item
						if feedItems[0].Title != "Test Article 1" {
							return fmt.Errorf("expected first item title 'Test Article 1', got %s", feedItems[0].Title)
						}

						// Verify second item
						if feedItems[1].Title != "Test Article 2" {
							return fmt.Errorf("expected second item title 'Test Article 2', got %s", feedItems[1].Title)
						}

						return nil
					}).
					Times(1)
			},
			wantErr: false,
			validate: func(t *testing.T, err error) {
				if err != nil {
					t.Errorf("Expected successful flow, got error: %v", err)
				}
			},
		},
		{
			name:   "feed_link_registration_fails",
			rssURL: rssURL,
			mockSetup: func() {
				// RSS feed link registration fails
				mockRegisterFeedLinkGateway.EXPECT().
					RegisterRSSFeedLink(ctx, rssURL).
					Return(fmt.Errorf("database connection failed")).
					Times(1)

				// Other methods should not be called
				mockFetchFeedGateway.EXPECT().
					FetchFeeds(gomock.Any(), gomock.Any()).
					Times(0)

				mockRegisterFeedsGateway.EXPECT().
					RegisterFeeds(gomock.Any(), gomock.Any()).
					Times(0)
			},
			wantErr: true,
			validate: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error for failed feed link registration")
				}
				if err.Error() != "failed to register RSS feed link" {
					t.Errorf("Expected specific error message, got: %s", err.Error())
				}
			},
		},
		{
			name:   "external_feed_fetch_fails",
			rssURL: rssURL,
			mockSetup: func() {
				// RSS feed link registration succeeds
				mockRegisterFeedLinkGateway.EXPECT().
					RegisterRSSFeedLink(ctx, rssURL).
					Return(nil).
					Times(1)

				// External feed fetching fails
				mockFetchFeedGateway.EXPECT().
					FetchFeeds(ctx, rssURL).
					Return(nil, fmt.Errorf("network timeout")).
					Times(1)

				// Feed storage should not be called
				mockRegisterFeedsGateway.EXPECT().
					RegisterFeeds(gomock.Any(), gomock.Any()).
					Times(0)
			},
			wantErr: true,
			validate: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error for failed external feed fetch")
				}
				if err.Error() != "failed to fetch feeds" {
					t.Errorf("Expected specific error message, got: %s", err.Error())
				}
			},
		},
		{
			name:   "feed_storage_fails",
			rssURL: rssURL,
			mockSetup: func() {
				// RSS feed link registration succeeds
				mockRegisterFeedLinkGateway.EXPECT().
					RegisterRSSFeedLink(ctx, rssURL).
					Return(nil).
					Times(1)

				// External feed fetching succeeds
				mockFetchFeedGateway.EXPECT().
					FetchFeeds(ctx, rssURL).
					Return(mockFeedItems, nil).
					Times(1)

				// Feed storage fails
				mockRegisterFeedsGateway.EXPECT().
					RegisterFeeds(ctx, gomock.Any()).
					Return(fmt.Errorf("database write failed")).
					Times(1)
			},
			wantErr: true,
			validate: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error for failed feed storage")
				}
				if err.Error() != "failed to register feeds" {
					t.Errorf("Expected specific error message, got: %s", err.Error())
				}
			},
		},
		{
			name:   "empty_url_should_fail",
			rssURL: "",
			mockSetup: func() {
				// Empty URL should fail validation
				mockRegisterFeedLinkGateway.EXPECT().
					RegisterRSSFeedLink(ctx, "").
					Return(fmt.Errorf("RSS feed link cannot be empty")).
					Times(1)
			},
			wantErr: true,
			validate: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error for empty URL")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks for this test case
			tt.mockSetup()

			// Create usecase with mocked dependencies
			usecase := NewRegisterFeedsUsecase(
				mockRegisterFeedLinkGateway,
				mockRegisterFeedsGateway,
				mockFetchFeedGateway,
			)

			// Execute the usecase
			err := usecase.Execute(ctx, tt.rssURL)

			// Validate results
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedsUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			tt.validate(t, err)
		})
	}
}

func TestRegisterFeedsUsecase_Execute_RealWorldScenarios(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	// Mock dependencies
	mockRegisterFeedLinkGateway := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsGateway := mocks.NewMockRegisterFeedsPort(ctrl)
	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)

	t.Run("large_feed_with_many_items", func(t *testing.T) {
		rssURL := "https://news.ycombinator.com/rss"

		// Create 100 mock feed items
		largeFeedItems := make([]*domain.FeedItem, 100)
		for i := 0; i < 100; i++ {
			largeFeedItems[i] = &domain.FeedItem{
				Title:           fmt.Sprintf("News Item %d", i+1),
				Description:     fmt.Sprintf("Description for news item %d", i+1),
				Link:            fmt.Sprintf("https://news.ycombinator.com/item?id=%d", i+1),
				Published:       time.Now().Add(time.Duration(-i) * time.Hour).Format(time.RFC3339),
				PublishedParsed: time.Now().Add(time.Duration(-i) * time.Hour),
			}
		}

		mockRegisterFeedLinkGateway.EXPECT().
			RegisterRSSFeedLink(ctx, rssURL).
			Return(nil).
			Times(1)

		mockFetchFeedGateway.EXPECT().
			FetchFeeds(ctx, rssURL).
			Return(largeFeedItems, nil).
			Times(1)

		mockRegisterFeedsGateway.EXPECT().
			RegisterFeeds(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, feedItems []*domain.FeedItem) error {
				if len(feedItems) != 100 {
					return fmt.Errorf("expected 100 feed items, got %d", len(feedItems))
				}
				return nil
			}).
			Times(1)

		usecase := NewRegisterFeedsUsecase(
			mockRegisterFeedLinkGateway,
			mockRegisterFeedsGateway,
			mockFetchFeedGateway,
		)

		err := usecase.Execute(ctx, rssURL)
		if err != nil {
			t.Errorf("Expected success for large feed, got error: %v", err)
		}
	})

	t.Run("feed_with_special_characters", func(t *testing.T) {
		rssURL := "https://example.com/特殊文字フィード.xml"

		specialFeedItems := []*domain.FeedItem{
			{
				Title:           "記事タイトル with émojis 🚀 and <script>tags</script>",
				Description:     "Description with special chars: ñáéíóú & HTML <b>tags</b>",
				Link:            "https://example.com/記事/1",
				Published:       time.Now().Format(time.RFC3339),
				PublishedParsed: time.Now(),
			},
		}

		mockRegisterFeedLinkGateway.EXPECT().
			RegisterRSSFeedLink(ctx, rssURL).
			Return(nil).
			Times(1)

		mockFetchFeedGateway.EXPECT().
			FetchFeeds(ctx, rssURL).
			Return(specialFeedItems, nil).
			Times(1)

		mockRegisterFeedsGateway.EXPECT().
			RegisterFeeds(ctx, gomock.Any()).
			Return(nil).
			Times(1)

		usecase := NewRegisterFeedsUsecase(
			mockRegisterFeedLinkGateway,
			mockRegisterFeedsGateway,
			mockFetchFeedGateway,
		)

		err := usecase.Execute(ctx, rssURL)
		if err != nil {
			t.Errorf("Expected success for special characters feed, got error: %v", err)
		}
	})
}
