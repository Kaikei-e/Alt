package usecase_test

import (
	"strings"
	"testing"

	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputValidator_Validate_EscapeSequences(t *testing.T) {
	validator := usecase.NewOutputValidator(0)

	tests := []struct {
		name           string
		input          string
		expectedAnswer string
		description    string
	}{
		{
			name: "newline escape sequence",
			input: `{
				"answer": "Line 1\nLine 2\nLine 3",
				"citations": [],
				"fallback": false,
				"reason": ""
			}`,
			expectedAnswer: "Line 1\nLine 2\nLine 3",
			description:    "JSON \\n should be converted to actual newlines",
		},
		{
			name: "tab escape sequence",
			input: `{
				"answer": "Column1\tColumn2\tColumn3",
				"citations": [],
				"fallback": false,
				"reason": ""
			}`,
			expectedAnswer: "Column1\tColumn2\tColumn3",
			description:    "JSON \\t should be converted to actual tabs",
		},
		{
			name: "carriage return escape sequence",
			input: `{
				"answer": "Line 1\r\nLine 2",
				"citations": [],
				"fallback": false,
				"reason": ""
			}`,
			expectedAnswer: "Line 1\r\nLine 2",
			description:    "JSON \\r\\n should be converted to actual CRLF",
		},
		{
			name: "escaped quote",
			input: `{
				"answer": "He said \"Hello\"",
				"citations": [],
				"fallback": false,
				"reason": ""
			}`,
			expectedAnswer: "He said \"Hello\"",
			description:    "JSON \\\" should be converted to actual quotes",
		},
		{
			name: "escaped backslash",
			input: `{
				"answer": "Path: C:\\Users\\test",
				"citations": [],
				"fallback": false,
				"reason": ""
			}`,
			expectedAnswer: "Path: C:\\Users\\test",
			description:    "JSON \\\\ should be converted to single backslash",
		},
		{
			name: "markdown with newlines",
			input: `{
				"answer": "## Heading\n\n### Subheading\n\n- Item 1\n- Item 2",
				"citations": [],
				"fallback": false,
				"reason": ""
			}`,
			expectedAnswer: "## Heading\n\n### Subheading\n\n- Item 1\n- Item 2",
			description:    "Markdown headings and lists should have proper newlines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.Validate(tt.input, nil)
			require.NoError(t, err, "Validate should not return error")
			assert.Equal(t, tt.expectedAnswer, result.Answer, tt.description)
		})
	}
}

func TestExtractAnswerOnly_EscapeSequences(t *testing.T) {
	// This tests the fallback extraction path when JSON parsing fails
	validator := usecase.NewOutputValidator(0)

	// Truncated JSON that would trigger extractAnswerOnly
	tests := []struct {
		name           string
		input          string
		expectedAnswer string
		description    string
	}{
		{
			name:           "truncated json with newlines",
			input:          `{"answer": "Line 1\nLine 2\nLine 3", "citations": [`,
			expectedAnswer: "Line 1\nLine 2\nLine 3",
			description:    "extractAnswerOnly should properly unescape \\n",
		},
		{
			name:           "truncated json with tabs",
			input:          `{"answer": "Col1\tCol2", "fallback":`,
			expectedAnswer: "Col1\tCol2",
			description:    "extractAnswerOnly should properly unescape \\t",
		},
		{
			name:           "truncated json with escaped quotes",
			input:          `{"answer": "He said \"Hi\"", "other":`,
			expectedAnswer: "He said \"Hi\"",
			description:    "extractAnswerOnly should properly unescape \\\"",
		},
		{
			name:           "truncated json with backslash",
			input:          `{"answer": "C:\\path", "x":`,
			expectedAnswer: "C:\\path",
			description:    "extractAnswerOnly should properly unescape \\\\",
		},
		{
			name:           "truncated json with literal backslash-n",
			input:          `{"answer": "Line 1\\nLine 2", "x":`,
			expectedAnswer: "Line 1\nLine 2",
			description:    "extractAnswerOnly + convertLiteralEscapes should convert literal \\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.Validate(tt.input, nil)
			require.NoError(t, err, "Validate should recover from truncated JSON")
			assert.Equal(t, tt.expectedAnswer, result.Answer, tt.description)
		})
	}
}

