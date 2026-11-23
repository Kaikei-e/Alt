package screener

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type pyK int

const (
	pyEOF pyK = iota
	pyNL
	pyIndent
	pyDedent
	pyIdent
	pyNum
	pyStr
	pyOp
)

type pyTok struct {
	kind  pyK
	val   string
	exprs []string
	line  int
}

func tokenizePy(src string) ([]pyTok, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	src = strings.TrimPrefix(src, "\uFEFF")
	n := len(src)
	indent := []int{0}
	var out []pyTok
	paren := 0
	atStart := true
	line := 1
	i := 0
	for i < n {
		if atStart && paren == 0 {
			col := 0
			j := i
			for j < n {
				switch src[j] {
				case ' ', '\f':
					col++
					j++
				case '\t':
					col += 8 - (col % 8)
					j++
				default:
					goto measured
				}
			}
		measured:
			if j >= n {
				i = j
				break
			}
			if src[j] == '\n' {
				line++
				i = j + 1
				continue
			}
			if src[j] == '#' {
				for j < n && src[j] != '\n' {
					j++
				}
				i = j
				continue
			}
			if src[j] == '\\' && j+1 < n && src[j+1] == '\n' {
				line++
				i = j + 2
				continue
			}
			cur := indent[len(indent)-1]
			switch {
			case col > cur:
				indent = append(indent, col)
				out = append(out, pyTok{kind: pyIndent, line: line})
			case col < cur:
				for col < indent[len(indent)-1] {
					indent = indent[:len(indent)-1]
					out = append(out, pyTok{kind: pyDedent, line: line})
				}
				if col != indent[len(indent)-1] {
					return nil, fmt.Errorf("line %d: inconsistent dedent", line)
				}
			}
			i = j
			atStart = false
			continue
		}
		start := i
		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case r == ' ' || r == '\t' || r == '\f' || r == '\v':
			i += size
		case r == '#':
			for i < n && src[i] != '\n' {
				i++
			}
		case r == '\\' && i+1 < n && src[i+1] == '\n':
			i += 2
			line++
		case r == '\n':
			if paren == 0 {
				out = append(out, pyTok{kind: pyNL, val: "\n", line: line})
				atStart = true
			}
			i++
			line++
		case r == '"' || r == '\'':
			end, exprs, err := scanPyString(src, i, "")
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			out = append(out, pyTok{kind: pyStr, val: src[i:end], exprs: exprs, line: line})
			line += strings.Count(src[i:end], "\n")
			i = end
		case isIdentStart(r):
			j := i + size
			for j < n {
				r2, sz := utf8.DecodeRuneInString(src[j:])
				if !isIdentCont(r2) {
					break
				}
				j += sz
			}
			word := src[i:j]
			if isStrPrefix(word) && j < n && (src[j] == '"' || src[j] == '\'') {
				end, exprs, err := scanPyString(src, j, word)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", line, err)
				}
				out = append(out, pyTok{kind: pyStr, val: src[i:end], exprs: exprs, line: line})
				line += strings.Count(src[i:end], "\n")
				i = end
				break
			}
			out = append(out, pyTok{kind: pyIdent, val: word, line: line})
			i = j
		case r == '.' && i+1 < n && src[i+1] >= '0' && src[i+1] <= '9':
			j := scanNumber(src, i)
			out = append(out, pyTok{kind: pyNum, val: src[i:j], line: line})
			i = j
		case r >= '0' && r <= '9':
			j := scanNumber(src, i)
			out = append(out, pyTok{kind: pyNum, val: src[i:j], line: line})
			i = j
		default:
			if op, ok := matchPyOp(src, i); ok {
				out = append(out, pyTok{kind: pyOp, val: op, line: line})
				switch op {
				case "(", "[", "{":
					paren++
				case ")", "]", "}":
					if paren > 0 {
						paren--
					}
				}
				i += len(op)
				break
			}
			return nil, fmt.Errorf("line %d: unexpected character %q", line, string(r))
		}
		if i == start {
			return nil, fmt.Errorf("line %d: tokenizer stuck", line)
		}
	}
	if !atStart && paren == 0 {
		out = append(out, pyTok{kind: pyNL, val: "\n", line: line})
	}
	for len(indent) > 1 {
		indent = indent[:len(indent)-1]
		out = append(out, pyTok{kind: pyDedent, line: line})
	}
	out = append(out, pyTok{kind: pyEOF, line: line})
	return out, nil
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isStrPrefix(s string) bool {
	if s == "" || len(s) > 3 {
		return false
	}
	lower := strings.ToLower(s)
	seen := map[byte]bool{}
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c != 'r' && c != 'f' && c != 'b' && c != 'u' {
			return false
		}
		if seen[c] {
			return false
		}
		seen[c] = true
	}
	if seen['u'] && (seen['b'] || seen['f']) {
		return false
	}
	if seen['b'] && seen['f'] {
		return false
	}
	return true
}

func matchPyOp(src string, i int) (string, bool) {
	rest := src[i:]
	candidates := []string{
		"...", "**=", "//=", ">>=", "<<=",
		"**", "//", "<<", ">>", "==", "!=", "<=", ">=",
		"+=", "-=", "*=", "/=", "%=", "@=", "&=", "|=", "^=", ":=", "->",
		"+", "-", "*", "/", "%", "@", "&", "|", "^", "~", "<", ">", "=",
		"(", ")", "[", "]", "{", "}", ",", ":", ".", ";",
	}
	for _, op := range candidates {
		if strings.HasPrefix(rest, op) {
			return op, true
		}
	}
	return "", false
}

