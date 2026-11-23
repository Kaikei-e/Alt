package suppress

import (
	"fmt"
	"sort"
	"strings"
)

const (
	minReasonLength       = 5
	exitClean             = 0
	exitViolations        = 1
	exitUsage             = 2
	maxExpiresHorizonDays = 366
	generatedHeaderLines  = 40
	maxReasonDisplay      = 90
	allowlistRelPath      = "scripts/lint/suppression_allowlist.toml"
)

func pyReprString(s string) string {
	hasSingle := strings.Contains(s, "'")
	hasDouble := strings.Contains(s, "\"")
	useDouble := hasSingle && !hasDouble
	var b strings.Builder
	if useDouble {
		b.WriteByte('"')
	} else {
		b.WriteByte('\'')
	}
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			if useDouble {
				b.WriteRune(r)
			} else {
				b.WriteString(`\'`)
			}
		case '"':
			if useDouble {
				b.WriteString(`\"`)
			} else {
				b.WriteRune(r)
			}
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	if useDouble {
		b.WriteByte('"')
	} else {
		b.WriteByte('\'')
	}
	return b.String()
}

func pyListRepr(items []string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = pyReprString(s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func sortStrings(ss []string) { sort.Strings(ss) }

func lstripDotSlash(s string) string {
	i := 0
	for i < len(s) && (s[i] == '.' || s[i] == '/') {
		i++
	}
	return s[i:]
}

func normalizeSourceLine(line string) string {
	return strings.Join(strings.Fields(line), " ")
}

func nonSpaceLen(s string) int {
	n := 0
	for _, r := range s {
		if !isPySpace(r) {
			n++
		}
	}
	return n
}

func isPySpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f', '\v', '\u00a0', '\u1680',
		'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005',
		'\u2006', '\u2007', '\u2008', '\u2009', '\u200a', '\u2028',
		'\u2029', '\u202f', '\u205f', '\u3000':
		return true
	default:
		return false
	}
}
