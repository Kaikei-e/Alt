package tokenize

import (
	"testing"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessTagToSynonyms_EmptyTags(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Test with empty tags array
	result := ProcessTagToSynonyms(tok, []string{})
	assert.NotNil(t, result)
	assert.Empty(t, result, "should return empty map for empty tags")
}

func TestProcessTagToSynonyms_AllJapaneseTags(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Valid compound noun tags decompose into 2..4 meaningful noun tokens
	tags := []string{"機械学習", "自然言語処理"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	assert.Equal(t, []string{"機械", "学習"}, result["機械学習"])
	assert.Equal(t, []string{"自然", "言語", "処理"}, result["自然言語処理"])
}

func TestProcessTagToSynonyms_SkipsSingleTokenSelfSynonyms(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Single-morpheme tags and noun+suffix tags with <2 meaningful noun tokens
	// must NOT register self-synonyms or 1-token entries.
	tags := []string{"テスト", "ひらがな", "カタカナ", "漢字", "日本語"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	assert.Empty(t, result, "single-token tags must not register self-synonyms")
}

func TestProcessTagToSynonyms_SkipsLongAndPollutedTags(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	tags := []string{
		"000mAh搭載の12インチAndroidタブレット", // >15 runes and contains particle/numbers
		"あいうえおかきくけこさしすせそたち",          // >15 runes
		"日本の経済動向",                    // contains particle の
		"機械学習、深層学習",                  // contains punctuation
		"2026年10月",                   // numbers and counter suffixes only
	}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	assert.Empty(t, result, "long sentence fragments, particles, punctuation, and numeric tags must be skipped")
}

func TestProcessTagToSynonyms_AllEnglishTags(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Test with only English tags - no synonyms needed
	tags := []string{"test", "english"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	assert.Empty(t, result, "should return empty map for English-only tags (no synonyms needed)")
}

func TestProcessTagToSynonyms_MixedTags(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Test with mixed Japanese compound noun and English tags
	tags := []string{"test", "機械学習", "english"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	// Only valid Japanese compound noun tags should have synonyms
	assert.Contains(t, result, "機械学習")
	assert.NotContains(t, result, "test", "English tags should not be in synonyms")
	assert.NotContains(t, result, "english", "English tags should not be in synonyms")
}

func TestIsJapaneseTag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Hiragana", "ひらがな", true},
		{"Katakana", "カタカナ", true},
		{"Kanji", "漢字", true},
		{"Mixed", "日本語test", true},
		{"English only", "english", false},
		{"Numbers only", "12345", false},
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsJapaneseTag(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
