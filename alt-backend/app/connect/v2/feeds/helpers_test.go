package feeds

import (
	"alt/domain"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeDescription_HTMLEntityDecoding(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"decodes &#39;", "Here&#39;s the news", "Here's the news"},
		{"decodes &amp;", "A &amp; B", "A & B"},
		{"decodes &quot;", "&quot;Hello&quot;", `"Hello"`},
		{"decodes &#x27;", "It&#x27;s fine", "It's fine"},
		{"strips tags and decodes", "<b>It&#39;s</b> &amp; more", "It's & more"},
		{"decodes multiple entities in title", "Here&#39;s what we&#39;re reading", "Here's what we're reading"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDescription(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertFeedsToProto_IDPriority(t *testing.T) {
	tests := []struct {
		name        string
		feed        *domain.FeedItem
		expectedID  string
		expectUUID  bool // when true, assert valid UUID format instead of exact match
	}{
		{
			name: "uses ArticleID when set",
			feed: &domain.FeedItem{
				Link:      "https://example.com/feed",
				ArticleID: "article-456",
			},
			expectedID: "article-456",
		},
		{
			name: "generates UUID when ArticleID is empty",
			feed: &domain.FeedItem{
				Link: "https://example.com/feed",
			},
			expectUUID: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertFeedsToProto([]*domain.FeedItem{tt.feed})
			assert.Len(t, result, 1)
			if tt.expectUUID {
				// UUID v4 format: 8-4-4-4-12 hex chars
				assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, result[0].Id)
			} else {
				assert.Equal(t, tt.expectedID, result[0].Id)
			}
		})
	}
}

func TestConvertFeedsToProto_DuplicateLinksUniqueIDs(t *testing.T) {
	// Simulates the favorites scenario: multiple feed items from the same source
	// have the same Link and no ArticleID — UUID fallback guarantees unique keys.
	feeds := []*domain.FeedItem{
		{Link: "https://example.com/feed", Title: "Article A"},
		{Link: "https://example.com/feed", Title: "Article B"},
		{Link: "https://example.com/feed", Title: "Article C"},
	}

	result := convertFeedsToProto(feeds)
	assert.Len(t, result, 3)

	ids := make(map[string]bool)
	for _, item := range result {
		assert.False(t, ids[item.Id], "duplicate ID found: %s", item.Id)
		ids[item.Id] = true
	}
}
