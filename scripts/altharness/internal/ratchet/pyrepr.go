package ratchet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func pyReprString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		quote = '"'
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte(quote)
	for i := 0; i < len(s); {
		r, sz := decodeWTF8(s, i)
		i += sz
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case rune(quote):
			b.WriteByte('\\')
			b.WriteByte(quote)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if pyPrintable(r) {
				b.WriteRune(r)
			} else if r < 0x100 {
				fmt.Fprintf(&b, "\\x%02x", r)
			} else if r < 0x10000 {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				fmt.Fprintf(&b, "\\U%08x", r)
			}
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func pyPrintable(r rune) bool {
	if r == ' ' {
		return true
	}
	if r < 0x20 || r == 0x7f {
		return false
	}
	if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Cs, r) || unicode.Is(unicode.Co, r) || unicode.Is(unicode.Cn, r) {
		return false
	}
	if r != ' ' && (unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) || unicode.Is(unicode.Zs, r)) {
		return false
	}
	return true
}

// decodeWTF8 returns a Unicode scalar, preserving lone surrogates encoded as WTF-8.
func decodeWTF8(s string, i int) (rune, int) {
	if i >= len(s) {
		return utf8.RuneError, 0
	}
	b := s[i]
	if b < 0x80 {
		return rune(b), 1
	}
	if i+2 < len(s) && b == 0xED && s[i+1] >= 0xA0 && s[i+1] <= 0xBF && s[i+2] >= 0x80 && s[i+2] <= 0xBF {
		r := rune(s[i+1]&0x3F)<<6 | rune(s[i+2]&0x3F)
		r |= 0xD800
		// ED A0..BF encodes U+D800..U+DFFF: high = 0xD800 + ((b1&0x3F)<<6 | b2&0x3F) but
		// ED A0 80 is U+D800. b1=0xA0 -> low 6 bits of the top are 0x20, which is the
		// standard 3-byte formula: ((b0&0x0F)<<12)|((b1&0x3F)<<6)|(b2&0x3F).
		r = (rune(b&0x0F) << 12) | (rune(s[i+1]&0x3F) << 6) | rune(s[i+2]&0x3F)
		return r, 3
	}
	r, sz := utf8.DecodeRuneInString(s[i:])
	if r == utf8.RuneError && sz == 1 {
		return rune(s[i]), 1
	}
	return r, sz
}

func pyListRepr(items []string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = pyReprString(s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func pyFloatRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	// Python uses a sign on the exponent: e+20 not e+20 already from Go for 'g'.
	// Go's FormatFloat uses 'e+02' style only when exponent is present as e+NN.
	return s
}
