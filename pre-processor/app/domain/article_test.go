package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPlaceholderSummary(t *testing.T) {
	assert.Equal(t, "本文が短すぎるため要約できませんでした。", PlaceholderTooShort)
	assert.Equal(t, "本文が長すぎるため要約できませんでした。", PlaceholderTooLong)
	assert.Equal(t, "本文をモデルが処理できなかったため要約できませんでした。", PlaceholderDeadLetter)
	assert.Equal(t, "品質基準を満たす要約を生成できなかったため要約できませんでした。", PlaceholderLowQualityExhausted)

	tests := []struct {
		name     string
		summary  string
		expected bool
	}{
		{
			name:     "too_short",
			summary:  PlaceholderTooShort,
			expected: true,
		},
		{
			name:     "too_long",
			summary:  PlaceholderTooLong,
			expected: true,
		},
		{
			name:     "dead_letter",
			summary:  PlaceholderDeadLetter,
			expected: true,
		},
		{
			name:     "low_quality_exhausted",
			summary:  PlaceholderLowQualityExhausted,
			expected: true,
		},
		{
			name:     "empty",
			summary:  "",
			expected: false,
		},
		{
			name:     "normal_summary",
			summary:  "この記事はAIの最新動向について述べている。",
			expected: false,
		},
		{
			name:     "partial_dead_letter",
			summary:  "本文をモデルが処理できなかったため",
			expected: false,
		},
		{
			name:     "trailing_period_missing",
			summary:  "品質基準を満たす要約を生成できなかったため要約できませんでした",
			expected: false,
		},
		{
			name:     "extra_trailing_text",
			summary:  PlaceholderDeadLetter + "追加",
			expected: false,
		},
		{
			name:     "leading_whitespace",
			summary:  " " + PlaceholderLowQualityExhausted,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsPlaceholderSummary(tc.summary))
		})
	}
}
