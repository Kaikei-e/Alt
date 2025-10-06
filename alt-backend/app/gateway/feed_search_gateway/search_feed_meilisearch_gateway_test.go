package feed_search_gateway

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestSearchFeedMeilisearchGateway_SearchFeeds(t *testing.T) {
	// Initialize logger to prevent nil pointer issues
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		query         string
		driverHits    []domain.SearchIndexerArticleHit
		driverError   error
		expectedCount int
		expectError   bool
	}{
		{
			name:  "should return mapped search results",
			query: "test query",
			driverHits: []domain.SearchIndexerArticleHit{
				{
					ID:      "article-123",
					Title:   "Test Article",
					Content: "Test content",
					Tags:    []string{"tech", "news"},
				},
			},
			driverError:   nil,
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "should return empty results",
			query:         "nonexistent",
			driverHits:    []domain.SearchIndexerArticleHit{},
			driverError:   nil,
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:          "should return error when driver fails",
			query:         "test",
			driverHits:    nil,
			driverError:   errors.New("search service unavailable"),
			expectedCount: 0,
			expectError:   true,
		},
		{
			name:  "should handle multiple results",
			query: "popular",
			driverHits: []domain.SearchIndexerArticleHit{
				{
					ID:      "article-1",
					Title:   "Article 1",
					Content: "Content 1",
					Tags:    []string{"tag1"},
				},
				{
					ID:      "article-2",
					Title:   "Article 2",
					Content: "Content 2",
					Tags:    []string{"tag2"},
				},
			},
			driverError:   nil,
			expectedCount: 2,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDriver := mocks.NewMockSearchIndexerPort(ctrl)

			userID := uuid.New()
			ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
				UserID:    userID,
				Email:     "user@example.com",
				Role:      domain.UserRoleUser,
				TenantID:  uuid.New(),
				SessionID: "session-123",
				LoginAt:   time.Now().Add(-time.Minute),
				ExpiresAt: time.Now().Add(time.Hour),
			})

			mockDriver.EXPECT().SearchArticles(ctx, tt.query, userID.String()).Return(tt.driverHits, tt.driverError)

			gateway := NewSearchFeedMeilisearchGateway(mockDriver)
			results, err := gateway.SearchFeeds(ctx, tt.query)

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

				if len(results) > 0 && len(tt.driverHits) > 0 {
					result := results[0]
					expected := tt.driverHits[0]

					if result.ID != expected.ID {
						t.Errorf("Expected ID %s, got %s", expected.ID, result.ID)
					}
					if result.Title != expected.Title {
						t.Errorf("Expected title %s, got %s", expected.Title, result.Title)
					}
					if result.Content != expected.Content {
						t.Errorf("Expected content %s, got %s", expected.Content, result.Content)
					}
				}
			}
		})
	}
}

func TestSearchFeedMeilisearchGateway_SearchFeeds_EmptyQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := uuid.New()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "user@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "session-456",
		LoginAt:   time.Now().Add(-time.Minute),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	mockDriver := mocks.NewMockSearchIndexerPort(ctrl)
	mockDriver.EXPECT().SearchArticles(ctx, "", userID.String()).Return([]domain.SearchIndexerArticleHit{}, nil)

	gateway := NewSearchFeedMeilisearchGateway(mockDriver)
	results, err := gateway.SearchFeeds(ctx, "")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("Expected 0 results, got %d", len(results))
	}
}
