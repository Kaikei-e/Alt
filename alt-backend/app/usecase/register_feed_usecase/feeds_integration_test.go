package register_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	register_feed_port "alt/port/register_feed_port"
	"alt/utils/logger"
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func integrationRegisterFeedResults(ids ...string) []register_feed_port.RegisterFeedResult {
	results := make([]register_feed_port.RegisterFeedResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, register_feed_port.RegisterFeedResult{
			ArticleID: id,
			Created:   true,
		})
	}
	return results
}

func TestRegisterFeedsUsecase_Execute_IntegrationFlow(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkGateway := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsGateway := mocks.NewMockRegisterFeedsPort(ctrl)

	rssURL := "https://example.com/feed.xml"

	mockFeedItems := []*domain.FeedItem{
		{
			Title:           "Test Article 1",
			Description:     "Description for article 1",
			Link:            "https://example.com/article1",
			Published:       "2025-01-13T10:00:00Z",
			PublishedParsed: time.Date(2025, 1, 13, 10, 0, 0, 0, time.UTC),
			Author:          domain.Author{Name: "Test Author"},
			Authors:         []domain.Author{{Name: "Test Author"}},
			Links:           []string{"https://example.com/article1"},
		},
		{
			Title:           "Test Article 2",
			Description:     "Description for article 2",
			Link:            "https://example.com/article2",
			Published:       "2025-01-13T11:00:00Z",
			PublishedParsed: time.Date(2025, 1, 13, 11, 0, 0, 0, time.UTC),
			Author:          domain.Author{Name: "Test Author"},
			Authors:         []domain.Author{{Name: "Test Author"}},
			Links:           []string{"https://example.com/article2"},
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
				pf := &domain.ParsedFeed{FeedLink: rssURL, Items: mockFeedItems}
				mockValidateFetch.EXPECT().ValidateAndFetch(ctx, rssURL).Return(pf, nil).Times(1)
				mockRegisterFeedLinkGateway.EXPECT().RegisterFeedLink(ctx, rssURL).Return(nil).Times(1)
				mockRegisterFeedsGateway.EXPECT().
					RegisterFeeds(ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, feedItems []*domain.FeedItem) ([]register_feed_port.RegisterFeedResult, error) {
						if len(feedItems) != 2 {
							return nil, fmt.Errorf("expected 2 feed items, got %d", len(feedItems))
						}
						if feedItems[0].Title != "Test Article 1" {
							return nil, fmt.Errorf("expected first item title 'Test Article 1', got %s", feedItems[0].Title)
						}
						if feedItems[1].Title != "Test Article 2" {
							return nil, fmt.Errorf("expected second item title 'Test Article 2', got %s", feedItems[1].Title)
						}
						return integrationRegisterFeedResults("id-1", "id-2"), nil
					}).Times(1)
			},
			wantErr: false,
			validate: func(t *testing.T, err error) {
				if err != nil {
					t.Errorf("Expected successful flow, got error: %v", err)
				}
			},
		},
		{
			name:   "validate_and_fetch_fails",
			rssURL: rssURL,
			mockSetup: func() {
				mockValidateFetch.EXPECT().ValidateAndFetch(ctx, rssURL).Return(nil, fmt.Errorf("network timeout")).Times(1)
			},
			wantErr: true,
			validate: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error for failed validate+fetch")
				}
				if err.Error() != "failed to register RSS feed link" {
					t.Errorf("Expected specific error message, got: %s", err.Error())
				}
			},
		},
		{
			name:   "feed_link_registration_fails",
			rssURL: rssURL,
			mockSetup: func() {
				pf := &domain.ParsedFeed{FeedLink: rssURL, Items: mockFeedItems}
				mockValidateFetch.EXPECT().ValidateAndFetch(ctx, rssURL).Return(pf, nil).Times(1)
				mockRegisterFeedLinkGateway.EXPECT().RegisterFeedLink(ctx, rssURL).Return(fmt.Errorf("database connection failed")).Times(1)
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
			name:   "feed_storage_fails",
			rssURL: rssURL,
			mockSetup: func() {
				pf := &domain.ParsedFeed{FeedLink: rssURL, Items: mockFeedItems}
				mockValidateFetch.EXPECT().ValidateAndFetch(ctx, rssURL).Return(pf, nil).Times(1)
				mockRegisterFeedLinkGateway.EXPECT().RegisterFeedLink(ctx, rssURL).Return(nil).Times(1)
				mockRegisterFeedsGateway.EXPECT().
					RegisterFeeds(ctx, gomock.Any()).
					Return(nil, fmt.Errorf("database write failed")).Times(1)
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
				mockValidateFetch.EXPECT().ValidateAndFetch(ctx, "").Return(nil, fmt.Errorf("RSS feed link cannot be empty")).Times(1)
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
			tt.mockSetup()

			usecase := NewRegisterFeedsUsecase(
				mockValidateFetch,
				mockRegisterFeedLinkGateway,
				mockRegisterFeedsGateway,
				nil,
			)

			err := usecase.Execute(ctx, tt.rssURL)

			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedsUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			tt.validate(t, err)
		})
	}
}

