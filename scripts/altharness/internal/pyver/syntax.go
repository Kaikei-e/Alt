package pyver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode"
	"unicode/utf8"
)

// findUnparenthesizedExceptLines reports 1-based lines with PEP 758
// unparenthesized multi-exception `except` clauses. A tokenizer error yields
// no hits, matching tokenize.TokenError handling in the Python checker.
func findUnparenthesizedExceptLines(source string) []int {
	toks, err := tokenizePython(source)
	if err != nil {
		return nil
	}
	var lines []int
	seen := map[int]struct{}{}
	for i, tok := range toks {
		if tok.kind != tokName || tok.text != "except" {
			continue
		}
		if unparenthesizedExcept(toks, i) {
			if _, ok := seen[tok.line]; !ok {
				seen[tok.line] = struct{}{}
				lines = append(lines, tok.line)
			}
		}
	}
	return lines
}

const (
	tokName = iota
	tokOp
	tokNewline
)

type pyToken struct {
	kind int
	text string
	line int
}

func unparenthesizedExcept(tokens []pyToken, start int) bool {
	depth := 0
	idx := start + 1
	if idx < len(tokens) && tokens[idx].kind == tokOp && tokens[idx].text == "*" {
		idx++
	}
	for idx < len(tokens) {
		tok := tokens[idx]
		if tok.kind == tokOp {
			switch tok.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth > 0 {
					depth--
				}
			case ",":
				if depth == 0 {
					return true
				}
			case ":":
				if depth == 0 {
					return false
				}
			}
		} else if depth == 0 && (tok.kind == tokNewline || (tok.kind == tokName && tok.text == "as")) {
			return false
		}
		idx++
	}
	return false
}

type scanner struct {
	src  string
	i    int
	line int
	toks []pyToken
	bad  bool
}

func tokenizePython(source string) ([]pyToken, error) {
	s := &scanner{src: source, line: 1}
	for s.i < len(s.src) && !s.bad {
		s.skipSpaceAndComment()
		if s.i >= len(s.src) || s.bad {
			break
		}
		if s.tryString() {
			continue
		}
		r, size := utf8.DecodeRuneInString(s.src[s.i:])
		if r == utf8.RuneError && size == 1 {
			s.bad = true
			break
		}
		if r == '\n' || r == '\r' {
			s.emit(tokNewline, "\n")
			s.i += size
			if r == '\r' && s.peekByte(0) == '\n' {
				s.i++
			}
			s.line++
			continue
		}
		if r == '\\' && s.peekByte(1) == '\n' {
			s.i += 2
			s.line++
			continue
		}
		if r == '\\' && s.peekByte(1) == '\r' {
			s.i += 2
			if s.peekByte(0) == '\n' {
				s.i++
			}
			s.line++
			continue
		}
		if isNameStart(r) {
			start := s.i
			line := s.line
			s.i += size
			for s.i < len(s.src) {
				nr, nsize := utf8.DecodeRuneInString(s.src[s.i:])
				if !isNameCont(nr) {
					break
				}
				s.i += nsize
			}
			s.toks = append(s.toks, pyToken{kind: tokName, text: s.src[start:s.i], line: line})
			continue
		}
		if s.tryNumber() {
			continue
		}
		if op, n, ok := s.matchOp(); ok {
			s.emit(tokOp, op)
			s.i += n
			continue
		}
		s.bad = true
	}
	if s.bad {
		return nil, fmt.Errorf("tokenize")
	}
	return s.toks, nil
}

func (s *scanner) emit(kind int, text string) {
	s.toks = append(s.toks, pyToken{kind: kind, text: text, line: s.line})
}

func (s *scanner) peekByte(n int) byte {
	j := s.i + n
	if j < 0 || j >= len(s.src) {
		return 0
	}
	return s.src[j]
}

func (s *scanner) skipSpaceAndComment() {
	for s.i < len(s.src) && !s.bad {
		r, size := utf8.DecodeRuneInString(s.src[s.i:])
		if r == '\n' || r == '\r' {
			return
		}
		if r == '#' {
			for s.i < len(s.src) && s.src[s.i] != '\n' {
				s.i++
			}
			return
		}
		if r == ' ' || r == '\t' || r == '\f' || r == '\v' {
			s.i += size
			continue
		}
		if unicode.IsSpace(r) && r != '\n' {
			s.i += size
			continue
		}
		return
	}
}

func isNameStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isNameCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (s *scanner) tryNumber() bool {
	if s.i >= len(s.src) {
		return false
	}
	rest := s.src[s.i:]
	if len(rest) >= 2 && rest[0] == '0' && (rest[1] == 'x' || rest[1] == 'X' || rest[1] == 'o' || rest[1] == 'O' || rest[1] == 'b' || rest[1] == 'B') {
		s.i += 2
		for s.i < len(s.src) {
			c := s.src[s.i]
			if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '_' {
				s.i++
				continue
			}
			break
		}
		return true
	}
	if rest[0] >= '0' && rest[0] <= '9' || (rest[0] == '.' && len(rest) > 1 && rest[1] >= '0' && rest[1] <= '9') {
		start := s.i
		if rest[0] == '.' {
			s.i++
		}
		for s.i < len(s.src) {
			c := s.src[s.i]
			if (c >= '0' && c <= '9') || c == '_' || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
				if (c == '+' || c == '-') && s.i > start {
					prev := s.src[s.i-1]
					if prev != 'e' && prev != 'E' {
						break
					}
				}
				s.i++
				continue
			}
			break
		}
		return s.i > start
	}
	return false
}

