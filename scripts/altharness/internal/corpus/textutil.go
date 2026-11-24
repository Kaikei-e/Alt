package corpus

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func snippet(text string) string {
	compact := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(compact) <= OutputSnippetLimit {
		return compact
	}
	runes := []rune(compact)
	return string(runes[:OutputSnippetLimit]) + "..."
}

func pyRepr(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return strconv.Quote(s)
	}
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\', '\'':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func pyList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = pyRepr(item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func nonSpaceLen(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

func pySplitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' && s[i] != '\r' {
			continue
		}
		lines = append(lines, s[start:i])
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			i++
		}
		start = i + 1
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func isNormalizedRepoRel(rel string) bool {
	if rel == "" || strings.TrimSpace(rel) != rel {
		return false
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "./") || strings.Contains(rel, `\`) {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	// local insertion sort keeps the import list smaller in this file
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j] < out[j-1] {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

func formatTimeout(seconds int) string {
	return fmt.Sprintf("%ds", seconds)
}
