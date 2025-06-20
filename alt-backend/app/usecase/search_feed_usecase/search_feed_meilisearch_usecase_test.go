package search_feed_usecase

import (
	"alt/domain"
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
			mockSearchPort.EXPECT().SearchFeeds(ctx, tt.query).Return(tt.mockResponse, tt.mockError)

			usecase := NewSearchFeedMeilisearchUsecase(mockSearchPort)
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

	t.Run("should correctly map search hits to feed items", func(t *testing.T) {
		expectedHits := []domain.SearchArticleHit{
			{
				ID:      "article-123",
				Title:   "Test Article Title",
				Content: "Test article content",
				Tags:    []string{"tech", "news"},
			},
		}

		mockSearchPort := mocks.NewMockSearchFeedPort(ctrl)
		mockSearchPort.EXPECT().SearchFeeds(ctx, "test").Return(expectedHits, nil)

		usecase := NewSearchFeedMeilisearchUsecase(mockSearchPort)
		results, err := usecase.Execute(ctx, "test")

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}

		result := results[0]
		expected := expectedHits[0]

		if result.Title != expected.Title {
			t.Errorf("Expected title %s, got %s", expected.Title, result.Title)
		}
		if result.Description != expected.Content {
			t.Errorf("Expected description %s, got %s", expected.Content, result.Description)
		}
		if result.Link != "" {
			t.Error("Expected link to be empty since search-indexer doesn't provide URLs")
		}
	})
}