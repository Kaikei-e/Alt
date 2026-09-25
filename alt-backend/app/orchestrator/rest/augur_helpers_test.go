package rest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractLastUserQuery(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		want     string
	}{
		{
			name:     "empty slice",
			messages: nil,
			want:     "",
		},
		{
			name: "no user message",
			messages: []ChatMessage{
				{Role: "system", Content: "you are helpful"},
				{Role: "assistant", Content: "how can I help?"},
			},
			want: "",
		},
		{
			name: "single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "what is Go?"},
			},
			want: "what is Go?",
		},
		{
			name: "multiple user messages returns last",
			messages: []ChatMessage{
				{Role: "user", Content: "first question"},
				{Role: "assistant", Content: "first answer"},
				{Role: "user", Content: "second question"},
			},
			want: "second question",
		},
		{
			name: "user message with empty content",
			messages: []ChatMessage{
				{Role: "user", Content: ""},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLastUserQuery(tt.messages)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeMetaEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
		omits    []string
		exact    string
	}{
		{
			name:  "strips sensitive fields from contexts and debug",
			input: "event:meta\ndata:{\"Contexts\":[{\"ChunkText\":\"secret text\",\"URL\":\"https://example.com/art1\",\"Title\":\"Title 1\",\"PublishedAt\":\"2026-01-01\",\"Score\":0.95,\"DocumentVersion\":2,\"ChunkID\":\"chunk-123\"}],\"Debug\":\"internal trace\"}\n\n",
			contains: []string{
				"event:meta\ndata:",
				"\"Citations\":",
				"\"URL\":\"https://example.com/art1\"",
				"\"Title\":\"Title 1\"",
				"\"PublishedAt\":\"2026-01-01\"",
			},
			omits: []string{
				"secret text",
				"ChunkText",
				"Score",
				"DocumentVersion",
				"ChunkID",
				"internal trace",
				"Debug",
			},
		},
		{
			name:  "malformed event without data prefix returns unmodified",
			input: "event:meta\nraw message here\n\n",
			exact: "event:meta\nraw message here\n\n",
		},
		{
			name:  "invalid JSON payload returns unmodified",
			input: "event:meta\ndata:invalid-json-payload\n\n",
			exact: "event:meta\ndata:invalid-json-payload\n\n",
		},
		{
			name:  "empty contexts",
			input: "event:meta\ndata:{\"Contexts\":[],\"Debug\":\"trace\"}\n\n",
			contains: []string{
				"event:meta\ndata:{\"Citations\":[]}\n\n",
			},
			omits: []string{
				"Debug",
				"trace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeMetaEvent(tt.input)
			if tt.exact != "" {
				assert.Equal(t, tt.exact, got)
			}
			for _, c := range tt.contains {
				assert.True(t, strings.Contains(got, c), "expected %q to contain %q", got, c)
			}
			for _, o := range tt.omits {
				assert.False(t, strings.Contains(got, o), "expected %q not to contain %q", got, o)
			}
		})
	}
}
