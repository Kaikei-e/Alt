package suppress

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type lineComment struct {
	line int
	raw  string
}

type goComment struct {
	line    int
	endLine int
	raw     string
	before  bool
}

type rustToken struct {
	kind      string
	text      string
	startLine int
	endLine   int
}

func scanPyComments(text string) []lineComment {
	s := &pyCommentScanner{text: text, line: 1}
	s.scan()
	return s.out
}

type pyCommentScanner struct {
	text string
	i    int
	line int
	out  []lineComment
}

func (s *pyCommentScanner) scan() {
	n := len(s.text)
	for s.i < n {
		if s.text[s.i] == '\n' {
			s.line++
			s.i++
			continue
		}
		if s.text[s.i] == '#' {
			s.consumeComment()
			continue
		}
		if s.tryString() {
			continue
		}
		s.i++
	}
}

func (s *pyCommentScanner) consumeComment() {
	start := s.i
	n := len(s.text)
	for s.i < n && s.text[s.i] != '\n' {
		s.i++
	}
	s.out = append(s.out, lineComment{line: s.line, raw: s.text[start:s.i]})
}

func isPyIdentCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isPyStrPrefix(p string) bool {
	if p == "" || len(p) > 2 {
		return false
	}
	lower := strings.ToLower(p)
	if len(lower) == 1 {
		switch lower[0] {
		case 'r', 'f', 'b', 'u', 't':
			return true
		default:
			return false
		}
	}
	switch lower {
	case "rb", "br", "rf", "fr", "rt", "tr":
		return true
	default:
		return false
	}
}

func (s *pyCommentScanner) tryString() bool {
	n := len(s.text)
	if s.i >= n {
		return false
	}
	if s.text[s.i] == '\'' || s.text[s.i] == '"' {
		s.scanString(s.text[s.i], false, false)
		return true
	}
	// Do not misread the trailing letter of an identifier (e.g. `if"x"`, `or'a'`)
	// as a string prefix.
	if s.i > 0 {
		prev, _ := utf8.DecodeLastRuneInString(s.text[:s.i])
		if isPyIdentCont(prev) {
			return false
		}
	}
	for plen := 2; plen >= 1; plen-- {
		j := s.i + plen
		if j < n && (s.text[j] == '\'' || s.text[j] == '"') {
			pref := s.text[s.i:j]
			if isPyStrPrefix(pref) {
				raw := strings.ContainsAny(pref, "rR")
				fstr := strings.ContainsAny(pref, "fFtT")
				s.i = j
				s.scanString(s.text[j], raw, fstr)
				return true
			}
		}
	}
	return false
}

func (s *pyCommentScanner) scanString(q byte, raw, fstr bool) {
	n := len(s.text)
	triple := s.i+2 < n && s.text[s.i+1] == q && s.text[s.i+2] == q
	if triple {
		s.i += 3
	} else {
		s.i++
	}
	for s.i < n {
		ch := s.text[s.i]
		if !triple && ch == '\n' {
			return
		}
		if triple {
			if s.i+2 < n && ch == q && s.text[s.i+1] == q && s.text[s.i+2] == q {
				s.i += 3
				return
			}
		} else if ch == q {
			s.i++
			return
		}
		if ch == '\n' {
			s.line++
			s.i++
			continue
		}
		if ch == '\\' {
			if s.i+1 >= n {
				s.i++
				return
			}
			next := s.text[s.i+1]
			// Non-raw f/t-string \N{...} is a Unicode name escape, not an interpolation.
			if !raw && fstr && next == 'N' && s.i+2 < n && s.text[s.i+2] == '{' {
				j := s.i + 3
				for j < n && s.text[j] != '}' && s.text[j] != '\n' {
					j++
				}
				if j < n && s.text[j] == '}' {
					s.i = j + 1
					continue
				}
			}
			if next == '\r' && s.i+2 < n && s.text[s.i+2] == '\n' {
				s.line++
				s.i += 3
				continue
			}
			if next == '\n' {
				s.line++
				s.i += 2
				continue
			}
			// Both raw and non-raw strings obey backslash-run parity for quotes
			// and backslashes: \\ consumes 2 bytes, \' or \" consumes 2 bytes.
			if next == '\\' || next == q || !fstr {
				s.i += 2
				continue
			}
			s.i++
			continue
		}
		if fstr && ch == '{' {
			if s.i+1 < n && s.text[s.i+1] == '{' {
				s.i += 2
				continue
			}
			s.i++
			s.scanFExpr(triple)
			continue
		}
		if fstr && ch == '}' && s.i+1 < n && s.text[s.i+1] == '}' {
			s.i += 2
			continue
		}
		s.i++
	}
}