func TestOutputValidator_Validate_LiteralBackslashN(t *testing.T) {
	// Test that literal \n in model output (not JSON escape) gets converted
	validator := usecase.NewOutputValidator(0)

	// This simulates what GPT-OSS might output - literal backslash-n in the text
	// Note: In Go raw string literal, \\n represents literal \n (two characters)
	input := `{
		"answer": "## Heading\\n\\n### Subheading\\n\\n- Item 1\\n- Item 2",
		"citations": [],
		"fallback": false,
		"reason": ""
	}`

	result, err := validator.Validate(input, nil)
	require.NoError(t, err)

	// After post-processing, literal \n should be converted to actual newlines
	expected := "## Heading\n\n### Subheading\n\n- Item 1\n- Item 2"
	assert.Equal(t, expected, result.Answer, "Literal \\n in model output should be converted to actual newlines")
}

func TestOutputValidator_Validate_EmptyAnswerRejection(t *testing.T) {
	validator := usecase.NewOutputValidator(0)

	// Empty answer without fallback flag should be rejected
	input := `{
		"answer": "",
		"citations": [],
		"fallback": false,
		"reason": ""
	}`

	result, err := validator.Validate(input, nil)
	assert.Error(t, err, "should reject empty answer without fallback")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "empty answer without fallback")
}

func TestOutputValidator_Validate_EmptyAnswerWithFallback(t *testing.T) {
	validator := usecase.NewOutputValidator(0)

	// Empty answer WITH fallback flag is valid
	input := `{
		"answer": "",
		"citations": [],
		"fallback": true,
		"reason": "insufficient context"
	}`

	result, err := validator.Validate(input, nil)
	assert.NoError(t, err, "empty answer with fallback=true should be valid")
	assert.True(t, result.Fallback)
}

func TestOutputValidator_Validate_WhitespaceOnlyAnswerRejection(t *testing.T) {
	validator := usecase.NewOutputValidator(0)

	// Whitespace-only answer should also be rejected
	input := `{
		"answer": "   \n  \n  ",
		"citations": [],
		"fallback": false,
		"reason": ""
	}`

	result, err := validator.Validate(input, nil)
	assert.Error(t, err, "should reject whitespace-only answer without fallback")
	assert.Nil(t, result)
}

func TestOutputValidator_Validate_IndexBasedCitations(t *testing.T) {
	validator := usecase.NewOutputValidator(0)

	contexts := []usecase.ContextItem{
		{ChunkText: "chunk1 text", Title: "Article 1"},
		{ChunkText: "chunk2 text", Title: "Article 2"},
		{ChunkText: "chunk3 text", Title: "Article 3"},
	}
	// Set ChunkIDs (UUIDs won't match "1", "2", "3")
	for i := range contexts {
		contexts[i].ChunkID = uuid.New()
	}

	input := `{
		"answer": "Some answer referencing [1] and [2].",
		"citations": [
			{"chunk_id": "1", "reason": "main source"},
			{"chunk_id": "2", "reason": "supporting source"}
		],
		"fallback": false,
		"reason": ""
	}`

	result, err := validator.Validate(input, contexts)
	require.NoError(t, err)
	assert.False(t, result.Fallback)
	assert.Len(t, result.Citations, 2, "index-based citations (1, 2) should be preserved")
	assert.Equal(t, "1", result.Citations[0].ChunkID)
	assert.Equal(t, "2", result.Citations[1].ChunkID)
}

func TestOutputValidator_Validate_IndexBasedCitationsOutOfRange(t *testing.T) {
	validator := usecase.NewOutputValidator(0)

	contexts := []usecase.ContextItem{
		{ChunkText: "chunk1 text", Title: "Article 1"},
	}
	contexts[0].ChunkID = uuid.New()

	input := `{
		"answer": "Answer with [1] and [99].",
		"citations": [
			{"chunk_id": "1", "reason": "valid"},
			{"chunk_id": "99", "reason": "out of range"}
		],
		"fallback": false,
		"reason": ""
	}`

	result, err := validator.Validate(input, contexts)
	require.NoError(t, err)
	assert.Len(t, result.Citations, 1, "only valid index citation should be kept")
	assert.Equal(t, "1", result.Citations[0].ChunkID)
}

