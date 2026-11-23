package suppress

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// tomlDecodeError matches tomllib.TOMLDecodeError text closely enough for CLI parity.
type tomlDecodeError struct{ msg string }

func (e *tomlDecodeError) Error() string { return e.msg }

type tomlTable struct {
	v        map[string]any
	inline   bool
	explicit bool
}

type tomlParser struct {
	s    string
	i    int
	line int
	col  int
}

func parseTOML(s string) (map[string]any, error) {
	if strings.HasPrefix(s, "\uFEFF") {
		s = s[len("\uFEFF"):]
	}
	p := &tomlParser{s: s, line: 1, col: 1}
	root := &tomlTable{v: map[string]any{}, explicit: true}
	current := root
	for {
		p.skipTrivia()
		if p.eof() {
			break
		}
		if p.peekByte() == '[' {
			next, err := p.parseHeader(root)
			if err != nil {
				return nil, err
			}
			current = next
			continue
		}
		if err := p.parseKV(current); err != nil {
			return nil, err
		}
	}
	out, err := unwrapTable(root)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func unwrapTable(t *tomlTable) (map[string]any, error) {
	out := make(map[string]any, len(t.v))
	for k, v := range t.v {
		u, err := unwrapVal(v)
		if err != nil {
			return nil, err
		}
		out[k] = u
	}
	return out, nil
}

func unwrapVal(v any) (any, error) {
	switch t := v.(type) {
	case *tomlTable:
		return unwrapTable(t)
	case []any:
		out := make([]any, len(t))
		for i, el := range t {
			u, err := unwrapVal(el)
			if err != nil {
				return nil, err
			}
			out[i] = u
		}
		return out, nil
	default:
		return v, nil
	}
}

func (p *tomlParser) eof() bool { return p.i >= len(p.s) }

func (p *tomlParser) peekByte() byte {
	if p.eof() {
		return 0
	}
	return p.s[p.i]
}

func (p *tomlParser) bump() rune {
	if p.eof() {
		return 0
	}
	r, w := utf8.DecodeRuneInString(p.s[p.i:])
	p.i += w
	if r == '\r' {
		if !p.eof() && p.s[p.i] == '\n' {
			p.i++
		}
		p.line++
		p.col = 1
		return '\n'
	}
	if r == '\n' {
		p.line++
		p.col = 1
		return '\n'
	}
	p.col++
	return r
}

func (p *tomlParser) errf(format string, args ...any) error {
	return &tomlDecodeError{msg: fmt.Sprintf(format+" (at line %d, column %d)", append(args, p.line, p.col)...)}
}

func (p *tomlParser) skipTrivia() {
	for !p.eof() {
		c := p.peekByte()
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.bump()
			continue
		}
		if c == '#' {
			for !p.eof() {
				c = p.peekByte()
				if c == '\n' || c == '\r' {
					break
				}
				p.bump()
			}
			continue
		}
		return
	}
}

func (p *tomlParser) skipSpaces() {
	for !p.eof() {
		c := p.peekByte()
		if c != ' ' && c != '\t' {
			return
		}
		p.bump()
	}
}

func (p *tomlParser) parseHeader(root *tomlTable) (*tomlTable, error) {
	// caller is on '['
	p.bump()
	array := false
	if p.peekByte() == '[' {
		array = true
		p.bump()
	}
	p.skipTrivia()
	parts, err := p.parseKeyParts()
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, p.errf("Invalid statement")
	}
	p.skipSpaces()
	if array {
		if p.peekByte() != ']' || p.i+1 >= len(p.s) || p.s[p.i+1] != ']' {
			return nil, p.errf("Expected ']]' at the end of an array declaration")
		}
		p.bump()
		p.bump()
		p.skipSpaces()
		if err := p.endOfHeader(); err != nil {
			return nil, err
		}
		return p.openArray(root, parts)
	}
	if p.peekByte() != ']' {
		return nil, p.errf("Expected ']' at the end of a table declaration")
	}
	p.bump()
	p.skipSpaces()
	if err := p.endOfHeader(); err != nil {
		return nil, err
	}
	return p.openTable(root, parts)
}

func (p *tomlParser) endOfHeader() error {
	if p.eof() || p.peekByte() == '\n' || p.peekByte() == '\r' || p.peekByte() == '#' {
		return nil
	}
	return p.errf("Invalid statement")
}

