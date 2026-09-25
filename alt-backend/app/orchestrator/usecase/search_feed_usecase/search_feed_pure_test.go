package search_feed_usecase

import (
	"alt/domain"
	"testing"
)

func TestConvertHitsToFeedItems(t *testing.T) {
	tests := []struct {
		name     string
		hits     []domain.SearchArticleHit
		urlMap   map[string]string
		expected []*domain.FeedItem
	}{
		{
			name:     "empty hits returns empty slice",
			hits:     []domain.SearchArticleHit{},
			urlMap:   map[string]string{},
			expected: []*domain.FeedItem{},
		},
		{
			name: "converts hits with matching URLs",
			hits: []domain.SearchArticleHit{
				{ID: "art-1", Title: "Title 1", Content: "Content 1"},
				{ID: "art-2", Title: "Title 2", Content: "Content 2"},
			},
			urlMap: map[string]string{
				"art-1": "https://example.com/1",
				"art-2": "https://example.com/2",
			},
			expected: []*domain.FeedItem{
				{ArticleID: "art-1", Title: "Title 1", Description: "Content 1", Link: "https://example.com/1"},
				{ArticleID: "art-2", Title: "Title 2", Description: "Content 2", Link: "https://example.com/2"},
			},
		},
		{
			name: "converts hits with missing URLs to empty links",
			hits: []domain.SearchArticleHit{
				{ID: "art-missing", Title: "Missing", Content: "No URL"},
			},
			urlMap: map[string]string{},
			expected: []*domain.FeedItem{
				{ArticleID: "art-missing", Title: "Missing", Description: "No URL", Link: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertHitsToFeedItems(tt.hits, tt.urlMap)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected len %d, got %d", len(tt.expected), len(got))
			}
			for i := range got {
				if got[i].ArticleID != tt.expected[i].ArticleID {
					t.Errorf("item[%d] ArticleID = %s, want %s", i, got[i].ArticleID, tt.expected[i].ArticleID)
				}
				if got[i].Title != tt.expected[i].Title {
					t.Errorf("item[%d] Title = %s, want %s", i, got[i].Title, tt.expected[i].Title)
				}
				if got[i].Description != tt.expected[i].Description {
					t.Errorf("item[%d] Description = %s, want %s", i, got[i].Description, tt.expected[i].Description)
				}
				if got[i].Link != tt.expected[i].Link {
					t.Errorf("item[%d] Link = %s, want %s", i, got[i].Link, tt.expected[i].Link)
				}
			}
		})
	}
}

func TestHasMoreSearchResults(t *testing.T) {
	tests := []struct {
		name          string
		returnedCount int
		limit         int
		offset        int
		expected      bool
	}{
		{
			name:          "fewer results than limit means no more",
			returnedCount: 5,
			limit:         10,
			offset:        0,
			expected:      false,
		},
		{
			name:          "zero results means no more",
			returnedCount: 0,
			limit:         10,
			offset:        0,
			expected:      false,
		},
		{
			name:          "full page under max cap means more results",
			returnedCount: 20,
			limit:         20,
			offset:        0,
			expected:      true,
		},
		{
			name:          "full page right below max cap means more results",
			returnedCount: 20,
			limit:         20,
			offset:        179,
			expected:      true,
		},
		{
			name:          "full page reaching max cap means no more",
			returnedCount: 20,
			limit:         20,
			offset:        180,
			expected:      false,
		},
		{
			name:          "offset past max cap means no more",
			returnedCount: 20,
			limit:         20,
			offset:        200,
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasMoreSearchResults(tt.returnedCount, tt.limit, tt.offset)
			if got != tt.expected {
				t.Errorf("hasMoreSearchResults(%d, %d, %d) = %v, want %v", tt.returnedCount, tt.limit, tt.offset, got, tt.expected)
			}
		})
	}
}
