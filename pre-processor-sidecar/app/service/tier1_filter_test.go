package service

import (
	"log/slog"
	"strings"
	"testing"

	"pre-processor-sidecar/models"

	"github.com/stretchr/testify/assert"
)

func TestFilterTier1Articles(t *testing.T) {
	logger := slog.Default()
	textOfLength := func(n int) string { return strings.Repeat("a", n) }

	tests := []struct {
		name         string
		articles     []*models.Article
		wantTier1    int
		wantFiltered int
	}{
		{
			name:         "empty input returns empty result",
			articles:     nil,
			wantTier1:    0,
			wantFiltered: 0,
		},
		{
			name: "all tier1 articles pass through",
			articles: []*models.Article{
				{Content: textOfLength(600), ArticleURL: "https://example.com/a1", Title: "Article 1"},
				{Content: textOfLength(800), ArticleURL: "https://example.com/a2", Title: "Article 2"},
			},
			wantTier1:    2,
			wantFiltered: 0,
		},
		{
			name: "all non-tier1 articles are filtered",
			articles: []*models.Article{
				{Content: "short", ArticleURL: "https://example.com/a1", Title: "Short"},
				{Content: textOfLength(300) + "続きをみる", ArticleURL: "https://example.com/a2", Title: "Truncated"},
			},
			wantTier1:    0,
			wantFiltered: 2,
		},
		{
			name: "mixed tier1 and non-tier1",
			articles: []*models.Article{
				{Content: textOfLength(600), ArticleURL: "https://example.com/good1", Title: "Good 1"},
				{Content: "test", ArticleURL: "https://zenn.dev/test", Title: "Placeholder"},
				{Content: textOfLength(1000), ArticleURL: "https://dev.to/article", Title: "Good 2"},
				{Content: textOfLength(100), ArticleURL: "https://example.com/short", Title: "Too Short"},
				{Content: textOfLength(700), ArticleURL: "https://example.com/good3", Title: "Good 3"},
			},
			wantTier1:    3,
			wantFiltered: 2,
		},
		{
			name: "crossword URL filtered even with long content",
			articles: []*models.Article{
				{Content: textOfLength(1000), ArticleURL: "https://theguardian.com/crosswords/cryptic/29847", Title: "Crossword"},
			},
			wantTier1:    0,
			wantFiltered: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterTier1Articles(tt.articles, logger)
			assert.Equal(t, tt.wantTier1, len(result.Tier1), "Tier1 count mismatch")
			assert.Equal(t, tt.wantFiltered, result.Filtered, "Filtered count mismatch")
		})
	}
}
