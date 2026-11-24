package ratchet

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// decodeUTF8Replace matches Python bytes.decode("utf-8", "replace"): one U+FFFD
// for each invalid start byte or incomplete/invalid sequence prefix.
func decodeUTF8Replace(b []byte) string {
	s, _ := decodeUTF8(b, true)
	return s
}

// decodeUTF8Strict matches Python bytes.decode("utf-8") error text.
func decodeUTF8Strict(b []byte) (string, error) {
	return decodeUTF8(b, false)
}

func decodeUTF8(b []byte, replace bool) (string, error) {
	var out strings.Builder
	out.Grow(len(b))
	for i := 0; i < len(b); {
		need, okStart := utf8SeqLen(b[i])
		if !okStart {
			if !replace {
				return "", utf8DecodeError(b, i, i+1, "invalid start byte")
			}
			out.WriteRune('\uFFFD')
			i++
			continue
		}
		if need == 1 {
			out.WriteByte(b[i])
			i++
			continue
		}
		got := 1
		bad := false
		for k := 1; k < need && i+k < len(b); k++ {
			if !utf8ContOK(b[i], k, b[i+k]) {
				bad = true
				break
			}
			got++
		}
		if bad || got < need && i+got < len(b) {
			// A non-continuation or out-of-range byte stops the prefix.
			if !bad && got < need {
				// fell through only when i+got < len but loop stopped? handled below
			}
			if bad || (got < need && i+got < len(b)) {
				end := i + got
				if !replace {
					return "", utf8DecodeError(b, i, end, "invalid continuation byte")
				}
				out.WriteRune('\uFFFD')
				i = end
				continue
			}
		}
		if got < need {
			if !replace {
				return "", utf8DecodeError(b, i, i+got, "unexpected end of data")
			}
			out.WriteRune('\uFFFD')
			i += got
			continue
		}
		r, _ := utf8.DecodeRune(b[i : i+need])
		out.WriteRune(r)
		i += need
	}
	return out.String(), nil
}

func utf8DecodeError(b []byte, start, end int, reason string) error {
	if end < start {
		end = start
	}
	if end > len(b) {
		end = len(b)
	}
	if end <= start {
		end = start + 1
		if end > len(b) {
			end = len(b)
		}
	}
	if end-start == 1 {
		return fmt.Errorf("'utf-8' codec can't decode byte 0x%02x in position %d: %s", b[start], start, reason)
	}
	return fmt.Errorf("'utf-8' codec can't decode bytes in position %d-%d: %s", start, end-1, reason)
}

func utf8SeqLen(c byte) (int, bool) {
	switch {
	case c < 0x80:
		return 1, true
	case c >= 0xC2 && c <= 0xDF:
		return 2, true
	case c >= 0xE0 && c <= 0xEF:
		return 3, true
	case c >= 0xF0 && c <= 0xF4:
		return 4, true
	default:
		return 0, false
	}
}

func utf8ContOK(start byte, idx int, c byte) bool {
	if c < 0x80 || c > 0xBF {
		return false
	}
	if idx != 1 {
		return true
	}
	switch start {
	case 0xE0:
		return c >= 0xA0
	case 0xED:
		return c <= 0x9F
	case 0xF0:
		return c >= 0x90
	case 0xF4:
		return c <= 0x8F
	default:
		return true
	}
}

// splitLines matches str.splitlines(): universal newlines, no trailing empty
// line when the text ends in a break.
func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		sz, ok := lineBreakAt(s, i)
		if !ok {
			_, rsz := utf8.DecodeRuneInString(s[i:])
			if rsz < 1 {
				rsz = 1
			}
			i += rsz
			continue
		}
		out = append(out, s[start:i])
		i += sz
		start = i
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func lineBreakAt(s string, i int) (int, bool) {
	if i >= len(s) {
		return 0, false
	}
	switch s[i] {
	case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e:
		return 1, true
	case '\r':
		if i+1 < len(s) && s[i+1] == '\n' {
			return 2, true
		}
		return 1, true
	}
	r, sz := utf8.DecodeRuneInString(s[i:])
	if r == 0x85 || r == 0x2028 || r == 0x2029 {
		return sz, true
	}
	return 0, false
}