func (s *pyCommentScanner) scanFExpr(outerTriple bool) {
	n := len(s.text)
	depth := 0
	for s.i < n {
		if s.tryString() {
			continue
		}
		ch := s.text[s.i]
		switch ch {
		case '\n':
			s.line++
			s.i++
		case '#':
			s.consumeComment()
		case '(', '[', '{':
			depth++
			s.i++
		case ')', ']':
			if depth > 0 {
				depth--
			}
			s.i++
		case '}':
			if depth == 0 {
				s.i++
				return
			}
			depth--
			s.i++
		case ':':
			if depth == 0 {
				s.i++
				s.scanFFormatSpec(outerTriple)
				return
			}
			s.i++
		case '!':
			if s.i+1 < n && s.text[s.i+1] == '=' {
				s.i += 2
				continue
			}
			s.i++
		case '\\':
			if s.i+1 < n {
				if s.text[s.i+1] == '\n' {
					s.line++
				}
				s.i += 2
			} else {
				s.i++
			}
		default:
			s.i++
		}
	}
}

func (s *pyCommentScanner) scanFFormatSpec(outerTriple bool) {
	n := len(s.text)
	for s.i < n {
		ch := s.text[s.i]
		if !outerTriple && ch == '\n' {
			return
		}
		if ch == '\n' {
			s.line++
			s.i++
			continue
		}
		if ch == '{' {
			s.i++
			s.scanFExpr(outerTriple)
			continue
		}
		if ch == '}' {
			s.i++
			return
		}
		if ch == '\\' && s.i+1 < n {
			if s.text[s.i+1] == '\n' {
				s.line++
			}
			s.i += 2
			continue
		}
		s.i++
	}
}

func skipQuoted(text string, pos int, quote byte) (int, int) {
	n := len(text)
	pos++
	newlines := 0
	for pos < n {
		ch := text[pos]
		if ch == '\\' && pos+1 < n {
			if text[pos+1] == '\n' {
				newlines++
			}
			pos += 2
			continue
		}
		if ch == '\n' {
			return pos, newlines
		}
		if ch == quote {
			return pos + 1, newlines
		}
		pos++
	}
	return pos, newlines
}

func scanGoComments(text string) []goComment {
	var comments []goComment
	n := len(text)
	i := 0
	line := 1
	seenPackage := false
	for i < n {
		ch := text[i]
		switch {
		case ch == '\n':
			line++
			i++
		case ch == '"' || ch == '\'':
			var nl int
			i, nl = skipQuoted(text, i, ch)
			line += nl
		case ch == '`':
			end := strings.IndexByte(text[i+1:], '`')
			if end < 0 {
				return comments
			}
			end = i + 1 + end
			line += strings.Count(text[i:end+1], "\n")
			i = end + 1
		case ch == '/' && i+1 < n && text[i+1] == '/':
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				end = n
			} else {
				end = i + end
			}
			comments = append(comments, goComment{line, line, text[i:end], !seenPackage})
			i = end
		case ch == '/' && i+1 < n && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				end = n
			} else {
				end = i + 2 + end + 2
			}
			endLine := line + strings.Count(text[i:end], "\n")
			comments = append(comments, goComment{line, endLine, text[i:end], !seenPackage})
			line = endLine
			i = end
		case !seenPackage && (isASCIILetter(ch) || ch == '_'):
			start := i
			for i < n && (isASCIIAlnum(text[i]) || text[i] == '_') {
				i++
			}
			if text[start:i] == "package" {
				seenPackage = true
			}
		default:
			i++
		}
	}
	return comments
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isASCIIAlnum(b byte) bool {
	return isASCIILetter(b) || (b >= '0' && b <= '9')
}

