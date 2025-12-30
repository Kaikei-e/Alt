package tokenize

import (
	"unicode"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

func InitTokenizer() (*tokenizer.Tokenizer, error) {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil, err
	}
	return t, nil
}

func containsJapanese(text string) bool {
	for _, r := range text {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

func tokenizeJapanese(t *tokenizer.Tokenizer, text string) ([]string, error) {
	return t.Wakati(text), nil
}

func IsJapaneseTag(text string) bool {
	return containsJapanese(text)
}

func createSynonyms(tokenizer *tokenizer.Tokenizer, text string) map[string][]string {
	tokens, err := tokenizeJapanese(tokenizer, text)
	if err != nil {
		return nil
	}

	return map[string][]string{
		text: tokens,
	}
}

func ProcessTagToSynonyms(tokenizer *tokenizer.Tokenizer, tags []string) map[string][]string {
	result := make(map[string][]string)

	for _, tag := range tags {
		if IsJapaneseTag(tag) {
			synonyms := createSynonyms(tokenizer, tag)
			for k, v := range synonyms {
				result[k] = v
			}
		}
	}

	// If no Japanese tags were processed, return empty map
	// (non-Japanese tags don't need synonym processing)
	return result
}
