package guards

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Token is one shell word. Quoted is true when the word contained a quote.
type Token struct {
	Text   string
	Quoted bool
}

var envAssignRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

var shellNames = map[string]struct{}{"sh": {}, "bash": {}, "zsh": {}, "dash": {}}

func isShellName(name string) bool {
	_, ok := shellNames[name]
	return ok
}

type splitState struct {
	command    string
	length     int
	segments   [][]Token
	currentSeg []Token
	buf        []byte
	hasToken   bool
	hasQuote   bool
	quote      string
}

func (s *splitState) flushToken() {
	if s.hasToken {
		s.currentSeg = append(s.currentSeg, Token{Text: string(s.buf), Quoted: s.hasQuote})
		s.buf = s.buf[:0]
		s.hasToken = false
		s.hasQuote = false
	}
}

func (s *splitState) flushSegment() {
	s.flushToken()
	if len(s.currentSeg) > 0 {
		s.segments = append(s.segments, append([]Token(nil), s.currentSeg...))
		s.currentSeg = s.currentSeg[:0]
	}
}

func scanSingle(s *splitState, i int) int {
	ch := s.command[i]
	if ch == '\'' {
		s.quote = "NORMAL"
	} else {
		s.buf = append(s.buf, ch)
	}
	return i + 1
}

func scanAnsiSingle(s *splitState, i int) int {
	ch := s.command[i]
	if ch == '\'' {
		s.quote = "NORMAL"
		return i + 1
	}
	if ch == '\\' && i+1 < s.length {
		return scanAnsiEscape(s, i+1)
	}
	s.buf = append(s.buf, ch)
	return i + 1
}

func scanAnsiEscape(s *splitState, i int) int {
	esc := s.command[i]
	switch esc {
	case '\'', '"', '\\', '?':
		s.buf = append(s.buf, esc)
		return i + 1
	case 'a':
		s.buf = append(s.buf, '\a')
		return i + 1
	case 'b':
		s.buf = append(s.buf, '\b')
		return i + 1
	case 'e', 'E':
		s.buf = append(s.buf, 0x1b)
		return i + 1
	case 'f':
		s.buf = append(s.buf, '\f')
		return i + 1
	case 'n':
		s.buf = append(s.buf, '\n')
		return i + 1
	case 'r':
		s.buf = append(s.buf, '\r')
		return i + 1
	case 't':
		s.buf = append(s.buf, '\t')
		return i + 1
	case 'v':
		s.buf = append(s.buf, '\v')
		return i + 1
	case 'x':
		end := i + 1
		for end < s.length && end < i+3 && isHexDigit(s.command[end]) {
			end++
		}
		if end > i+1 {
			if v, err := strconv.ParseUint(s.command[i+1:end], 16, 8); err == nil {
				s.buf = append(s.buf, byte(v))
				return end
			}
		}
		s.buf = append(s.buf, '\\', 'x')
		return i + 1
	case 'u', 'U':
		maxDigits := 4
		if esc == 'U' {
			maxDigits = 8
		}
		end := i + 1
		for end < s.length && end < i+1+maxDigits && isHexDigit(s.command[end]) {
			end++
		}
		if end > i+1 {
			if v, err := strconv.ParseUint(s.command[i+1:end], 16, 32); err == nil {
				s.buf = utf8.AppendRune(s.buf, rune(v))
				return end
			}
		}
		s.buf = append(s.buf, '\\', esc)
		return i + 1
	case 'c':
		if i+1 < s.length {
			s.buf = append(s.buf, s.command[i+1]&0x1f)
			return i + 2
		}
		s.buf = append(s.buf, '\\', 'c')
		return i + 1
	default:
		if esc >= '0' && esc <= '7' {
			end := i
			for end < s.length && end < i+3 && s.command[end] >= '0' && s.command[end] <= '7' {
				end++
			}
			if v, err := strconv.ParseUint(s.command[i:end], 8, 8); err == nil {
				s.buf = append(s.buf, byte(v))
				return end
			}
		}
		s.buf = append(s.buf, '\\', esc)
		return i + 1
	}
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func scanDouble(s *splitState, i int) int {
	ch := s.command[i]
	if ch == '\\' {
		if i+1 < s.length && s.command[i+1] == '\n' {
			return i + 2
		}
		if i+1 < s.length {
			s.buf = append(s.buf, s.command[i+1])
			return i + 2
		}
	} else if ch == '"' {
		s.quote = "NORMAL"
		return i + 1
	}
	s.buf = append(s.buf, ch)
	return i + 1
}

func scanNormalBackslash(s *splitState, i int) int {
	if i+1 < s.length && s.command[i+1] == '\n' {
		return i + 2
	}
	if i+1 < s.length {
		s.buf = append(s.buf, s.command[i+1])
		s.hasToken = true
		return i + 2
	}
	return i + 1
}

func isASCIISpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\v' || ch == '\f' || ch == '\r'
}

