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
	return tokenizePyMode(src, false)
}

func tokenizePyExpr(src string) ([]pyTok, error) {
	return tokenizePyMode(src, true)
}

func tokenizePyMode(src string, exprMode bool) ([]pyTok, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	src = strings.TrimPrefix(src, "\uFEFF")
	n := len(src)
	indent := []int{0}
	var out []pyTok
	var bracketStack []byte
	atStart := !exprMode
	line := 1
	i := 0
	for i < n {
		if !exprMode && atStart && len(bracketStack) == 0 {
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
			if !exprMode && len(bracketStack) == 0 {
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
					bracketStack = append(bracketStack, op[0])
				case ")", "]", "}":
					if len(bracketStack) == 0 {
						return nil, fmt.Errorf("line %d: unmatched closing bracket %q", line, op)
					}
					open := bracketStack[len(bracketStack)-1]
					if matchingPyBracket(open) != op[0] {
						return nil, fmt.Errorf("line %d: mismatched bracket %q closed by %q", line, string(open), op)
					}
					bracketStack = bracketStack[:len(bracketStack)-1]
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
	if len(bracketStack) > 0 {
		return nil, fmt.Errorf("line %d: unclosed bracket %q", line, string(bracketStack[len(bracketStack)-1]))
	}
	if !exprMode && !atStart {
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
	if s == "" || len(s) > 2 {
		return false
	}
	lower := strings.ToLower(s)
	seen := map[byte]bool{}
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c != 'r' && c != 'f' && c != 'b' && c != 'u' && c != 't' {
			return false
		}
		if seen[c] {
			return false
		}
		seen[c] = true
	}
	if seen['u'] && (seen['b'] || seen['f'] || seen['r'] || seen['t']) {
		return false
	}
	if seen['b'] && (seen['f'] || seen['t']) {
		return false
	}
	if seen['f'] && seen['t'] {
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
	lowerPrefix := strings.ToLower(prefix)
	raw := strings.Contains(lowerPrefix, "r")
	fstr := strings.ContainsAny(lowerPrefix, "ft")
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
			if i+1 >= len(src) {
				return 0, nil, fmt.Errorf("unterminated string")
			}
			// In both raw and non-raw strings, pairs of backslashes (\\),
			// escaped quotes (\' or \"), and line continuations (\<newline>)
			// consume two bytes. For f/t-strings, a single backslash before '{'
			// or '}' does NOT escape the brace (except \N{...} in non-raw).
			next := src[i+1]
			if !raw && fstr && next == 'N' && i+2 < len(src) && src[i+2] == '{' {
				j := i + 3
				for j < len(src) && src[j] != '}' && src[j] != '\n' {
					j++
				}
				if j < len(src) && src[j] == '}' {
					i = j + 1
					continue
				}
			}
			if next == '\\' || next == quote || next == '\n' || !fstr {
				i += 2
				continue
			}
			i++
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
				for i < len(src) {
					if !triple && src[i] == '\n' {
						return 0, nil, fmt.Errorf("unterminated f-string expression")
					}
					if src[i] == '{' {
						nestedStart := i + 1
						nestedEnd, err := scanFExpr(src, nestedStart)
						if err != nil {
							return 0, nil, err
						}
						exprs = append(exprs, src[nestedStart:nestedEnd])
						i = nestedEnd
						if i < len(src) && src[i] == '!' {
							i += 2
						}
						if i < len(src) && src[i] == ':' {
							i++
							nestedDepth := 0
							for i < len(src) {
								if src[i] == '{' {
									nestedDepth++
								} else if src[i] == '}' {
									if nestedDepth == 0 {
										break
									}
									nestedDepth--
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
					if src[i] == '}' {
						break
					}
					if src[i] == '\\' && i+1 < len(src) {
						i += 2
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

func matchingPyBracket(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	default:
		return 0
	}
}

func scanFExpr(src string, i int) (int, error) {
	var stack []byte
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
		case '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case '(', '[', '{':
			stack = append(stack, c)
			i++
		case ')', ']', '}':
			if c == '}' && len(stack) == 0 {
				return i, nil
			}
			if len(stack) == 0 {
				return 0, fmt.Errorf("unmatched closing bracket %q in f-string expression", string(c))
			}
			open := stack[len(stack)-1]
			if matchingPyBracket(open) != c {
				return 0, fmt.Errorf("mismatched bracket %q closed by %q in f-string expression", string(open), string(c))
			}
			stack = stack[:len(stack)-1]
			i++
		case '!':
			if i+1 < len(src) && src[i+1] == '=' {
				i += 2
				continue
			}
			if len(stack) == 0 {
				return i, nil
			}
			i++
		case ':':
			if len(stack) == 0 {
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
	if src[i] == '"' || src[i] == '\'' {
		end, _, err := scanPyString(src, i, "")
		if err != nil {
			return 0, false, err
		}
		return end, true, nil
	}
	// Do not misread the trailing letter of an identifier (e.g. `if"x"`, `or'a'`)
	// as a string prefix.
	if i > 0 {
		prev, _ := utf8.DecodeLastRuneInString(src[:i])
		if isIdentCont(prev) {
			return 0, false, nil
		}
	}
	j := i
	for k := 0; k < 2 && j < len(src) && isPrefixByte(src[j]); k++ {
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
	return 0, false, nil
}

func isPrefixByte(b byte) bool {
	switch b {
	case 'r', 'R', 'f', 'F', 'b', 'B', 'u', 'U', 't', 'T':
		return true
	default:
		return false
	}
}
