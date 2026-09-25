package fetch_article_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	logger.InitLogger()
	os.Exit(m.Run())
}

func TestShouldUseCachedArticle(t *testing.T) {
	tests := []struct {
		name     string
		article  *domain.ArticleContent
		expected bool
	}{
		{
			name:     "nil article returns false",
			article:  nil,
			expected: false,
		},
		{
			name:     "empty content returns false",
			article:  &domain.ArticleContent{Content: ""},
			expected: false,
		},
		{
			name:     "whitespace only returns false",
			article:  &domain.ArticleContent{Content: "   \n\t  "},
			expected: false,
		},
		{
			name:     "content shorter than 100 characters returns false",
			article:  &domain.ArticleContent{Content: strings.Repeat("a", 99)},
			expected: false,
		},
		{
			name:     "content exactly 100 characters returns true",
			article:  &domain.ArticleContent{Content: strings.Repeat("a", 100)},
			expected: true,
		},
		{
			name:     "content longer than 100 characters returns true",
			article:  &domain.ArticleContent{Content: strings.Repeat("a", 101)},
			expected: true,
		},
		{
			name:     "content with padding meeting 100 trimmed characters returns true",
			article:  &domain.ArticleContent{Content: "  " + strings.Repeat("a", 100) + "  "},
			expected: true,
		},
		{
			name:     "content with padding below 100 trimmed characters returns false",
			article:  &domain.ArticleContent{Content: "  " + strings.Repeat("a", 99) + "  "},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseCachedArticle(tt.article)
			if got != tt.expected {
				t.Errorf("shouldUseCachedArticle() = %v, want %v", got, tt.expected)
			}
		})
	}
}