func (p *tomlParser) openTable(root *tomlTable, parts []string) (*tomlTable, error) {
	cur := root
	for i, part := range parts {
		last := i == len(parts)-1
		existing, ok := cur.v[part]
		if last {
			if !ok {
				nt := &tomlTable{v: map[string]any{}, explicit: true}
				cur.v[part] = nt
				return nt, nil
			}
			et, isT := existing.(*tomlTable)
			if !isT || et.inline || et.explicit {
				return nil, p.errf("Cannot redefine table")
			}
			et.explicit = true
			return et, nil
		}
		if !ok {
			nt := &tomlTable{v: map[string]any{}}
			cur.v[part] = nt
			cur = nt
			continue
		}
		et, isT := existing.(*tomlTable)
		if !isT || et.inline {
			return nil, p.errf("Cannot extend non-table")
		}
		cur = et
	}
	return cur, nil
}

func (p *tomlParser) openArray(root *tomlTable, parts []string) (*tomlTable, error) {
	cur := root
	for i, part := range parts[:len(parts)-1] {
		_ = i
		existing, ok := cur.v[part]
		if !ok {
			nt := &tomlTable{v: map[string]any{}}
			cur.v[part] = nt
			cur = nt
			continue
		}
		et, isT := existing.(*tomlTable)
		if !isT || et.inline {
			return nil, p.errf("Cannot extend non-table")
		}
		cur = et
	}
	last := parts[len(parts)-1]
	existing, ok := cur.v[last]
	var arr []any
	if ok {
		var isArr bool
		arr, isArr = existing.([]any)
		if !isArr {
			return nil, p.errf("Cannot redefine existing key")
		}
	}
	elem := &tomlTable{v: map[string]any{}, explicit: true}
	arr = append(arr, elem)
	cur.v[last] = arr
	return elem, nil
}

func (p *tomlParser) parseKV(cur *tomlTable) error {
	parts, err := p.parseKeyParts()
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return p.errf("Invalid statement")
	}
	p.skipTrivia()
	if p.peekByte() != '=' {
		return p.errf("Expected '=' after a key in a key/value pair")
	}
	p.bump()
	p.skipTrivia()
	val, err := p.parseValue()
	if err != nil {
		return err
	}
	return p.put(cur, parts, val)
}

func (p *tomlParser) put(cur *tomlTable, parts []string, val any) error {
	for _, part := range parts[:len(parts)-1] {
		existing, ok := cur.v[part]
		if !ok {
			nt := &tomlTable{v: map[string]any{}}
			cur.v[part] = nt
			cur = nt
			continue
		}
		et, isT := existing.(*tomlTable)
		if !isT || et.inline {
			return p.errf("Cannot extend non-table")
		}
		cur = et
	}
	last := parts[len(parts)-1]
	if _, exists := cur.v[last]; exists {
		return p.errf("Cannot redefine existing key")
	}
	cur.v[last] = val
	return nil
}

func (p *tomlParser) parseKeyParts() ([]string, error) {
	var parts []string
	for {
		p.skipTrivia()
		part, err := p.parseKeyOne()
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
		saved := p.snapshot()
		p.skipTrivia()
		if p.peekByte() == '.' {
			p.bump()
			continue
		}
		p.restore(saved)
		return parts, nil
	}
}

type snap struct{ i, line, col int }

func (p *tomlParser) snapshot() snap { return snap{p.i, p.line, p.col} }
func (p *tomlParser) restore(s snap) { p.i, p.line, p.col = s.i, s.line, s.col }

func (p *tomlParser) parseKeyOne() (string, error) {
	if p.eof() {
		return "", p.errf("Expected key")
	}
	c := p.peekByte()
	if c == '"' || c == '\'' {
		v, err := p.parseString()
		if err != nil {
			return "", err
		}
		s, ok := v.(string)
		if !ok {
			return "", p.errf("Expected key")
		}
		return s, nil
	}
	start := p.i
	for !p.eof() {
		c = p.peekByte()
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			p.bump()
			continue
		}
		break
	}
	if p.i == start {
		return "", p.errf("Invalid statement")
	}
	return p.s[start:p.i], nil
}