func goCommentMaps(comments []goComment) (map[int]string, map[int][]string) {
	byLine := map[int]string{}
	onLine := map[int][]string{}
	for _, c := range comments {
		onLine[c.line] = append(onLine[c.line], c.raw)
		if c.line == c.endLine {
			byLine[c.line] = stripCommentDelimiters(c.raw)
		}
	}
	return byLine, onLine
}

func trailingSameLine(onLine map[int][]string, start int, raw string) string {
	list := onLine[start]
	idx := 0
	for i, item := range list {
		if item == raw {
			idx = i
			break
		}
	}
	if idx+1 >= len(list) {
		return ""
	}
	return strings.Join(list[idx+1:], " ")
}

func extractGoNolintReason(rest, trailing string) string {
	body := rest
	if strings.HasSuffix(body, "*/") && len(body) >= 2 {
		body = body[:len(body)-2]
	}
	if i := strings.Index(body, "//"); i >= 0 {
		return strings.TrimSpace(body[i+2:])
	}
	if i := strings.Index(trailing, "//"); i >= 0 {
		return strings.TrimSpace(trailing[i+2:])
	}
	return ""
}

func skipRustRawString(text string, pos int) (int, int, bool) {
	n := len(text)
	j := pos
	if j < n && (text[j] == 'b' || text[j] == 'c') {
		j++
	}
	if j >= n || text[j] != 'r' {
		return 0, 0, false
	}
	j++
	hashes := 0
	for j < n && text[j] == '#' {
		hashes++
		j++
	}
	if j >= n || text[j] != '"' {
		return 0, 0, false
	}
	closing := "\"" + strings.Repeat("#", hashes)
	end := strings.Index(text[j+1:], closing)
	if end < 0 {
		end = n
	} else {
		end = j + 1 + end + len(closing)
	}
	return end, strings.Count(text[pos:end], "\n"), true
}

