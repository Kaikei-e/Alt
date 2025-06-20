package feed_url_link_gateway

import (
	"alt/driver/models"
	"alt/mocks"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestFeedURLLinkGateway_GetFeedURLsByArticleIDs(t *testing.T) {
	tests := []struct {
		name           string
		articleIDs     []string
		mockSetup      func(*mocks.MockFeedURLLinkDriver)
		expectedURLMap []models.FeedAndArticle
		expectedError  bool
		errorMessage   string
	}{
		{
			name:       "successful URL retrieval for multiple articles",
			articleIDs: []string{"1", "2", "3"},
			mockSetup: func(m *mocks.MockFeedURLLinkDriver) {
				m.EXPECT().
					GetFeedURLsByArticleIDs(gomock.Any(), []string{"1", "2", "3"}).
					Return([]models.FeedAndArticle{
						{FeedID: "1", ArticleID: "1", URL: "https://example1.com/rss"},
						{FeedID: "2", ArticleID: "2", URL: "https://example2.com/rss"},
						{FeedID: "3", ArticleID: "3", URL: "https://example3.com/rss"},
					}, nil)
			},
			expectedURLMap: []models.FeedAndArticle{
				{FeedID: "1", ArticleID: "1", URL: "https://example1.com/rss"},
				{FeedID: "2", ArticleID: "2", URL: "https://example2.com/rss"},
				{FeedID: "3", ArticleID: "3", URL: "https://example3.com/rss"},
			},
			expectedError: false,
		},
		{
			name:       "partial results - some articles not found",
			articleIDs: []string{"1", "999"},
			mockSetup: func(m *mocks.MockFeedURLLinkDriver) {
				m.EXPECT().
					GetFeedURLsByArticleIDs(gomock.Any(), []string{"1", "999"}).
					Return([]models.FeedAndArticle{
						{FeedID: "1", ArticleID: "1", URL: "https://example1.com/rss"},
					}, nil)
			},
			expectedURLMap: []models.FeedAndArticle{
				{FeedID: "1", ArticleID: "1", URL: "https://example1.com/rss"},
			},
			expectedError: false,
		},
		{
			name:       "empty article IDs",
			articleIDs: []string{},
			mockSetup: func(m *mocks.MockFeedURLLinkDriver) {
				m.EXPECT().
					GetFeedURLsByArticleIDs(gomock.Any(), []string{}).
					Return([]models.FeedAndArticle{}, nil)
			},
			expectedURLMap: []models.FeedAndArticle{},
			expectedError:  false,
		},
		{
			name:       "database error",
			articleIDs: []string{"1", "2"},
			mockSetup: func(m *mocks.MockFeedURLLinkDriver) {
				m.EXPECT().
					GetFeedURLsByArticleIDs(gomock.Any(), []string{"1", "2"}).
					Return([]models.FeedAndArticle{}, errors.New("database connection failed"))
			},
			expectedURLMap: []models.FeedAndArticle{},
			expectedError:  true,
			errorMessage:   "database connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDriver := mocks.NewMockFeedURLLinkDriver(ctrl)
			tt.mockSetup(mockDriver)

			gateway := NewFeedURLLinkGateway(mockDriver)

			result, err := gateway.GetFeedURLsByArticleIDs(context.Background(), tt.articleIDs)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedURLMap, result)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURLMap, result)
			}
		})
	}
}
