package corpus

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseTOML parses the TOML subset used by corpus meta files and the
// suppression ratchet (comments, basic and literal strings, bools, ints,
// arrays, and dotted tables with bare or quoted keys).
func ParseTOML(text string) (map[string]any, error) {
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("invalid UTF-8")
	}
	p := &tomlParser{s: text, n: len(text)}
	root := map[string]any{}
	current := root
	if err := p.skipWS(); err != nil {
		return nil, err
	}
	for p.i < p.n {
		if p.peekByte() == '[' {
			parts, err := p.parseTableHeader()
			if err != nil {
				return nil, err
			}
			if err := p.finishLine(); err != nil {
				return nil, err
			}
			current, err = ensureTable(root, parts)
			if err != nil {
				return nil, err
			}
		} else {
			parts, err := p.parseKey()
			if err != nil {
				return nil, err
			}
			p.skipHSpace()
			if !p.consume('=') {
				return nil, p.errorf("expected '='")
			}
			p.skipHSpace()
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			if err := p.finishLine(); err != nil {
				return nil, err
			}
			if err := assignKey(current, parts, val); err != nil {
				return nil, err
			}
		}
		if err := p.skipWS(); err != nil {
			return nil, err
		}
	}
	return root, nil
}

type tomlParser struct {
	s string
	i int
	n int
}

func (p *tomlParser) errorf(format string, args ...any) error {
	line, col := 1, 1
	for i := 0; i < p.i && i < p.n; i++ {
		if p.s[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return fmt.Errorf(format+" (at line %d, column %d)", append(args, line, col)...)
}

func (p *tomlParser) peekByte() byte {
	if p.i >= p.n {
		return 0
	}
	return p.s[p.i]
}

func (p *tomlParser) consume(b byte) bool {
	if p.i < p.n && p.s[p.i] == b {
		p.i++
		return true
	}
	return false
}

func (p *tomlParser) skipHSpace() {
	for p.i < p.n {
		c := p.s[p.i]
		if c == ' ' || c == '\t' {
			p.i++
			continue
		}
		break
	}
}

func (p *tomlParser) skipWS() error {
	for p.i < p.n {
		c := p.s[p.i]
		switch c {
		case ' ', '\t', '\r', '\n':
			p.i++
		case '#':
			for p.i < p.n && p.s[p.i] != '\n' {
				p.i++
			}
		default:
			return nil
		}
	}
	return nil
}

func (p *tomlParser) finishLine() error {
	p.skipHSpace()
	if p.i >= p.n {
		return nil
	}
	switch p.s[p.i] {
	case '#', '\n', '\r':
		return nil
	default:
		return p.errorf("unexpected trailing content")
	}
}

func (p *tomlParser) parseTableHeader() ([]string, error) {
	if !p.consume('[') {
		return nil, p.errorf("expected '['")
	}
	if p.peekByte() == '[' {
		return nil, p.errorf("array tables are not supported")
	}
	parts, err := p.parseKey()
	if err != nil {
		return nil, err
	}
	p.skipHSpace()
	if !p.consume(']') {
		return nil, p.errorf("expected ']'")
	}
	return parts, nil
}

func (p *tomlParser) parseKey() ([]string, error) {
	var parts []string
	for {
		p.skipHSpace()
		part, err := p.parseKeyPart()
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
		p.skipHSpace()
		if p.peekByte() != '.' {
			break
		}
		p.i++
	}
	if len(parts) == 0 {
		return nil, p.errorf("expected key")
	}
	return parts, nil
}

func (p *tomlParser) parseKeyPart() (string, error) {
	if p.i >= p.n {
		return "", p.errorf("expected key")
	}
	switch p.s[p.i] {
	case '"':
		return p.parseBasicString()
	case '\'':
		return p.parseLiteralString()
	default:
		start := p.i
		for p.i < p.n {
			c := p.s[p.i]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
				p.i++
				continue
			}
			break
		}
		if start == p.i {
			return "", p.errorf("expected key")
		}
		return p.s[start:p.i], nil
	}
}

func (p *tomlParser) parseValue() (any, error) {
	if p.i >= p.n {
		return nil, p.errorf("expected value")
	}
	switch p.s[p.i] {
	case '"':
		return p.parseBasicString()
	case '\'':
		return p.parseLiteralString()
	case '[':
		return p.parseArray()
	case 't':
		if strings.HasPrefix(p.s[p.i:], "true") && p.endsWord(p.i+4) {
			p.i += 4
			return true, nil
		}
	case 'f':
		if strings.HasPrefix(p.s[p.i:], "false") && p.endsWord(p.i+5) {
			p.i += 5
			return false, nil
		}
	}
	return p.parseNumber()
}

func (p *tomlParser) endsWord(idx int) bool {
	if idx >= p.n {
		return true
	}
	c := p.s[idx]
	return !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-')
}

func (p *tomlParser) parseArray() ([]any, error) {
	if !p.consume('[') {
		return nil, p.errorf("expected '['")
	}
	var items []any
	for {
		if err := p.skipWS(); err != nil {
			return nil, err
		}
		if p.consume(']') {
			return items, nil
		}
		if p.i >= p.n {
			return nil, p.errorf("unterminated array")
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, val)
		if err := p.skipWS(); err != nil {
			return nil, err
		}
		if p.consume(',') {
			continue
		}
		if p.consume(']') {
			return items, nil
		}
		return nil, p.errorf("expected ',' or ']'")
	}
}

func (p *tomlParser) parseBasicString() (string, error) {
	if !p.consume('"') {
		return "", p.errorf("expected string")
	}
	var b strings.Builder
	for p.i < p.n {
		c := p.s[p.i]
		if c == '"' {
			p.i++
			return b.String(), nil
		}
		if c == '\n' || c == '\r' {
			return "", p.errorf("unterminated string")
		}
		if c != '\\' {
			b.WriteByte(c)
			p.i++
			continue
		}
		if p.i+1 >= p.n {
			return "", p.errorf("unterminated escape")
		}
		esc := p.s[p.i+1]
		p.i += 2
		switch esc {
		case 'b':
			b.WriteByte('\b')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'f':
			b.WriteByte('\f')
		case 'r':
			b.WriteByte('\r')
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case 'u':
			r, err := p.readHexRune(4)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
		case 'U':
			r, err := p.readHexRune(8)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
		default:
			return "", p.errorf("invalid escape")
		}
	}
	return "", p.errorf("unterminated string")
}

func (p *tomlParser) readHexRune(n int) (rune, error) {
	if p.i+n > p.n {
		return 0, p.errorf("short unicode escape")
	}
	v, err := strconv.ParseUint(p.s[p.i:p.i+n], 16, 32)
	if err != nil {
		return 0, p.errorf("invalid unicode escape")
	}
	p.i += n
	return rune(v), nil
}

func (p *tomlParser) parseLiteralString() (string, error) {
	if !p.consume('\'') {
		return "", p.errorf("expected string")
	}
	start := p.i
	for p.i < p.n {
		if p.s[p.i] == '\'' {
			s := p.s[start:p.i]
			p.i++
			return s, nil
		}
		if p.s[p.i] == '\n' || p.s[p.i] == '\r' {
			return "", p.errorf("unterminated string")
		}
		p.i++
	}
	return "", p.errorf("unterminated string")
}

func (p *tomlParser) parseNumber() (any, error) {
	start := p.i
	if p.peekByte() == '+' || p.peekByte() == '-' {
		p.i++
	}
	if p.i >= p.n || p.s[p.i] < '0' || p.s[p.i] > '9' {
		return nil, p.errorf("expected value")
	}
	for p.i < p.n {
		c := p.s[p.i]
		if c >= '0' && c <= '9' {
			p.i++
			continue
		}
		break
	}
	raw := p.s[start:p.i]
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, p.errorf("invalid integer")
	}
	return n, nil
}