func skipRustBlockComment(text string, pos int) (int, int) {
	n := len(text)
	depth := 1
	i := pos + 2
	newlines := 0
	for i < n && depth > 0 {
		if text[i] == '\n' {
			newlines++
			i++
			continue
		}
		if i+1 < n && text[i] == '/' && text[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if i+1 < n && text[i] == '*' && text[i+1] == '/' {
			depth--
			i += 2
			continue
		}
		i++
	}
	return i, newlines
}

func skipRustCharLiteral(text string, pos int) (int, bool) {
	n := len(text)
	if pos+2 >= n || text[pos] != '\'' {
		return 0, false
	}
	if text[pos+1] == '\\' {
		j := pos + 2
		for j < n && text[j] != '\'' && text[j] != '\n' {
			j++
		}
		if j < n && text[j] == '\'' {
			return j + 1, true
		}
		return 0, false
	}
	if text[pos+1] != '\n' && text[pos+2] == '\'' {
		return pos + 3, true
	}
	return 0, false
}

func matchRustAttrStart(text string, pos int) (bool, int, bool) {
	n := len(text)
	if pos >= n || text[pos] != '#' {
		return false, 0, false
	}
	j := pos + 1
	for j < n && unicode.IsSpace(rune(text[j])) && text[j] < 128 {
		j++
	}
	isInner := false
	if j < n && text[j] == '!' {
		isInner = true
		j++
		for j < n && unicode.IsSpace(rune(text[j])) && text[j] < 128 {
			j++
		}
	}
	if j < n && text[j] == '[' {
		return isInner, j, true
	}
	return false, 0, false
}

func readRustAttribute(text string, pos, startLine int, isInner bool, bracketPos int) (rustToken, int, int) {
	n := len(text)
	line := startLine + strings.Count(text[pos:bracketPos], "\n")
	i := bracketPos
	depth := 0
	for i < n {
		ch := text[i]
		switch ch {
		case '\n':
			line++
			i++
		case '"':
			var nl int
			i, nl = skipQuoted(text, i, '"')
			line += nl
		case '[':
			depth++
			i++
		case ']':
			depth--
			i++
			if depth == 0 {
				kind := "ATTR_OUTER"
				if isInner {
					kind = "ATTR_INNER"
				}
				return rustToken{kind, text[pos:i], startLine, line}, i, line
			}
		default:
			i++
		}
	}
	kind := "ATTR_OUTER"
	if isInner {
		kind = "ATTR_INNER"
	}
	return rustToken{kind, text[pos:i], startLine, line}, i, line
}

func scanRustTokens(text string) []rustToken {
	var tokens []rustToken
	n := len(text)
	i := 0
	line := 1
	for i < n {
		ch := text[i]
		if ch == '\n' {
			line++
			i++
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\f' || ch == '\v' {
			i++
			continue
		}
		if tok, ni, nline, ok := scanRustCommentOrAttr(text, i, line); ok {
			i, line = ni, nline
			if tok != nil {
				tokens = append(tokens, *tok)
			}
			continue
		}
		tok, ni, nline := scanRustStringOrWord(text, i, line)
		i, line = ni, nline
		if tok != nil {
			tokens = append(tokens, *tok)
		}
	}
	return tokens
}

func scanRustCommentOrAttr(text string, pos, line int) (*rustToken, int, int, bool) {
	n := len(text)
	ch := text[pos]
	if ch == '/' && pos+1 < n && text[pos+1] == '/' {
		end := strings.IndexByte(text[pos:], '\n')
		if end < 0 {
			end = n
		} else {
			end = pos + end
		}
		tok := rustToken{"COMMENT", text[pos:end], line, line}
		return &tok, end, line, true
	}
	if ch == '/' && pos+1 < n && text[pos+1] == '*' {
		end, nl := skipRustBlockComment(text, pos)
		tok := rustToken{"COMMENT", text[pos:end], line, line + nl}
		return &tok, end, line + nl, true
	}
	if ch == '#' {
		isInner, bracket, ok := matchRustAttrStart(text, pos)
		if ok {
			tok, ni, nline := readRustAttribute(text, pos, line, isInner, bracket)
			return &tok, ni, nline, true
		}
	}
	return nil, 0, 0, false
}

func scanRustStringOrWord(text string, pos, line int) (*rustToken, int, int) {
	n := len(text)
	ch := text[pos]
	if ch == '"' {
		end, nl := skipQuoted(text, pos, '"')
		return nil, end, line + nl
	}
	if ch == 'r' || ch == 'b' || ch == 'c' {
		if end, nl, ok := skipRustRawString(text, pos); ok {
			return nil, end, line + nl
		}
		if (ch == 'b' || ch == 'c') && pos+1 < n && text[pos+1] == '"' {
			end, nl := skipQuoted(text, pos+1, '"')
			return nil, end, line + nl
		}
	}
	if ch == '\'' {
		if end, ok := skipRustCharLiteral(text, pos); ok {
			return nil, end, line
		}
		return nil, pos + 1, line
	}
	if isASCIILetter(ch) || ch == '_' {
		end := pos
		for end < n && (isASCIIAlnum(text[end]) || text[end] == '_') {
			end++
		}
		tok := rustToken{"WORD", text[pos:end], line, line}
		return &tok, end, line
	}
	r, size := utf8.DecodeRuneInString(text[pos:])
	if r == utf8.RuneError && size == 1 {
		size = 1
	}
	tok := rustToken{"PUNCT", text[pos : pos+size], line, line}
	return &tok, pos + size, line
}

func rustAttrAppliesToMod(tokens []rustToken, attrIndex int) bool {
	j := attrIndex + 1
	n := len(tokens)
	for j < n && (tokens[j].kind == "COMMENT" || tokens[j].kind == "ATTR_OUTER") {
		j++
	}
	if j < n && tokens[j].kind == "WORD" && tokens[j].text == "pub" {
		j++
		if j < n && tokens[j].kind == "PUNCT" && tokens[j].text == "(" {
			depth := 1
			j++
			for j < n && depth > 0 {
				if tokens[j].kind == "PUNCT" && tokens[j].text == "(" {
					depth++
				} else if tokens[j].kind == "PUNCT" && tokens[j].text == ")" {
					depth--
				}
				j++
			}
		}
	}
	for j < n && tokens[j].kind == "COMMENT" {
		j++
	}
	return j+1 < n && tokens[j].kind == "WORD" && tokens[j].text == "mod" && tokens[j+1].kind == "WORD"
}

func rustContextForAttr(tokens []rustToken, attrIndex int) string {
	attr := tokens[attrIndex]
	topLine := attr.startLine
	k := attrIndex - 1
	for k >= 0 && tokens[k].kind == "ATTR_OUTER" && tokens[k].endLine >= topLine-1 {
		topLine = tokens[k].startLine
		k--
	}
	var preceding []string
	expected := topLine - 1
	for k >= 0 && tokens[k].kind == "COMMENT" && tokens[k].endLine == expected {
		preceding = append(preceding, stripCommentDelimiters(tokens[k].text))
		expected = tokens[k].startLine - 1
		k--
	}
	for i, j := 0, len(preceding)-1; i < j; i, j = i+1, j-1 {
		preceding[i], preceding[j] = preceding[j], preceding[i]
	}
	j := attrIndex + 1
	for j < len(tokens) && tokens[j].startLine == attr.endLine {
		if tokens[j].kind == "COMMENT" {
			preceding = append(preceding, stripCommentDelimiters(tokens[j].text))
		}
		j++
	}
	return strings.Join(preceding, "\n")
}

func skipTSTemplate(text string, pos int) (int, int) {
	n := len(text)
	i := pos + 1
	newlines := 0
	for i < n {
		ch := text[i]
		if ch == '\\' && i+1 < n {
			if text[i+1] == '\n' {
				newlines++
			}
			i += 2
			continue
		}
		if ch == '\n' {
			newlines++
		} else if ch == '`' {
			return i + 1, newlines
		}
		i++
	}
	return i, newlines
}

type spanComment struct {
	line    int
	endLine int
	raw     string
}

func scanTSComments(text string, svelte bool) []spanComment {
	var comments []spanComment
	n := len(text)
	i := 0
	line := 1
	for i < n {
		ch := text[i]
		switch {
		case ch == '\n':
			line++
			i++
		case ch == '"' || ch == '\'':
			var nl int
			i, nl = skipQuoted(text, i, ch)
			line += nl
		case ch == '`':
			var nl int
			i, nl = skipTSTemplate(text, i)
			line += nl
		case ch == '/' && i+1 < n && text[i+1] == '/':
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				end = n
			} else {
				end = i + end
			}
			comments = append(comments, spanComment{line, line, text[i:end]})
			i = end
		case ch == '/' && i+1 < n && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				end = n
			} else {
				end = i + 2 + end + 2
			}
			endLine := line + strings.Count(text[i:end], "\n")
			comments = append(comments, spanComment{line, endLine, text[i:end]})
			line = endLine
			i = end
		case svelte && i+3 < n && text[i:i+4] == "<!--":
			end := strings.Index(text[i+4:], "-->")
			if end < 0 {
				end = n
			} else {
				end = i + 4 + end + 3
			}
			endLine := line + strings.Count(text[i:end], "\n")
			comments = append(comments, spanComment{line, endLine, text[i:end]})
			line = endLine
			i = end
		default:
			i++
		}
	}
	return comments
}
