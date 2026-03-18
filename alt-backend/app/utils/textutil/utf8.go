package textutil

import "strings"

// TruncateValidUTF8 trims invalid UTF-8 and truncates by rune count.
// If truncation happens, it appends a single ellipsis rune.
func TruncateValidUTF8(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}

	valid := strings.ToValidUTF8(s, "")
	runes := []rune(valid)
	if len(runes) <= maxRunes {
		return valid
	}

	return string(runes[:maxRunes]) + "…"
}
