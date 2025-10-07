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

func processTagWithoutSynonyms(tags []string) []string {
	tagWithoutSynonyms := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !IsJapaneseTag(tag) {
			tagWithoutSynonyms = append(tagWithoutSynonyms, tag)
		}
	}
	return tagWithoutSynonyms
}

func ProcessTagToSynonyms(tokenizer *tokenizer.Tokenizer, tags []string) map[string][]string {
	synonyms := make([]map[string][]string, 0, len(tags))
	for _, tag := range tags {
		if IsJapaneseTag(tag) {
			synonyms = append(synonyms, createSynonyms(tokenizer, tag))
		}
	}

	if len(synonyms) == 0 {
		nonJapaneseTags := processTagWithoutSynonyms(tags)
		if len(nonJapaneseTags) == 0 {
			// No tags to process, return empty map
			return map[string][]string{}
		}
		return map[string][]string{
			nonJapaneseTags[0]: nonJapaneseTags,
		}
	}

	return synonyms[0]
}