func scanNormal(s *splitState, i int) int {
	ch := s.command[i]
	if ch == '$' && i+1 < s.length && (s.command[i+1] == '\'' || s.command[i+1] == '"') {
		if s.command[i+1] == '\'' {
			s.quote = "ANSI_SINGLE"
		} else {
			s.quote = "DOUBLE"
		}
		s.hasToken = true
		s.hasQuote = true
		return i + 2
	}
	if ch == '\'' || ch == '"' {
		if ch == '\'' {
			s.quote = "SINGLE"
		} else {
			s.quote = "DOUBLE"
		}
		s.hasToken = true
		s.hasQuote = true
		return i + 1
	}
	if ch == '\\' {
		return scanNormalBackslash(s, i)
	}
	if ch == '#' && !s.hasToken {
		for i < s.length && s.command[i] != '\n' {
			i++
		}
		return i
	}
	if ch == '\n' || ch == ';' || ch == '(' || ch == ')' || ch == '&' || ch == '|' {
		s.flushSegment()
		step := 1
		if (ch == '&' || ch == '|') && i+1 < s.length && s.command[i+1] == ch {
			step = 2
		}
		return i + step
	}
	if isASCIISpace(ch) {
		s.flushToken()
		return i + 1
	}
	s.buf = append(s.buf, ch)
	s.hasToken = true
	return i + 1
}

func splitShellSegments(command string) [][]Token {
	state := &splitState{command: command, length: len(command), quote: "NORMAL"}
	i := 0
	for i < state.length {
		switch state.quote {
		case "SINGLE":
			i = scanSingle(state, i)
		case "ANSI_SINGLE":
			i = scanAnsiSingle(state, i)
		case "DOUBLE":
			i = scanDouble(state, i)
		default:
			i = scanNormal(state, i)
		}
	}
	state.flushSegment()
	return state.segments
}

func scanDollarParen(command string, start, length int) (string, int) {
	depth := 1
	quote := "NORMAL"
	j := start
	for j < length && depth > 0 {
		c := command[j]
		switch quote {
		case "SINGLE":
			if c == '\'' {
				quote = "NORMAL"
			}
			j++
		case "ANSI_SINGLE":
			if c == '\\' && j+1 < length {
				j += 2
				continue
			}
			if c == '\'' {
				quote = "NORMAL"
			}
			j++
		case "DOUBLE":
			if c == '\\' && j+1 < length {
				j += 2
				continue
			}
			if c == '"' {
				quote = "NORMAL"
			}
			j++
		default:
			if c == '\\' && j+1 < length {
				j += 2
				continue
			}
			if c == '$' && j+1 < length && command[j+1] == '\'' {
				quote = "ANSI_SINGLE"
				j += 2
				continue
			}
			if c == '\'' {
				quote = "SINGLE"
				j++
				continue
			}
			if c == '"' {
				quote = "DOUBLE"
				j++
				continue
			}
			if c == '(' {
				depth++
				j++
				continue
			}
			if c == ')' {
				depth--
				if depth == 0 {
					return command[start:j], j + 1
				}
				j++
				continue
			}
			j++
		}
	}
	return command[start:j], j + 1
}

