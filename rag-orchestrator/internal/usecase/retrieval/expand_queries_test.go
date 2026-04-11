package retrieval

import (
	"strings"
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

// --- Phase 2 RED tests ---

func TestFilterExpandedQueries_AllIdentical_DeduplicatesToOneThenRejectsIfOnlyLeak(t *testing.T) {
	// The exact failure from production: all expanded queries are the same instruction echo
	queries := []string{
		"Japanese queries and English queries must be translated to each other.",
		"Japanese queries and English queries must be translated to each other.",
		"Japanese queries and English queries must be translated to each other.",
		"Japanese queries and English queries must be translated to each other.",
	}
	result := filterExpandedQueries(queries)
	assert.Empty(t, result, "All-identical instruction echoes should be completely filtered")
}

func TestFilterExpandedQueries_InstructionEcho_Filtered(t *testing.T) {
	queries := []string{
		"Output Japanese queries first, then English queries.",
		"Do not add numbering, bullets, labels, or explanations.",
		"サプライチェーン 混乱 原因",
		"global supply chain disruption causes",
	}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"サプライチェーン 混乱 原因", "global supply chain disruption causes"}, result)
}

func TestFilterExpandedQueries_DuplicateRemoval_PreservesOrder(t *testing.T) {
	queries := []string{
		"global supply chain disruption",
		"global supply chain disruption",
		"global supply chain disruption causes",
		"Global Supply Chain Disruption", // case-insensitive dup
	}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"global supply chain disruption", "global supply chain disruption causes"}, result)
}

func TestFilterExpandedQueries_TooShort_Filtered(t *testing.T) {
	queries := []string{"ab", "global supply chain disruption", "x", "OK"}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"global supply chain disruption"}, result)
}

func TestFilterExpandedQueries_TooLong_Filtered(t *testing.T) {
	longQuery := strings.Repeat("a", 201)
	queries := []string{longQuery, "global supply chain disruption"}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"global supply chain disruption"}, result)
}

func TestFilterExpandedQueries_XMLTagLeak_Filtered(t *testing.T) {
	queries := []string{
		"</example>",
		"<input>イランの石油危機はなぜ起きた？</input>",
		"global supply chain disruption causes",
	}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"global supply chain disruption causes"}, result)
}

func TestFilterExpandedQueries_DateOnly_Filtered(t *testing.T) {
	queries := []string{
		"2026-04-07",
		"2026/03/15",
		"global supply chain disruption causes",
	}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"global supply chain disruption causes"}, result)
}

func TestFilterExpandedQueries_DateOnly_Various(t *testing.T) {
	assert.True(t, isDateOnly("2026-04-07"))
	assert.True(t, isDateOnly("2026/03/15"))
	assert.True(t, isDateOnly("2026.01.01"))
	assert.False(t, isDateOnly("2026年のAI動向"))
	assert.False(t, isDateOnly("global supply chain disruption 2026"))
	assert.False(t, isDateOnly(""))
}

func TestIsXMLTagLeak(t *testing.T) {
	assert.True(t, isXMLTagLeak("</example>"))
	assert.True(t, isXMLTagLeak("<input>something</input>"))
	assert.True(t, isXMLTagLeak("<task>Generate queries</task>"))
	assert.False(t, isXMLTagLeak("global supply chain disruption"))
	assert.False(t, isXMLTagLeak("サプライチェーン 混乱 原因"))
	assert.False(t, isXMLTagLeak(""))
}

func TestIsInstructionLeak_KnownMetaPatterns(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "exact production echo",
			query:    "Japanese queries and English queries must be translated to each other.",
			expected: true,
		},
		{
			name:     "output instruction echo",
			query:    "Output ONLY the generated queries, one per line.",
			expected: true,
		},
		{
			name:     "generate instruction echo",
			query:    "Generate exactly 3 English query variations.",
			expected: true,
		},
		{
			name:     "Japanese queries first echo",
			query:    "Japanese queries first, then English queries.",
			expected: true,
		},
		{
			name:     "real Japanese query",
			query:    "サプライチェーン 混乱 原因",
			expected: false,
		},
		{
			name:     "real English query",
			query:    "global supply chain disruption causes and background",
			expected: false,
		},
		{
			name:     "query about translation",
			query:    "machine translation quality improvement",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInstructionLeak(tt.query)
			assert.Equal(t, tt.expected, result, "query: %q", tt.query)
		})
	}
}

func TestIsGarbagePattern(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{"smiley repeats", ":):):):):):):):):):)", true},
		{"dot repeats", "..............................", true},
		{"ha repeats", "hahahahahahahahahaha", true},
		{"ab repeats", "ababababababab", true},
		{"real query Japanese", "最新のAI技術動向", false},
		{"real query English", "latest AI technology trends", false},
		{"short string", "abc", false},
		{"empty", "", false},
		{"mixed with real text", ":) this is a real query", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGarbagePattern(tt.query)
			assert.Equal(t, tt.expected, result, "query: %q", tt.query)
		})
	}
}

func TestFilterExpandedQueries_ConversationMessageLeak_Filtered(t *testing.T) {
	queries := []string{
		"assistant: Hello! I m Augur. Ask me anything about your RSS feeds.",
		"user: サプライチェーンの混乱の原因は？",
		"global supply chain disruption causes",
		"サプライチェーン 混乱 原因",
	}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"global supply chain disruption causes", "サプライチェーン 混乱 原因"}, result)
}

func TestFilterExpandedQueries_AssistantPrefix_Filtered(t *testing.T) {
	queries := []string{
		"assistant: 前回の回答では...",
		"assistant:Hello! I'm Augur.",
		"global supply chain disruption",
	}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"global supply chain disruption"}, result)
}

func TestFilterExpandedQueries_GarbagePattern_Filtered(t *testing.T) {
	queries := []string{
		":):):):):):):):):):):):):):):):):):):):):):):):):):):):):):):",
		"latest AI technology trends",
	}
	result := filterExpandedQueries(queries)
	assert.Equal(t, []string{"latest AI technology trends"}, result)
}

func TestFilterSearchQueries_PreservesValidPlannerQueries(t *testing.T) {
	queries := []string{
		"ヴァンス副大統領 最新動向",
		"JD Vance vice president recent activities",
		"Vance policy changes 2026",
		"US vice president Vance news",
	}
	result := FilterSearchQueries(queries, "ヴァンス副大統領の直近の動きは？")
	assert.Equal(t, queries, result)
}

func TestFilterSearchQueries_FallbackToResolvedQuery(t *testing.T) {
	// When all planner queries are filtered (e.g. all garbage), fall back to resolvedQuery
	queries := []string{
		"2026-04-07",
		":):):):):):):):):)",
		"",
	}
	result := FilterSearchQueries(queries, "ヴァンス副大統領の直近の動きは？")
	assert.Equal(t, []string{"ヴァンス副大統領の直近の動きは？"}, result)
}

func TestFilterSearchQueries_EmptyResolvedQueryNoFallback(t *testing.T) {
	queries := []string{"2026-04-07"}
	result := FilterSearchQueries(queries, "")
	assert.Empty(t, result)
}
