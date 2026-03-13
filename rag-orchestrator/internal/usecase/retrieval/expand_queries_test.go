package retrieval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRomanizedJapanese(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		// Romanized Japanese with macron — should be filtered
		{
			name:     "romanized with macron vowels",
			query:    "Tōkyō-to no jinkō suii",
			expected: true,
		},
		{
			name:     "romanized with macron and hyphens",
			query:    "Den-shi kō-gaku gijutsu shōkai",
			expected: true,
		},
		// Romanized Japanese with multiple hyphens — should be filtered
		{
			name:     "romanized with multiple hyphenated syllables",
			query:    "Den-shi Kou-gaku no Shin-gijutsu",
			expected: true,
		},
		// Valid English queries — should NOT be filtered
		{
			name:     "normal english query",
			query:    "artificial intelligence research trends",
			expected: false,
		},
		{
			name:     "english with one hyphenated compound",
			query:    "AI-powered healthcare innovation",
			expected: false,
		},
		{
			name:     "english technology query",
			query:    "Machine Learning Best Practices",
			expected: false,
		},
		// Japanese queries — should NOT be filtered
		{
			name:     "japanese query",
			query:    "人工知能 最新 研究 動向",
			expected: false,
		},
		{
			name:     "mixed japanese english",
			query:    "AI技術 machine learning 最新動向",
			expected: false,
		},
		// Edge cases
		{
			name:     "empty string",
			query:    "",
			expected: false,
		},
		{
			name:     "single word",
			query:    "technology",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRomanizedJapanese(tt.query)
			assert.Equal(t, tt.expected, result, "query: %q", tt.query)
		})
	}
}

func TestFilterExpandedQueries(t *testing.T) {
	tests := []struct {
		name     string
		queries  []string
		expected []string
	}{
		{
			name: "filters romanized queries with macrons",
			queries: []string{
				"人工知能 最新 研究 動向",
				"Jinkō-chinō saishin kenkyū dōkō",
				"artificial intelligence research trends",
				"機械学習 技術 応用",
				"America AI research trends",
				"machine learning technology applications",
				"深層学習 ニューラルネットワーク",
				"Shinsō-gakushū nyūraru nettowāku",
				"deep learning neural networks",
				"自然言語処理 最新動向",
				"NLP latest developments",
				"natural language processing advances",
			},
			expected: []string{
				"人工知能 最新 研究 動向",
				// "Jinkō-chinō ..." filtered (macron ō)
				"artificial intelligence research trends",
				"機械学習 技術 応用",
				"America AI research trends",
				"machine learning technology applications",
				"深層学習 ニューラルネットワーク",
				// "Shinsō-gakushū ..." filtered (macron ō, ū, ā)
				"deep learning neural networks",
				"自然言語処理 最新動向",
				// capped at 8
			},
		},
		{
			name: "caps at maxExpandedQueries",
			queries: []string{
				"query1", "query2", "query3", "query4",
				"query5", "query6", "query7", "query8",
				"query9", "query10",
			},
			expected: []string{
				"query1", "query2", "query3", "query4",
				"query5", "query6", "query7", "query8",
			},
		},
		{
			name:     "empty input",
			queries:  []string{},
			expected: []string{},
		},
		{
			name:     "nil input",
			queries:  nil,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterExpandedQueries(tt.queries)
			assert.Equal(t, tt.expected, result)
		})
	}
}