func TestRegisterFeedsUsecase_Execute_RealWorldScenarios(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkGateway := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsGateway := mocks.NewMockRegisterFeedsPort(ctrl)

	t.Run("large_feed_with_many_items", func(t *testing.T) {
		rssURL := "https://news.ycombinator.com/rss"

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

		pf := &domain.ParsedFeed{FeedLink: rssURL, Items: largeFeedItems}
		mockValidateFetch.EXPECT().ValidateAndFetch(ctx, rssURL).Return(pf, nil).Times(1)
		mockRegisterFeedLinkGateway.EXPECT().RegisterFeedLink(ctx, rssURL).Return(nil).Times(1)
		mockRegisterFeedsGateway.EXPECT().
			RegisterFeeds(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, feedItems []*domain.FeedItem) ([]register_feed_port.RegisterFeedResult, error) {
				if len(feedItems) != 100 {
					return nil, fmt.Errorf("expected 100 feed items, got %d", len(feedItems))
				}
				results := make([]register_feed_port.RegisterFeedResult, len(feedItems))
				for i := range feedItems {
					results[i] = register_feed_port.RegisterFeedResult{
						ArticleID: fmt.Sprintf("id-%d", i+1),
						Created:   true,
					}
				}
				return results, nil
			}).Times(1)

		usecase := NewRegisterFeedsUsecase(
			mockValidateFetch,
			mockRegisterFeedLinkGateway,
			mockRegisterFeedsGateway,
			nil,
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
				Title:           "記事タイトル with émojis and <script>tags</script>",
				Description:     "Description with special chars: ñáéíóú & HTML <b>tags</b>",
				Link:            "https://example.com/記事/1",
				Published:       time.Now().Format(time.RFC3339),
				PublishedParsed: time.Now(),
			},
		}

		pf := &domain.ParsedFeed{FeedLink: rssURL, Items: specialFeedItems}
		mockValidateFetch.EXPECT().ValidateAndFetch(ctx, rssURL).Return(pf, nil).Times(1)
		mockRegisterFeedLinkGateway.EXPECT().RegisterFeedLink(ctx, rssURL).Return(nil).Times(1)
		mockRegisterFeedsGateway.EXPECT().
			RegisterFeeds(ctx, gomock.Any()).
			Return(integrationRegisterFeedResults("id-1"), nil).Times(1)

		usecase := NewRegisterFeedsUsecase(
			mockValidateFetch,
			mockRegisterFeedLinkGateway,
			mockRegisterFeedsGateway,
			nil,
		)

		err := usecase.Execute(ctx, rssURL)
		if err != nil {
			t.Errorf("Expected success for special characters feed, got error: %v", err)
		}
	})
}