func TestOutputValidator_Validate_ShortAnswerFlag(t *testing.T) {
	validator := usecase.NewOutputValidator(800)

	// 799 runes — should be flagged as short
	shortAnswer := strings.Repeat("あ", 799)
	input := `{"answer": "` + shortAnswer + `", "citations": [], "fallback": false, "reason": ""}`

	result, err := validator.Validate(input, nil)
	require.NoError(t, err)
	assert.True(t, result.ShortAnswer, "answer with 799 runes should be flagged as short")
}

func TestOutputValidator_Validate_LongAnswerNoFlag(t *testing.T) {
	validator := usecase.NewOutputValidator(800)

	// Exactly 800 runes — should NOT be flagged
	longAnswer := strings.Repeat("あ", 800)
	input := `{"answer": "` + longAnswer + `", "citations": [], "fallback": false, "reason": ""}`

	result, err := validator.Validate(input, nil)
	require.NoError(t, err)
	assert.False(t, result.ShortAnswer, "answer with 800 runes should not be flagged as short")
}

func TestOutputValidator_Validate_RuneCountNotByteCount(t *testing.T) {
	validator := usecase.NewOutputValidator(100)

	// 100 Japanese characters = 300 bytes but 100 runes
	japaneseText := strings.Repeat("日", 100)
	input := `{"answer": "` + japaneseText + `", "citations": [], "fallback": false, "reason": ""}`

	result, err := validator.Validate(input, nil)
	require.NoError(t, err)
	assert.False(t, result.ShortAnswer, "should count runes not bytes; 100 Japanese chars = 100 runes >= 100 min")
}

func TestOutputValidator_ConvertLiteralEscapes_OnlyNewlines(t *testing.T) {
	// Test that convertLiteralEscapes only converts \n, not \t or \r
	// This is important to avoid breaking paths like C:\temp
	validator := usecase.NewOutputValidator(0)

	// Model outputs text with literal \n but also \t should not be converted
	// Note: We use \\n in Go string to represent literal backslash-n (2 chars)
	// And \t in Go string is an actual tab character (from JSON parsing)
	input := `{
		"answer": "Line 1\\nLine 2 with tab:\there",
		"citations": [],
		"fallback": false,
		"reason": ""
	}`

	result, err := validator.Validate(input, nil)
	require.NoError(t, err)

	// \n should be converted to newline, but \t in JSON stays as tab (standard JSON behavior)
	// We don't double-convert things
	expected := "Line 1\nLine 2 with tab:\there"
	assert.Equal(t, expected, result.Answer, "Only literal \\n should be converted, not \\t")
}

// --- Phase 4: Answer Quality Checks ---

func TestAssessAnswerQuality_CoverageCheck(t *testing.T) {
	flags := usecase.AssessAnswerQuality(
		"This answer discusses AI and machine learning trends",
		"What are the latest AI trends and cybersecurity news?",
		[]usecase.LLMCitation{{ChunkID: "1"}},
		usecase.IntentGeneral,
	)
	// "cybersecurity" not covered in answer
	assert.Contains(t, flags, "low_keyword_coverage")
}

func TestAssessAnswerQuality_GoodCoverage(t *testing.T) {
	flags := usecase.AssessAnswerQuality(
		"AI trends include transformers and large language models. Cybersecurity news covers ransomware.",
		"AI trends and cybersecurity news",
		[]usecase.LLMCitation{{ChunkID: "1"}, {ChunkID: "2"}},
		usecase.IntentGeneral,
	)
	assert.NotContains(t, flags, "low_keyword_coverage")
}

func TestAssessAnswerQuality_CitationDensity(t *testing.T) {
	// Long answer with no citations
	longAnswer := strings.Repeat("This is a detailed answer about technology. ", 50)
	flags := usecase.AssessAnswerQuality(
		longAnswer,
		"tech question",
		nil, // no citations
		usecase.IntentGeneral,
	)
	assert.Contains(t, flags, "low_citation_density")
}

func TestAssessAnswerQuality_CoherenceCheck(t *testing.T) {
	// Answer that doesn't end with sentence-ending punctuation
	flags := usecase.AssessAnswerQuality(
		"This answer is truncated and does not end properly with",
		"test query",
		[]usecase.LLMCitation{{ChunkID: "1"}},
		usecase.IntentGeneral,
	)
	assert.Contains(t, flags, "incoherent_ending")
}

