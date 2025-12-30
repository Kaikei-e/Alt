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

	// Test with only Japanese tags
	tags := []string{"日本語", "テスト"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result, "should return synonyms for Japanese tags")
	// Check that both Japanese tags have synonyms
	assert.Contains(t, result, "日本語")
	assert.Contains(t, result, "テスト")
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

	// Test with mixed Japanese and English tags
	tags := []string{"test", "日本語", "english"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	// Only Japanese tags should have synonyms
	assert.Contains(t, result, "日本語")
	assert.NotContains(t, result, "test", "English tags should not be in synonyms")
	assert.NotContains(t, result, "english", "English tags should not be in synonyms")
}

func TestProcessTagToSynonyms_OnlyJapaneseCharacters(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Test with tags containing only Japanese characters
	tags := []string{"ひらがな", "カタカナ", "漢字"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	// Japanese tags should generate synonyms
	assert.NotEmpty(t, result)
	assert.Len(t, result, 3, "should have synonyms for all 3 Japanese tags")
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