func (p *tomlParser) parseValue() (any, error) {
	if p.eof() {
		return nil, p.errf("Expected value")
	}
	c := p.peekByte()
	switch c {
	case '"', '\'':
		return p.parseString()
	case '[':
		return p.parseArray()
	case '{':
		return p.parseInline()
	case 't':
		if p.consumeWord("true") {
			return true, nil
		}
	case 'f':
		if p.consumeWord("false") {
			return false, nil
		}
	}
	return p.parseAtom()
}

func (p *tomlParser) consumeWord(w string) bool {
	if strings.HasPrefix(p.s[p.i:], w) {
		rest := p.i + len(w)
		if rest < len(p.s) {
			c := p.s[rest]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
				return false
			}
		}
		for range w {
			p.bump()
		}
		return true
	}
	return false
}

func (p *tomlParser) parseString() (any, error) {
	quote := p.peekByte()
	triple := p.i+2 < len(p.s) && p.s[p.i] == quote && p.s[p.i+1] == quote && p.s[p.i+2] == quote
	if triple {
		p.bump()
		p.bump()
		p.bump()
		return p.parseMultiString(quote)
	}
	p.bump()
	var b strings.Builder
	for !p.eof() {
		c := p.peekByte()
		if c == '\n' || c == '\r' {
			return nil, p.errf("Unclosed string")
		}
		if c == quote {
			p.bump()
			return b.String(), nil
		}
		if quote == '"' && c == '\\' {
			esc, err := p.parseEscape(false)
			if err != nil {
				return nil, err
			}
			b.WriteString(esc)
			continue
		}
		r := p.bump()
		b.WriteRune(r)
	}
	return nil, p.errf("Unclosed string")
}

func (p *tomlParser) parseMultiString(quote byte) (any, error) {
	// A newline right after the opening quotes is dropped.
	if !p.eof() && (p.peekByte() == '\n' || p.peekByte() == '\r') {
		p.bump()
	}
	var b strings.Builder
	for !p.eof() {
		if p.i+2 < len(p.s) && p.s[p.i] == quote && p.s[p.i+1] == quote && p.s[p.i+2] == quote {
			p.bump()
			p.bump()
			p.bump()
			return b.String(), nil
		}
		c := p.peekByte()
		if quote == '"' && c == '\\' {
			// Line-ending backslash trims the following whitespace.
			if p.i+1 < len(p.s) && (p.s[p.i+1] == '\n' || p.s[p.i+1] == '\r') {
				p.bump()
				for !p.eof() {
					n := p.peekByte()
					if n == ' ' || n == '\t' || n == '\n' || n == '\r' {
						p.bump()
						continue
					}
					break
				}
				continue
			}
			esc, err := p.parseEscape(true)
			if err != nil {
				return nil, err
			}
			b.WriteString(esc)
			continue
		}
		r := p.bump()
		b.WriteRune(r)
	}
	return nil, p.errf("Unclosed string")
}

func (p *tomlParser) parseEscape(multi bool) (string, error) {
	p.bump() // backslash
	if p.eof() {
		return "", p.errf("Unclosed string")
	}
	c := p.bump()
	switch c {
	case 'b':
		return "\b", nil
	case 't':
		return "\t", nil
	case 'n':
		return "\n", nil
	case 'f':
		return "\f", nil
	case 'r':
		return "\r", nil
	case '"':
		return "\"", nil
	case '\\':
		return "\\", nil
	case 'u':
		return p.parseU(4)
	case 'U':
		return p.parseU(8)
	case '\n':
		if multi {
			return "", nil
		}
		return "", p.errf("Invalid escape")
	default:
		return "", p.errf("Invalid escape")
	}
}

func (p *tomlParser) parseU(n int) (string, error) {
	if p.i+n > len(p.s) {
		return "", p.errf("Invalid unicode escape")
	}
	hex := p.s[p.i : p.i+n]
	for range n {
		p.bump()
	}
	v, err := strconv.ParseInt(hex, 16, 32)
	if err != nil || !utf8.ValidRune(rune(v)) {
		return "", p.errf("Invalid unicode escape")
	}
	return string(rune(v)), nil
}

