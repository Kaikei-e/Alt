package guards

import (
	"regexp"
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