func (s *scanner) matchOp() (string, int, bool) {
	ops := []string{
		"**=", "//=", "<<=", ">>=", "...", ":=", "**", "//", "<<", ">>", "==", "!=", "<=", ">=",
		"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "->",
	}
	for _, op := range ops {
		if strings.HasPrefix(s.src[s.i:], op) {
			return op, len(op), true
		}
	}
	if s.i < len(s.src) {
		c := s.src[s.i]
		if strings.ContainsRune("()[]{}:,.+-*/%@&|^~<>", rune(c)) {
			return s.src[s.i : s.i+1], 1, true
		}
	}
	return "", 0, false
}

func isStrPrefix(p string) bool {
	switch strings.ToLower(p) {
	case "r", "u", "b", "f", "rb", "br", "rf", "fr":
		return true
	default:
		return false
	}
}

func (s *scanner) tryString() bool {
	rest := s.src[s.i:]
	bestLen := -1
	for n := 3; n >= 0; n-- {
		if len(rest) < n {
			continue
		}
		pref := rest[:n]
		if n > 0 && !isPrefixLetters(pref) {
			continue
		}
		if n > 0 && !isStrPrefix(pref) {
			continue
		}
		if len(rest) > n && (rest[n] == '\'' || rest[n] == '"') {
			bestLen = n
			break
		}
	}
	if bestLen < 0 {
		return false
	}
	pref := rest[:bestLen]
	s.i += bestLen
	lower := strings.ToLower(pref)
	s.consumeString(strings.Contains(lower, "f"), strings.Contains(lower, "r"))
	return true
}

func isPrefixLetters(p string) bool {
	for i := 0; i < len(p); i++ {
		c := p[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

func (s *scanner) consumeString(isF, isRaw bool) {
	if s.i >= len(s.src) {
		s.bad = true
		return
	}
	quote := s.src[s.i]
	triple := strings.HasPrefix(s.src[s.i:], strings.Repeat(string(quote), 3))
	if triple {
		s.i += 3
	} else {
		s.i++
	}
	for s.i < len(s.src) {
		c := s.src[s.i]
		if c == '\\' {
			if isRaw {
				if s.i+1 < len(s.src) && s.src[s.i+1] == quote {
					s.i += 2
					continue
				}
				s.i++
				continue
			}
			if s.i+1 >= len(s.src) {
				s.bad = true
				return
			}
			if s.src[s.i+1] == '\n' {
				s.i += 2
				s.line++
				continue
			}
			if s.src[s.i+1] == '\r' {
				s.i += 2
				if s.i < len(s.src) && s.src[s.i] == '\n' {
					s.i++
				}
				s.line++
				continue
			}
			s.i += 2
			continue
		}
		if triple && strings.HasPrefix(s.src[s.i:], strings.Repeat(string(quote), 3)) {
			s.i += 3
			return
		}
		if !triple && c == quote {
			s.i++
			return
		}
		if isF && c == '{' {
			if s.i+1 < len(s.src) && s.src[s.i+1] == '{' {
				s.i += 2
				continue
			}
			s.i++
			s.skipFExpr()
			continue
		}
		if isF && c == '}' && s.i+1 < len(s.src) && s.src[s.i+1] == '}' {
			s.i += 2
			continue
		}
		if c == '\n' {
			s.line++
		}
		s.i++
	}
	s.bad = true
}

func (s *scanner) skipFExpr() {
	depth := 1
	for s.i < len(s.src) && depth > 0 && !s.bad {
		if s.tryString() {
			continue
		}
		c := s.src[s.i]
		if c == '{' {
			depth++
			s.i++
			continue
		}
		if c == '}' {
			depth--
			s.i++
			continue
		}
		if c == '\n' {
			s.line++
		}
		s.i++
	}
	if depth != 0 {
		s.bad = true
	}
}

type astRequest struct {
	Src  string `json:"src"`
	Name string `json:"name"`
	Maj  int    `json:"maj"`
	Min  int    `json:"min"`
}

type astResponse struct {
	OK   bool   `json:"ok"`
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

const astHelper = `
import ast, json, sys
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = json.loads(line)
    try:
        ast.parse(req["src"], filename=req["name"], feature_version=(int(req["maj"]), int(req["min"])))
        sys.stdout.write('{"ok": true}\n')
    except SyntaxError as exc:
        sys.stdout.write(json.dumps({"ok": False, "line": exc.lineno or 1, "msg": exc.msg}) + "\n")
    except Exception as exc:
        sys.stdout.write(json.dumps({"ok": False, "line": 1, "msg": str(exc)}) + "\n")
    sys.stdout.flush()
`

func astParseBatch(ctx context.Context, bin string, reqs []astRequest) ([]astResponse, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, bin, "-u", "-c", astHelper)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ast stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ast stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}
	writeErr := make(chan error, 1)
	go func() {
		enc := json.NewEncoder(stdin)
		var err error
		for _, req := range reqs {
			if err = enc.Encode(req); err != nil {
				break
			}
		}
		closeErr := stdin.Close()
		if err == nil {
			err = closeErr
		}
		writeErr <- err
	}()
	var out []astResponse
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var resp astResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			_ = cmd.Wait()
			return nil, fmt.Errorf("ast response: %w", err)
		}
		out = append(out, resp)
	}
	scanErr := sc.Err()
	if err := <-writeErr; err != nil && !isClosedPipe(err) {
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ast parse helper: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	if scanErr != nil {
		return nil, scanErr
	}
	if len(out) != len(reqs) {
		return nil, fmt.Errorf("ast parse helper returned %d results for %d files", len(out), len(reqs))
	}
	return out, nil
}

func isClosedPipe(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF || err == io.ErrClosedPipe {
		return true
	}
	return strings.Contains(err.Error(), "closed pipe")
}
