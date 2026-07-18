package retrieval

import "unicode/utf8"

const queryLogPreviewRunes = 48

func queryLogPreview(query string) string {
	if utf8.RuneCountInString(query) <= queryLogPreviewRunes {
		return query
	}
	runes := []rune(query)
	return string(runes[:queryLogPreviewRunes]) + "..."
}