func ensureTable(root map[string]any, parts []string) (map[string]any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty table name")
	}
	cur := root
	for i, part := range parts {
		existing, ok := cur[part]
		if i == len(parts)-1 {
			if ok {
				return nil, fmt.Errorf("duplicate table %q", strings.Join(parts, "."))
			}
			next := map[string]any{}
			cur[part] = next
			return next, nil
		}
		if !ok {
			next := map[string]any{}
			cur[part] = next
			cur = next
			continue
		}
		child, ok := existing.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("table path %q conflicts with an existing value", strings.Join(parts, "."))
		}
		cur = child
	}
	return nil, fmt.Errorf("empty table name")
}

func assignKey(table map[string]any, parts []string, val any) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty key")
	}
	cur := table
	for i, part := range parts {
		if i == len(parts)-1 {
			if _, ok := cur[part]; ok {
				return fmt.Errorf("duplicate key %q", strings.Join(parts, "."))
			}
			cur[part] = val
			return nil
		}
		existing, ok := cur[part]
		if !ok {
			next := map[string]any{}
			cur[part] = next
			cur = next
			continue
		}
		child, ok := existing.(map[string]any)
		if !ok {
			return fmt.Errorf("key path %q conflicts with an existing value", strings.Join(parts, "."))
		}
		cur = child
	}
	return fmt.Errorf("empty key")
}
