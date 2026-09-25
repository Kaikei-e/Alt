package feeds

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSSEData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extracts plain JSON string",
			input:    "data: \"Hello World\"\n",
			expected: "Hello World",
		},
		{
			name:     "decodes escaped Unicode characters",
			input:    "data: \"2025\\u5e74\\u306e\\u30cb\\u30e5\\u30fc\\u30b9\"\n",
			expected: "2025年のニュース",
		},
		{
			name:     "handles multiple data lines with Unicode",
			input:    "data: \"\\u3053\\u3093\\u306b\\u3061\\u306f\"\ndata: \"\\u4e16\\u754c\"\n",
			expected: "こんにちは世界",
		},
		{
			name:     "handles non-JSON content as fallback",
			input:    "data: plain text\n",
			expected: "plain text",
		},
		{
			name:     "handles empty data",
			input:    "data: \n",
			expected: "",
		},
		{
			name:     "ignores non-data lines",
			input:    "event: message\ndata: \"Test\"\nid: 123\n",
			expected: "Test",
		},
		{
			name:     "handles mixed JSON and non-JSON",
			input:    "data: \"JSON string\"\ndata: plain text\n",
			expected: "JSON stringplain text",
		},
		{
			name:     "handles empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSSEData(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
