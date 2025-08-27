package service

import (
	"testing"

	"pre-processor-sidecar/models"

	"github.com/stretchr/testify/assert"
)

func TestSubscriptionSyncService_IsSubscriptionChanged(t *testing.T) {
	tests := map[string]struct {
		existing *models.Subscription
		incoming *models.Subscription
		expected bool
	}{
		"no_changes": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			expected: false,
		},
		"title_changed": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Updated Tech News",
				Category:    "Technology",
			},
			expected: true,
		},
		"category_changed": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Science",
			},
			expected: true,
		},
		"url_changed": {
			existing: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://example.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			incoming: &models.Subscription{
				InoreaderID: "feed/http://example.com/rss",
				FeedURL:     "http://newexample.com/rss",
				Title:       "Tech News",
				Category:    "Technology",
			},
			expected: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			syncService := &SubscriptionSyncService{}
			result := syncService.IsSubscriptionChanged(tc.existing, tc.incoming)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSubscriptionSyncService_GetSyncStats(t *testing.T) {
	syncService := NewSubscriptionSyncService(nil, nil, nil)

	// Check initial state
	stats := syncService.GetSyncStats()
	assert.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.TotalSyncs)
	assert.Equal(t, int64(0), stats.SuccessfulSyncs)
	assert.Equal(t, int64(0), stats.FailedSyncs)
}

// Tests use proper basic testing without complex mocking dependencies