package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripTrackingParams(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "UTM parameters removed",
			input:    "https://example.com/feed?utm_source=rss&utm_medium=feed&utm_campaign=spring",
			expected: "https://example.com/feed",
		},
		{
			name:     "fbclid removed",
			input:    "https://example.com/feed?fbclid=abc123",
			expected: "https://example.com/feed",
		},
		{
			name:     "gclid removed",
			input:    "https://example.com/feed?gclid=xyz789",
			expected: "https://example.com/feed",
		},
		{
			name:     "msclkid removed",
			input:    "https://example.com/feed?msclkid=def456",
			expected: "https://example.com/feed",
		},
		{
			name:     "mc_eid removed",
			input:    "https://example.com/feed?mc_eid=ghi012",
			expected: "https://example.com/feed",
		},
		{
			name:     "legitimate params preserved, UTM removed",
			input:    "https://example.com/feed?page=2&utm_source=rss",
			expected: "https://example.com/feed?page=2",
		},
		{
			name:     "no parameters unchanged",
			input:    "https://example.com/feed",
			expected: "https://example.com/feed",
		},
		{
			name:     "uppercase UTM removed (case insensitive)",
			input:    "https://example.com/feed?UTM_SOURCE=rss&UTM_MEDIUM=feed",
			expected: "https://example.com/feed",
		},
		{
			name:     "mixed case UTM removed",
			input:    "https://example.com/feed?Utm_Source=rss",
			expected: "https://example.com/feed",
		},
		{
			name:     "real world: hackernoon with utm_source=chatgpt",
			input:    "https://hackernoon.com/feed?utm_source=chatgpt.com",
			expected: "https://hackernoon.com/feed",
		},
		{
			name:     "fragment removed",
			input:    "https://example.com/feed#section",
			expected: "https://example.com/feed",
		},
		{
			name:     "trailing slash removed",
			input:    "https://example.com/feed/",
			expected: "https://example.com/feed",
		},
		{
			name:     "remaining params sorted by key",
			input:    "https://example.com/feed?z=1&a=2&m=3",
			expected: "https://example.com/feed?a=2&m=3&z=1",
		},
		{
			name:     "all UTM variants removed at once",
			input:    "https://example.com/feed?utm_source=a&utm_medium=b&utm_campaign=c&utm_term=d&utm_content=e&utm_id=f",
			expected: "https://example.com/feed",
		},
		{
			name:     "mixed tracking and legitimate params",
			input:    "https://example.com/feed?format=atom&utm_source=rss&page=2&fbclid=abc",
			expected: "https://example.com/feed?format=atom&page=2",
		},
		{
			name:     "root URL trailing slash removed",
			input:    "https://example.com/",
			expected: "https://example.com",
		},
		{
			name:     "root URL without trailing slash",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:    "invalid URL returns error",
			input:   "://invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StripTrackingParams(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripTrackingParams_Idempotent(t *testing.T) {
	input := "https://example.com/feed?page=2&utm_source=rss"

	result1, err := StripTrackingParams(input)
	require.NoError(t, err)

	result2, err := StripTrackingParams(result1)
	require.NoError(t, err)

	assert.Equal(t, result1, result2, "StripTrackingParams should be idempotent")
}