func (p *tomlParser) parseArray() (any, error) {
	p.bump() // [
	var arr []any
	for {
		p.skipTrivia()
		if p.eof() {
			return nil, p.errf("Unclosed array")
		}
		if p.peekByte() == ']' {
			p.bump()
			return arr, nil
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
		p.skipTrivia()
		if p.peekByte() == ',' {
			p.bump()
			continue
		}
		if p.peekByte() == ']' {
			p.bump()
			return arr, nil
		}
		return nil, p.errf("Expected ',' or ']' after array element")
	}
}

func (p *tomlParser) parseInline() (any, error) {
	p.bump() // {
	t := &tomlTable{v: map[string]any{}, inline: true, explicit: true}
	for {
		p.skipTrivia()
		if p.eof() {
			return nil, p.errf("Unclosed inline table")
		}
		if p.peekByte() == '}' {
			p.bump()
			return t, nil
		}
		parts, err := p.parseKeyParts()
		if err != nil {
			return nil, err
		}
		p.skipTrivia()
		if p.peekByte() != '=' {
			return nil, p.errf("Expected '=' after a key in a key/value pair")
		}
		p.bump()
		p.skipTrivia()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		if err := p.put(t, parts, val); err != nil {
			return nil, err
		}
		p.skipTrivia()
		if p.peekByte() == ',' {
			p.bump()
			continue
		}
		if p.peekByte() == '}' {
			p.bump()
			return t, nil
		}
		return nil, p.errf("Expected ',' or '}' after inline table element")
	}
}

func (p *tomlParser) parseAtom() (any, error) {
	startLine, startCol := p.line, p.col
	start := p.i
	for !p.eof() {
		c := p.peekByte()
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '#' || c == ',' || c == ']' || c == '}' {
			break
		}
		p.bump()
	}
	if p.i == start {
		return nil, p.errf("Invalid value")
	}
	tok := p.s[start:p.i]
	if isDateToken(tok) {
		return tomlTime{raw: tok}, nil
	}
	if v, ok := parseNumber(tok); ok {
		return v, nil
	}
	// Restore a friendlier column for the error start.
	_ = startLine
	_ = startCol
	return nil, &tomlDecodeError{msg: fmt.Sprintf("Invalid value %s (at line %d, column %d)", pyReprString(tok), startLine, startCol)}
}

type tomlTime struct{ raw string }

func isDateToken(tok string) bool {
	if len(tok) < 8 {
		return false
	}
	// Local date, local time, or offset date-time. Any of these is not a string.
	if len(tok) >= 10 && tok[4] == '-' && tok[7] == '-' && isDigits(tok[:4]) && isDigits(tok[5:7]) && isDigits(tok[8:10]) {
		return true
	}
	if len(tok) >= 8 && tok[2] == ':' && tok[5] == ':' && isDigits(tok[:2]) && isDigits(tok[3:5]) && isDigits(tok[6:8]) {
		return true
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func parseNumber(tok string) (any, bool) {
	if tok == "inf" || tok == "+inf" || tok == "-inf" || tok == "nan" || tok == "+nan" || tok == "-nan" {
		f, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return nil, false
		}
		return f, true
	}
	sign := ""
	body := tok
	if strings.HasPrefix(body, "+") || strings.HasPrefix(body, "-") {
		sign = body[:1]
		body = body[1:]
	}
	base := 10
	switch {
	case strings.HasPrefix(body, "0x") || strings.HasPrefix(body, "0X"):
		base = 16
		body = body[2:]
	case strings.HasPrefix(body, "0o") || strings.HasPrefix(body, "0O"):
		base = 8
		body = body[2:]
	case strings.HasPrefix(body, "0b") || strings.HasPrefix(body, "0B"):
		base = 2
		body = body[2:]
	}
	if strings.Contains(body, ".") || strings.ContainsAny(body, "eE") {
		clean := sign + strings.ReplaceAll(body, "_", "")
		if strings.Contains(tok, "_") && !validUnderscores(tok) {
			return nil, false
		}
		f, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			return nil, false
		}
		return f, true
	}
	if !validUnderscores(sign+body) && strings.Contains(body, "_") {
		return nil, false
	}
	clean := strings.ReplaceAll(body, "_", "")
	if clean == "" {
		return nil, false
	}
	n, err := strconv.ParseInt(sign+clean, base, 64)
	if err != nil {
		return nil, false
	}
	return n, true
}

func validUnderscores(s string) bool {
	if strings.Contains(s, "__") || strings.HasSuffix(s, "_") || strings.HasPrefix(strings.TrimLeft(s, "+-"), "_") {
		return false
	}
	return true
}