func scanNumber(src string, i int) int {
	n := len(src)
	if i < n && src[i] == '.' {
		i++
		for i < n && (isDigit(src[i]) || src[i] == '_') {
			i++
		}
		return scanExponent(src, i)
	}
	if i+1 < n && src[i] == '0' && (src[i+1] == 'x' || src[i+1] == 'X' || src[i+1] == 'o' || src[i+1] == 'O' || src[i+1] == 'b' || src[i+1] == 'B') {
		i += 2
		for i < n && (isHex(src[i]) || src[i] == '_') {
			i++
		}
		return i
	}
	for i < n && (isDigit(src[i]) || src[i] == '_') {
		i++
	}
	if i < n && src[i] == '.' {
		i++
		for i < n && (isDigit(src[i]) || src[i] == '_') {
			i++
		}
	}
	return scanExponent(src, i)
}

func scanExponent(src string, i int) int {
	n := len(src)
	if i < n && (src[i] == 'e' || src[i] == 'E') {
		j := i + 1
		if j < n && (src[j] == '+' || src[j] == '-') {
			j++
		}
		if j < n && isDigit(src[j]) {
			i = j
			for i < n && (isDigit(src[i]) || src[i] == '_') {
				i++
			}
		}
	}
	if i < n && (src[i] == 'j' || src[i] == 'J') {
		i++
	}
	return i
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
func isHex(b byte) bool {
	return isDigit(b) || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func scanPyString(src string, i int, prefix string) (int, []string, error) {
	if i >= len(src) {
		return 0, nil, fmt.Errorf("unterminated string")
	}
	raw := strings.Contains(strings.ToLower(prefix), "r")
	fstr := strings.Contains(strings.ToLower(prefix), "f")
	quote := src[i]
	triple := false
	if i+2 < len(src) && src[i+1] == quote && src[i+2] == quote {
		triple = true
		i += 3
	} else {
		i++
	}
	endMark := string(quote)
	if triple {
		endMark = strings.Repeat(endMark, 3)
	}
	var exprs []string
	for i < len(src) {
		if !triple && src[i] == '\n' {
			return 0, nil, fmt.Errorf("unterminated string")
		}
		if strings.HasPrefix(src[i:], endMark) {
			return i + len(endMark), exprs, nil
		}
		if src[i] == '\\' {
			if raw && i+1 < len(src) && src[i+1] != quote && !(triple && strings.HasPrefix(src[i+1:], endMark)) {
				i++
				continue
			}
			i += 2
			if i > len(src) {
				i = len(src)
			}
			continue
		}
		if fstr && src[i] == '{' {
			if i+1 < len(src) && src[i+1] == '{' {
				i += 2
				continue
			}
			exprStart := i + 1
			exprEnd, err := scanFExpr(src, exprStart)
			if err != nil {
				return 0, nil, err
			}
			exprs = append(exprs, src[exprStart:exprEnd])
			i = exprEnd
			if i < len(src) && src[i] == '!' {
				i++
				if i < len(src) {
					i++
				}
			}
			if i < len(src) && src[i] == ':' {
				i++
				depth := 0
				for i < len(src) {
					if src[i] == '{' {
						depth++
						i++
						continue
					}
					if src[i] == '}' {
						if depth == 0 {
							break
						}
						depth--
						i++
						continue
					}
					i++
				}
			}
			if i >= len(src) || src[i] != '}' {
				return 0, nil, fmt.Errorf("unterminated f-string expression")
			}
			i++
			continue
		}
		if fstr && src[i] == '}' && i+1 < len(src) && src[i+1] == '}' {
			i += 2
			continue
		}
		i++
	}
	return 0, nil, fmt.Errorf("unterminated string")
}

func scanFExpr(src string, i int) (int, error) {
	depth := 0
	for i < len(src) {
		if end, ok, err := atPyString(src, i); ok || err != nil {
			if err != nil {
				return 0, err
			}
			i = end
			continue
		}
		c := src[i]
		switch c {
		case '(', '[', '{':
			depth++
			i++
		case ')', ']', '}':
			if c == '}' && depth == 0 {
				return i, nil
			}
			if depth == 0 {
				return 0, fmt.Errorf("f-string expression")
			}
			depth--
			i++
		case '!', ':':
			if depth == 0 {
				return i, nil
			}
			i++
		case '\\':
			i += 2
		default:
			_, size := utf8.DecodeRuneInString(src[i:])
			if size < 1 {
				size = 1
			}
			i += size
		}
	}
	return 0, fmt.Errorf("unterminated f-string expression")
}

func atPyString(src string, i int) (int, bool, error) {
	if i >= len(src) {
		return 0, false, nil
	}
	j := i
	for k := 0; k < 3 && j < len(src) && isPrefixByte(src[j]); k++ {
		j++
	}
	for ; j > i; j-- {
		if isStrPrefix(src[i:j]) && j < len(src) && (src[j] == '"' || src[j] == '\'') {
			end, _, err := scanPyString(src, j, src[i:j])
			if err != nil {
				return 0, false, err
			}
			return end, true, nil
		}
	}
	if src[i] == '"' || src[i] == '\'' {
		end, _, err := scanPyString(src, i, "")
		if err != nil {
			return 0, false, err
		}
		return end, true, nil
	}
	return 0, false, nil
}

func isPrefixByte(b byte) bool {
	switch b {
	case 'r', 'R', 'f', 'F', 'b', 'B', 'u', 'U':
		return true
	default:
		return false
	}
}
