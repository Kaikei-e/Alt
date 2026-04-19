// Package service exposes article-processing services.
//
// DetectLanguage classifies short text (titles, lead paragraphs) into a BCP-47
// short code. The pipeline only needs a coarse JP/EN split to drive downstream
// language-balanced retrieval in Acolyte — richer detection (model-based) can
// replace this later without changing callers.
package service

import (
	"strings"
	"unicode"
)

// DetectLanguage returns one of "ja", "en", or "und".
//
// The classifier counts letters (CJK/Kana + Latin) and ignores whitespace,
// digits, and punctuation so that "OpenAI releases o3 in 2026" is treated as
// English rather than being diluted by the year. "und" is returned when the
// text has no letters or is too short to judge.
func DetectLanguage(text string) string {
	if strings.TrimSpace(text) == "" {
		return "und"
	}

	var jpLetters, enLetters int
	for _, r := range text {
		switch {
		case isJapaneseLetter(r):
			jpLetters++
		case unicode.IsLetter(r) && r < 0x0100:
			enLetters++
		}
	}

	totalLetters := jpLetters + enLetters
	if totalLetters < 2 {
		return "und"
	}

	// CJK characters are information-dense — even a minority of CJK letters is
	// a strong Japanese signal. Threshold tuned so "The word 寿司 is popular"
	// stays English while "東京オリンピック 2028 開催地決定" stays Japanese.
	if jpLetters*3 >= totalLetters {
		return "ja"
	}
	if enLetters > jpLetters {
		return "en"
	}
	return "und"
}

func isJapaneseLetter(r rune) bool {
	switch {
	case unicode.Is(unicode.Hiragana, r):
		return true
	case unicode.Is(unicode.Katakana, r):
		return true
	case unicode.Is(unicode.Han, r):
		return true
	}
	return false
}
