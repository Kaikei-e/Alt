package screener

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var rustCharLitRE = regexp.MustCompile(`^'(?:\\(?:x[0-9A-Fa-f]{2}|u\{[0-9A-Fa-f_]+\}|.)|[^'\\])'`)

func stripCStyle(src string, rust bool) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	n := len(src)
	for i < n {
		ch := src[i]
		var nxt byte
		if i+1 < n {
			nxt = src[i+1]
		}
		if ch == '/' && nxt == '/' {
			rel := strings.IndexByte(src[i+2:], '\n')
			if rel == -1 {
				i = n
			} else {
				i = i + 2 + rel
			}
			continue
		}
		if ch == '/' && nxt == '*' {
			i = skipBlockComment(src, i, rust, &b)
			continue
		}
		if rust && (ch == 'r' || ((ch == 'b' || ch == 'c') && nxt == 'r')) {
			if end, ok := trySkipRustRaw(src, i, &b); ok {
				i = end
				continue
			}
			b.WriteByte(ch)
			i++
			continue
		}
		if rust && (ch == '\'' || ch == 'b') {
			if end, ok := trySkipRustChar(src, i, &b); ok {
				i = end
				continue
			}
			b.WriteByte(ch)
			i++
			continue
		}
		if ch == '"' || ch == '`' || (!rust && ch == '\'') {
			i = skipQuoted(src, i, &b)
			continue
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}

func skipBlockComment(src string, start int, nested bool, out *strings.Builder) int {
	depth := 1
	idx := start + 2
	n := len(src)
	for idx < n && depth > 0 {
		if nested && idx+1 < n && src[idx] == '/' && src[idx+1] == '*' {
			depth++
			idx += 2
			continue
		}
		if idx+1 < n && src[idx] == '*' && src[idx+1] == '/' {
			depth--
			idx += 2
			continue
		}
		if src[idx] == '\n' {
			out.WriteByte('\n')
		}
		idx++
	}
	out.WriteByte(' ')
	return idx
}

func trySkipRustRaw(src string, idx int, out *strings.Builder) (int, bool) {
	end, ok := scanRustRawStringEnd(src, idx)
	if !ok {
		return 0, false
	}
	newlines := strings.Count(src[idx:end], "\n")
	out.WriteString("\"\"")
	out.WriteString(strings.Repeat("\n", newlines))
	return end, true
}

func trySkipRustChar(src string, idx int, out *strings.Builder) (int, bool) {
	prevIdent := prevIsIdent(src, idx)
	ch := src[idx]
	quoteIdx := -1
	if ch == 'b' && !prevIdent && idx+1 < len(src) && src[idx+1] == '\'' {
		quoteIdx = idx + 1
	} else if ch == '\'' {
		quoteIdx = idx
	} else {
		return 0, false
	}
	rest := src[quoteIdx:]
	if len(rest) > 16 {
		rest = rest[:16]
	}
	loc := rustCharLitRE.FindStringIndex(rest)
	if loc == nil {
		return 0, false
	}
	out.WriteString("''")
	return quoteIdx + loc[1], true
}

func skipQuoted(src string, start int, out *strings.Builder) int {
	quote := src[start]
	out.WriteByte(quote)
	idx := start + 1
	n := len(src)
	for idx < n {
		c := src[idx]
		if c == '\\' {
			idx += 2
			continue
		}
		if c == quote {
			out.WriteByte(quote)
			return idx + 1
		}
		if c == '\n' {
			out.WriteByte('\n')
		}
		idx++
	}
	return idx
}

func scanRustRawStringEnd(src string, start int) (int, bool) {
	if prevIsIdent(src, start) {
		return 0, false
	}
	n := len(src)
	if start >= n {
		return 0, false
	}
	ch := src[start]
	rIdx := start
	switch ch {
	case 'r':
		rIdx = start
	case 'b', 'c':
		if start+1 < n && src[start+1] == 'r' {
			rIdx = start + 1
		} else {
			return 0, false
		}
	default:
		return 0, false
	}
	j := rIdx + 1
	for j < n && src[j] == '#' {
		j++
	}
	if j >= n || src[j] != '"' {
		return 0, false
	}
	hashes := j - (rIdx + 1)
	endDelim := "\"" + strings.Repeat("#", hashes)
	rel := strings.Index(src[j+1:], endDelim)
	if rel == -1 {
		return n, true
	}
	return j + 1 + rel + len(endDelim), true
}

func prevIsIdent(src string, start int) bool {
	if start <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(src[:start])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func scanEscapedQuoteEnd(src string, start int, quote byte) int {
	idx := start + 1
	n := len(src)
	for idx < n {
		c := src[idx]
		if c == '\\' {
			idx += 2
			continue
		}
		if c == quote {
			return idx + 1
		}
		idx++
	}
	return n
}

func scanStringLiteralEnd(src string, start int, language string) (int, bool) {
	if start < 0 || start >= len(src) {
		return 0, false
	}
	ch := src[start]
	if language == "rust" {
		if ch == 'r' || ((ch == 'b' || ch == 'c') && start+1 < len(src) && src[start+1] == 'r') {
			return scanRustRawStringEnd(src, start)
		}
		if ch == 'b' && start+1 < len(src) && src[start+1] == '\'' {
			if prevIsIdent(src, start) {
				return 0, false
			}
			rest := src[start+1:]
			if len(rest) > 16 {
				rest = rest[:16]
			}
			loc := rustCharLitRE.FindStringIndex(rest)
			if loc == nil {
				return 0, false
			}
			return start + 1 + loc[1], true
		}
		if ch == '\'' {
			rest := src[start:]
			if len(rest) > 16 {
				rest = rest[:16]
			}
			loc := rustCharLitRE.FindStringIndex(rest)
			if loc == nil {
				return 0, false
			}
			return start + loc[1], true
		}
		if ch == '"' {
			return scanEscapedQuoteEnd(src, start, ch), true
		}
		return 0, false
	}
	if language == "go" && ch == '`' {
		rel := strings.IndexByte(src[start+1:], '`')
		if rel == -1 {
			return len(src), true
		}
		return start + 1 + rel + 1, true
	}
	valid := ch == '"' || ch == '\''
	if language != "go" {
		valid = valid || ch == '`'
	}
	if !valid {
		return 0, false
	}
	return scanEscapedQuoteEnd(src, start, ch), true
}

func stripCommentsKeepStrings(src, language string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	n := len(src)
	nested := language == "rust"
	for i < n {
		if end, ok := scanStringLiteralEnd(src, i, language); ok {
			b.WriteString(src[i:end])
			i = end
			continue
		}
		ch := src[i]
		var nxt byte
		if i+1 < n {
			nxt = src[i+1]
		}
		if ch == '/' && nxt == '/' {
			rel := strings.IndexByte(src[i+2:], '\n')
			if rel == -1 {
				i = n
			} else {
				i = i + 2 + rel
			}
			continue
		}
		if ch == '/' && nxt == '*' {
			i = skipBlockComment(src, i, nested, &b)
			continue
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}
