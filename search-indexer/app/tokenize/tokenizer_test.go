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
}

func TestProcessTagToSynonyms_AllEnglishTags(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Test with only English tags
	tags := []string{"test", "english"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result, "should return map for English tags")
	assert.Contains(t, result, "test", "should use first tag as key")
}

func TestProcessTagToSynonyms_MixedTags(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Test with mixed Japanese and English tags
	tags := []string{"test", "日本語", "english"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result, "should handle mixed tags")
}

func TestProcessTagToSynonyms_OnlyJapaneseCharacters(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	require.NoError(t, err)

	// Test with tags containing only Japanese characters (all filtered out)
	tags := []string{"ひらがな", "カタカナ", "漢字"}
	result := ProcessTagToSynonyms(tok, tags)
	assert.NotNil(t, result)
	// Japanese tags should generate synonyms
	assert.NotEmpty(t, result)
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

func TestProcessTagWithoutSynonyms_EmptyArray(t *testing.T) {
	// This should not panic even with empty array
	result := processTagWithoutSynonyms([]string{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestProcessTagWithoutSynonyms_MixedTags(t *testing.T) {
	tags := []string{"english", "日本語", "test"}
	result := processTagWithoutSynonyms(tags)
	assert.NotNil(t, result)
	assert.Contains(t, result, "english")
	assert.Contains(t, result, "test")
	assert.NotContains(t, result, "日本語", "should filter out Japanese tags")
}
