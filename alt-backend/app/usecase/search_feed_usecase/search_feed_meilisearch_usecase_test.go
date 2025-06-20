package search_feed_usecase

import (
	"alt/domain"
	"alt/driver/models"
	"alt/mocks"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func generateMockSearchArticleHits(amount int) []domain.SearchArticleHit {
	hits := make([]domain.SearchArticleHit, amount)
	for i := 0; i < amount; i++ {
		hits[i] = domain.SearchArticleHit{
			ID:      "123",
			Title:   "Test Article",
			Content: "Test content",
			Tags:    []string{"test"},
		}
	}
	return hits
}

func TestSearchFeedMeilisearchUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name          string
		query         string
		mockResponse  []domain.SearchArticleHit
		mockError     error
		expectedCount int
		expectError   bool
	}{
		{
			name:          "should return 10 search results",
			query:         "test query",
			mockResponse:  generateMockSearchArticleHits(10),
			mockError:     nil,
			expectedCount: 10,
			expectError:   false,
		},
		{
			name:          "should return empty results for empty query",
			query:         "",
			mockResponse:  []domain.SearchArticleHit{},
			mockError:     nil,
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:          "should return empty results when no matches",
			query:         "nonexistent",
			mockResponse:  []domain.SearchArticleHit{},
			mockError:     nil,
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:          "should return error when search service fails",
			query:         "test query",
			mockResponse:  nil,
			mockError:     errors.New("search service unavailable"),
			expectedCount: 0,
			expectError:   true,
		},
		{
			name:          "should handle large result sets",
			query:         "popular query",
			mockResponse:  generateMockSearchArticleHits(100),
			mockError:     nil,
			expectedCount: 100,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSearchPort := mocks.NewMockSearchFeedPort(ctrl)
			mockURLPort := mocks.NewMockFeedURLLinkPort(ctrl)

			mockSearchPort.EXPECT().SearchFeeds(ctx, tt.query).Return(tt.mockResponse, tt.mockError)

			if tt.mockResponse != nil && len(tt.mockResponse) > 0 && tt.mockError == nil {
				articleIDs := make([]string, len(tt.mockResponse))
				for i, hit := range tt.mockResponse {
					articleIDs[i] = hit.ID
				}
				mockURLPort.EXPECT().GetFeedURLsByArticleIDs(ctx, articleIDs).Return([]models.FeedAndArticle{}, nil)
			}

			usecase := NewSearchFeedMeilisearchUsecase(mockSearchPort, mockURLPort)
			results, err := usecase.Execute(ctx, tt.query)

			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if len(results) != 0 {
					t.Fatalf("Expected 0 results on error, got %d", len(results))
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if len(results) != tt.expectedCount {
					t.Fatalf("Expected %d results, got %d", tt.expectedCount, len(results))
				}
			}
		})
	}
}

func TestSearchFeedMeilisearchUsecase_Execute_DataMapping(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("should correctly map search hits to feed items with URLs", func(t *testing.T) {
		expectedHits := []domain.SearchArticleHit{
			{
				ID:      "article-123",
				Title:   "Test Article Title",
				Content: "Test article content",
				Tags:    []string{"tech", "news"},
			},
			{
				ID:      "article-456",
				Title:   "Another Article",
				Content: "Another content",
				Tags:    []string{"science"},
			},
		}

		expectedURLMap := []models.FeedAndArticle{
			{FeedID: "feed-123", ArticleID: "article-123", URL: "https://example1.com/rss"},
			{FeedID: "feed-456", ArticleID: "article-456", URL: "https://example2.com/rss"},
		}

		mockSearchPort := mocks.NewMockSearchFeedPort(ctrl)
		mockURLPort := mocks.NewMockFeedURLLinkPort(ctrl)

		mockSearchPort.EXPECT().SearchFeeds(ctx, "test").Return(expectedHits, nil)
		mockURLPort.EXPECT().GetFeedURLsByArticleIDs(ctx, []string{"article-123", "article-456"}).Return(expectedURLMap, nil)

		usecase := NewSearchFeedMeilisearchUsecase(mockSearchPort, mockURLPort)
		results, err := usecase.Execute(ctx, "test")

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(results))
		}

		// Check first result
		result1 := results[0]
		expected1 := expectedHits[0]
		if result1.Title != expected1.Title {
			t.Errorf("Expected title %s, got %s", expected1.Title, result1.Title)
		}
		if result1.Description != expected1.Content {
			t.Errorf("Expected description %s, got %s", expected1.Content, result1.Description)
		}
		if result1.Link != expectedURLMap[0].URL {
			t.Errorf("Expected link %s, got %s", expectedURLMap[0].URL, result1.Link)
		}

		// Check second result
		result2 := results[1]
		if result2.Link != expectedURLMap[1].URL {
			t.Errorf("Expected link %s, got %s", expectedURLMap[1].URL, result2.Link)
		}

	})

	t.Run("should handle missing URLs gracefully", func(t *testing.T) {
		expectedHits := []domain.SearchArticleHit{
			{
				ID:      "article-123",
				Title:   "Test Article Title",
				Content: "Test article content",
				Tags:    []string{"tech", "news"},
			},
		}

		expectedURLMap := []models.FeedAndArticle{} // No URLs found

		mockSearchPort := mocks.NewMockSearchFeedPort(ctrl)
		mockURLPort := mocks.NewMockFeedURLLinkPort(ctrl)

		mockSearchPort.EXPECT().SearchFeeds(ctx, "test").Return(expectedHits, nil)
		mockURLPort.EXPECT().GetFeedURLsByArticleIDs(ctx, []string{"article-123"}).Return(expectedURLMap, nil)

		usecase := NewSearchFeedMeilisearchUsecase(mockSearchPort, mockURLPort)
		results, err := usecase.Execute(ctx, "test")

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}

		result := results[0]
		if result.Link != "" {
			t.Errorf("Expected empty link when URL not found, got %s", result.Link)
		}
	})
}
