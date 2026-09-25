package summarization

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSSESummary(t *testing.T) {
	tests := []struct {
		name    string
		sseData string
		want    string
	}{
		{
			name:    "empty string",
			sseData: "",
			want:    "",
		},
		{
			name:    "plain string without data prefix returns unchanged",
			sseData: "This is a plain summary text.",
			want:    "This is a plain summary text.",
		},
		{
			name:    "single JSON string chunk",
			sseData: "data: \"Hello, world!\"\n\n",
			want:    "Hello, world!",
		},
		{
			name:    "multiple JSON string chunks concatenated",
			sseData: "data: \"Hello, \"\ndata: \"world!\"\n\n",
			want:    "Hello, world!",
		},
		{
			name:    "plain text data fallback when not valid JSON string",
			sseData: "data: raw chunk text\n\n",
			want:    "raw chunk text",
		},
		{
			name:    "mixed whitespace and comments ignored",
			sseData: ": comment\n\ndata: \"first \"\n\ndata: \"second\"\n",
			want:    "first second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSSESummary(tt.sseData)
			assert.Equal(t, tt.want, got)
		})
	}
}
