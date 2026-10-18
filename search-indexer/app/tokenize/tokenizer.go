package tokenize

import (
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

const (
	// MaxTagRunes bounds tag length eligible for synonym decomposition so
	// sentence fragments never enter the Meilisearch synonyms dictionary.
	MaxTagRunes = 15
	// MinCompoundTokens requires at least 2 meaningful noun components (never
	// registering a 1-token self-synonym).
	MinCompoundTokens = 2
	// MaxCompoundTokens bounds the number of component nouns a compound noun
	// tag may decompose into.
	MaxCompoundTokens = 4
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

func hasSentencePunctuationOrSpace(text string) bool {
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return true
		}
		switch r {
		case '。', '、', '！', '？', '!', '?', ',', '，', '．', '「', '」', '『', '』', '（', '）', '(', ')':
			return true
		}
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isMeaningfulNounSubcategory(sub string) bool {
	switch sub {
	case "一般", "固有名詞", "サ変接続", "形容動詞語幹":
		return true
	default:
		return false
	}
}

func tokenizeJapaneseCompoundNouns(t *tokenizer.Tokenizer, text string) []string {
	runeCount := utf8.RuneCountInString(text)
	if runeCount < 2 || runeCount > MaxTagRunes {
		return nil
	}
	if hasSentencePunctuationOrSpace(text) {
		return nil
	}

	tokens := t.Analyze(text, tokenizer.Normal)
	if len(tokens) < MinCompoundTokens {
		return nil
	}

	meaningful := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		pos := tok.POS()
		if len(pos) == 0 || pos[0] != "名詞" {
			// Any particle (助詞), verb (動詞), auxiliary verb (助動詞), symbol (記号),
			// etc. means this tag is a clause/phrase rather than a compound noun.
			return nil
		}
		if len(pos) >= 2 && !isMeaningfulNounSubcategory(pos[1]) {
			// Skip numbers (数), suffixes (接尾), pronouns (代名詞), dependent nouns (非自立).
			continue
		}
		surface := tok.Surface
		if surface == text || utf8.RuneCountInString(surface) < 2 || isAllDigits(surface) {
			continue
		}
		if !slices.Contains(meaningful, surface) {
			meaningful = append(meaningful, surface)
		}
	}

	if len(meaningful) < MinCompoundTokens || len(meaningful) > MaxCompoundTokens {
		return nil
	}
	return meaningful
}

func IsJapaneseTag(text string) bool {
	return containsJapanese(text)
}

func createSynonyms(t *tokenizer.Tokenizer, text string) map[string][]string {
	tokens := tokenizeJapaneseCompoundNouns(t, text)
	if len(tokens) == 0 {
		return nil
	}

	return map[string][]string{
		text: tokens,
	}
}

func ProcessTagToSynonyms(t *tokenizer.Tokenizer, tags []string) map[string][]string {
	result := make(map[string][]string)

	for _, tag := range tags {
		if IsJapaneseTag(tag) {
			synonyms := createSynonyms(t, tag)
			for k, v := range synonyms {
				result[k] = v
			}
		}
	}

	// If no Japanese tags were processed, return empty map
	// (non-Japanese tags don't need synonym processing)
	return result
}