func TestAssessAnswerQuality_CoherentEnding(t *testing.T) {
	flags := usecase.AssessAnswerQuality(
		"This is a complete answer about the topic。",
		"test query",
		[]usecase.LLMCitation{{ChunkID: "1"}},
		usecase.IntentGeneral,
	)
	assert.NotContains(t, flags, "incoherent_ending")
}

func TestAssessAnswerQuality_FactCheckNeedsEvidence(t *testing.T) {
	flags := usecase.AssessAnswerQuality(
		"Yes, that is true。",
		"Is it true that AI can do X?",
		[]usecase.LLMCitation{{ChunkID: "1"}},
		usecase.IntentFactCheck,
	)
	assert.Contains(t, flags, "fact_check_missing_evidence")
}

func TestAssessAnswerQuality_FactCheckHasEvidence(t *testing.T) {
	flags := usecase.AssessAnswerQuality(
		"根拠として、最新の研究では... したがって事実です。",
		"AIがXできるのは本当？",
		[]usecase.LLMCitation{{ChunkID: "1"}},
		usecase.IntentFactCheck,
	)
	assert.NotContains(t, flags, "fact_check_missing_evidence")
}

// --- Phase 0: Article-scoped keyword coverage fix ---

func TestAssessAnswerQuality_ArticleScopedJapaneseAnswer_ShouldNotFlagLowCoverage(t *testing.T) {
	// Reproduces production bug: article-scoped query includes English article title,
	// but answer is in Japanese. The keyword coverage check was comparing English
	// title words against Japanese answer text, causing false "low_keyword_coverage".
	answer := "2026年以降、サプライチェーン攻撃はソフトウェア組織を標的とする攻撃の最有力手段となることが予想されます。" +
		"特にCI/CDパイプラインは依然として脆弱であり、攻撃の侵入経路が多く存在します。" +
		"SolarWinds攻撃やLog4Shell、XZ Utilsのバックドアといった過去の事例は、サプライチェーン攻撃の脅威を明確に示しています。"
	query := "Regarding the article: Supply Chain Security for Developers: Protecting Your CI/CD Pipeline in 2026 - DEV Community [articleId: 7a68810e-c4bc-43ff-8ab8-b42eca504531]\n\nQuestion:\n今後の業界への影響は？"

	flags := usecase.AssessAnswerQuality(
		answer,
		query,
		[]usecase.LLMCitation{{ChunkID: "1"}, {ChunkID: "2"}},
		usecase.IntentArticleScoped,
	)
	assert.NotContains(t, flags, "low_keyword_coverage",
		"article-scoped query with English title + Japanese answer should not trigger low_keyword_coverage")
}

func TestAssessAnswerQuality_ArticleScopedJapaneseQuery_ShouldCheckUserQuestionOnly(t *testing.T) {
	// Japanese article title + Japanese question + Japanese answer.
	// Keywords should come from user question, not article metadata.
	answer := "この記事の主張に対して、性弱説という概念は個人の責任を軽視する可能性があります。" +
		"単に仕組みを疑うだけでなく、個人の能力開発も重要です。"
	query := "Regarding the article: 何度注意しても部下が同じミスを繰り返す原因　根性論に頼らない「性弱説的マネジメント」による指導方法 | ログミーBusiness [articleId: a320697f-ecd9-4adb-8efd-bd20dcd6d20c]\n\nQuestion:\n反論はある？"

	flags := usecase.AssessAnswerQuality(
		answer,
		query,
		[]usecase.LLMCitation{{ChunkID: "1"}},
		usecase.IntentArticleScoped,
	)
	assert.NotContains(t, flags, "low_keyword_coverage",
		"should check user question keywords, not article title keywords")
}

func TestCheckKeywordCoverage_GeneralQueryStillWorks(t *testing.T) {
	// Non-article-scoped query should work as before
	flags := usecase.AssessAnswerQuality(
		"The latest cybersecurity trends include ransomware and supply chain attacks.",
		"What are the latest cybersecurity trends",
		[]usecase.LLMCitation{{ChunkID: "1"}},
		usecase.IntentGeneral,
	)
	assert.NotContains(t, flags, "low_keyword_coverage",
		"general query with matching keywords should pass")
}