func scanBacktick(command string, start, length int) (string, int) {
	j := start
	for j < length {
		if command[j] == '\\' && j+1 < length {
			j += 2
			continue
		}
		if command[j] == '`' {
			break
		}
		j++
	}
	return command[start:j], j + 1
}

func extractSubstitutions(command string) []string {
	var subs []string
	i := 0
	n := len(command)
	inDouble := false
	for i < n {
		ch := command[i]
		if ch == '\\' && i+1 < n {
			i += 2
			continue
		}
		if !inDouble && ch == '$' && i+1 < n && command[i+1] == '\'' {
			i += 2
			for i < n {
				if command[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if command[i] == '\'' {
					i++
					break
				}
				i++
			}
			continue
		}
		if !inDouble && ch == '\'' {
			i++
			for i < n && command[i] != '\'' {
				i++
			}
			i++
			continue
		}
		if ch == '"' {
			inDouble = !inDouble
			i++
			continue
		}
		if ch == '$' && i+1 < n && command[i+1] == '(' {
			inner, next := scanDollarParen(command, i+2, n)
			subs = append(subs, inner)
			i = next
			continue
		}
		if ch == '`' {
			inner, next := scanBacktick(command, i+1, n)
			subs = append(subs, inner)
			i = next
			continue
		}
		i++
	}
	return subs
}

func baseName(path string) string {
	path = strings.ReplaceAll(path, `\`, "/")
	if path == "" {
		return ""
	}
	i := strings.LastIndex(path, "/")
	if i >= 0 {
		return path[i+1:]
	}
	return path
}

func stepQuoted(command string, i int, quote string) (int, string) {
	ch := command[i]
	if quote == "SINGLE" {
		next := "SINGLE"
		if ch == '\'' {
			next = "NORMAL"
		}
		return i + 1, next
	}
	if quote == "ANSI_SINGLE" {
		if ch == '\\' && i+1 < len(command) {
			return i + 2, "ANSI_SINGLE"
		}
		next := "ANSI_SINGLE"
		if ch == '\'' {
			next = "NORMAL"
		}
		return i + 1, next
	}
	if ch == '\\' && i+1 < len(command) {
		return i + 2, "DOUBLE"
	}
	next := "DOUBLE"
	if ch == '"' {
		next = "NORMAL"
	}
	return i + 1, next
}

func hasBackgroundAmpersand(command string) bool {
	n := len(command)
	i := 0
	quote := "NORMAL"
	for i < n {
		if quote != "NORMAL" {
			i, quote = stepQuoted(command, i, quote)
			continue
		}
		ch := command[i]
		if ch == '\\' && i+1 < n {
			i += 2
			continue
		}
		if ch == '$' && i+1 < n && command[i+1] == '\'' {
			quote = "ANSI_SINGLE"
			i += 2
			continue
		}
		if ch == '\'' || ch == '"' {
			if ch == '\'' {
				quote = "SINGLE"
			} else {
				quote = "DOUBLE"
			}
			i++
			continue
		}
		if ch == '#' && (i == 0 || isASCIISpace(command[i-1]) || strings.ContainsRune(";|&()", rune(command[i-1]))) {
			nl := strings.IndexByte(command[i:], '\n')
			if nl == -1 {
				i = n
			} else {
				i = i + nl + 1
			}
			continue
		}
		if ch == '&' {
			prev := byte(0)
			if i > 0 {
				prev = command[i-1]
			}
			next := byte(0)
			if i+1 < n {
				next = command[i+1]
			}
			if next == '&' || next == '>' {
				i += 2
				continue
			}
			if prev == '>' || prev == '<' || prev == '|' {
				i++
				continue
			}
			return true
		}
		i++
	}
	return false
}

func validUTF8(s string) bool { return utf8.ValidString(s) }
