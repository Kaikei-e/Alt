package ledgers

import "strings"

// pyQuote matches Python repr() for the short strings these CLIs interpolate.
func pyQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
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

func invalidBase(rev string) bool {
	if strings.TrimSpace(rev) == "" || strings.HasPrefix(rev, "-") {
		return true
	}
	if strings.ContainsAny(rev, "\n\r\x00") {
		return true
	}
	return false
}
