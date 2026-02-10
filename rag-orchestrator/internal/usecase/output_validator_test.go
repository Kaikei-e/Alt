package usecase_test

import (
	"testing"

	"rag-orchestrator/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputValidator_Validate_EscapeSequences(t *testing.T) {
	validator := usecase.NewOutputValidator()

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
	validator := usecase.NewOutputValidator()

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
	validator := usecase.NewOutputValidator()

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
	validator := usecase.NewOutputValidator()

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
	validator := usecase.NewOutputValidator()

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
	validator := usecase.NewOutputValidator()

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

func TestOutputValidator_ConvertLiteralEscapes_OnlyNewlines(t *testing.T) {
	// Test that convertLiteralEscapes only converts \n, not \t or \r
	// This is important to avoid breaking paths like C:\temp
	validator := usecase.NewOutputValidator()

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
