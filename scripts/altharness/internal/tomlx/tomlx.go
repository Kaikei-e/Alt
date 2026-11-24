// Package tomlx provides a pure-stdlib TOML 1.0 parser and struct/map
// unmarshaler for alt-harness so scripts/altharness has zero external Go
// module dependencies and builds cleanly in offline sandboxes.
package tomlx

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// DecodeError reports a TOML syntax or decoding error with line/column info.
type DecodeError struct {
	Msg string
}

func (e *DecodeError) Error() string { return e.Msg }

// Date represents an unquoted TOML date/time value (so it is distinct from string).
type Date struct {
	Raw string
}

// ParseBytes parses a UTF-8 TOML document into a map[string]any tree.
func ParseBytes(data []byte) (map[string]any, error) {
	if !utf8.Valid(data) {
		return nil, &DecodeError{Msg: "invalid UTF-8 in TOML document"}
	}
	return Parse(string(data))
}

// Parse parses a UTF-8 TOML string into a map[string]any tree.
func Parse(s string) (map[string]any, error) {
	if !utf8.ValidString(s) {
		return nil, &DecodeError{Msg: "invalid UTF-8 in TOML document"}
	}
	if strings.HasPrefix(s, "\uFEFF") {
		s = s[len("\uFEFF"):]
	}
	p := &parser{s: s, line: 1, col: 1}
	root := &table{v: map[string]any{}, explicit: true}
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
		p.skipSpaces()
		if err := p.endOfLine(); err != nil {
			return nil, err
		}
	}
	return unwrapTable(root), nil
}

// Unmarshal parses TOML data and populates dst (pointer to map or struct).
func Unmarshal(data []byte, dst any) error {
	if dst == nil {
		return &DecodeError{Msg: "tomlx: cannot unmarshal into nil"}
	}
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &DecodeError{Msg: "tomlx: destination must be a non-nil pointer"}
	}
	doc, err := ParseBytes(data)
	if err != nil {
		return err
	}
	return assignValue(rv.Elem(), doc)
}

type table struct {
	v        map[string]any
	inline   bool
	explicit bool
	dotted   bool
}

type arrayOfTables struct {
	items []*table
}

type parser struct {
	s    string
	i    int
	line int
	col  int
}

func unwrapTable(t *table) map[string]any {
	out := make(map[string]any, len(t.v))
	for k, v := range t.v {
		out[k] = unwrapVal(v)
	}
	return out
}

func unwrapVal(v any) any {
	switch t := v.(type) {
	case *table:
		return unwrapTable(t)
	case *arrayOfTables:
		out := make([]any, len(t.items))
		for i, el := range t.items {
			out[i] = unwrapTable(el)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, el := range t {
			out[i] = unwrapVal(el)
		}
		return out
	default:
		return v
	}
}

func (p *parser) eof() bool { return p.i >= len(p.s) }

func (p *parser) peekByte() byte {
	if p.eof() {
		return 0
	}
	return p.s[p.i]
}

func (p *parser) bump() rune {
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

func (p *parser) errf(format string, args ...any) error {
	return &DecodeError{
		Msg: fmt.Sprintf(format+" (at line %d, column %d)", append(args, p.line, p.col)...),
	}
}

func (p *parser) skipTrivia() {
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

func (p *parser) skipSpaces() {
	for !p.eof() {
		c := p.peekByte()
		if c != ' ' && c != '\t' {
			return
		}
		p.bump()
	}
}

func (p *parser) endOfLine() error {
	if p.eof() || p.peekByte() == '\n' || p.peekByte() == '\r' || p.peekByte() == '#' {
		return nil
	}
	return p.errf("unexpected trailing content %q", p.peekByte())
}

func (p *parser) parseHeader(root *table) (*table, error) {
	p.bump() // '['
	array := false
	if p.peekByte() == '[' {
		array = true
		p.bump()
	}
	p.skipSpaces()
	parts, err := p.parseKeyParts()
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, p.errf("empty table header")
	}
	p.skipSpaces()
	if array {
		if p.peekByte() != ']' || p.i+1 >= len(p.s) || p.s[p.i+1] != ']' {
			return nil, p.errf("expected ']]' at end of array table header")
		}
		p.bump()
		p.bump()
		p.skipSpaces()
		if err := p.endOfLine(); err != nil {
			return nil, err
		}
		return p.openArrayTable(root, parts)
	}
	if p.peekByte() != ']' {
		return nil, p.errf("expected ']' at end of table header")
	}
	p.bump()
	p.skipSpaces()
	if err := p.endOfLine(); err != nil {
		return nil, err
	}
	return p.openTable(root, parts)
}

func (p *parser) descendHeader(root *table, parts []string) (*table, error) {
	cur := root
	for _, part := range parts {
		existing, ok := cur.v[part]
		if !ok {
			nt := &table{v: map[string]any{}}
			cur.v[part] = nt
			cur = nt
			continue
		}
		switch et := existing.(type) {
		case *table:
			if et.inline {
				return nil, p.errf("cannot extend inline table %q", part)
			}
			cur = et
		case *arrayOfTables:
			if len(et.items) == 0 {
				return nil, p.errf("empty array of tables %q", part)
			}
			cur = et.items[len(et.items)-1]
		default:
			return nil, p.errf("cannot extend non-table key %q", part)
		}
	}
	return cur, nil
}

func (p *parser) openTable(root *table, parts []string) (*table, error) {
	cur, err := p.descendHeader(root, parts[:len(parts)-1])
	if err != nil {
		return nil, err
	}
	last := parts[len(parts)-1]
	existing, ok := cur.v[last]
	if !ok {
		nt := &table{v: map[string]any{}, explicit: true}
		cur.v[last] = nt
		return nt, nil
	}
	et, isT := existing.(*table)
	if !isT || et.inline || et.explicit || et.dotted {
		return nil, p.errf("cannot redefine table %q", strings.Join(parts, "."))
	}
	et.explicit = true
	return et, nil
}

func (p *parser) openArrayTable(root *table, parts []string) (*table, error) {
	cur, err := p.descendHeader(root, parts[:len(parts)-1])
	if err != nil {
		return nil, err
	}
	last := parts[len(parts)-1]
	existing, ok := cur.v[last]
	var aot *arrayOfTables
	if ok {
		var isAOT bool
		aot, isAOT = existing.(*arrayOfTables)
		if !isAOT {
			return nil, p.errf("cannot redefine key %q as array of tables", strings.Join(parts, "."))
		}
	} else {
		aot = &arrayOfTables{}
		cur.v[last] = aot
	}
	elem := &table{v: map[string]any{}, explicit: true}
	aot.items = append(aot.items, elem)
	return elem, nil
}

func (p *parser) parseKV(cur *table) error {
	parts, err := p.parseKeyParts()
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return p.errf("expected key")
	}
	p.skipSpaces()
	if p.peekByte() != '=' {
		return p.errf("expected '=' after key")
	}
	p.bump()
	p.skipSpaces()
	val, err := p.parseValue()
	if err != nil {
		return err
	}
	return p.put(cur, parts, val)
}

func (p *parser) put(cur *table, parts []string, val any) error {
	for _, part := range parts[:len(parts)-1] {
		existing, ok := cur.v[part]
		if !ok {
			nt := &table{v: map[string]any{}, dotted: true}
			cur.v[part] = nt
			cur = nt
			continue
		}
		et, isT := existing.(*table)
		if !isT || et.inline {
			return p.errf("cannot extend non-table key %q", part)
		}
		cur = et
	}
	last := parts[len(parts)-1]
	if _, exists := cur.v[last]; exists {
		return p.errf("cannot redefine key %q", strings.Join(parts, "."))
	}
	cur.v[last] = val
	return nil
}

func (p *parser) parseKeyParts() ([]string, error) {
	var parts []string
	for {
		p.skipSpaces()
		part, err := p.parseKeyOne()
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
		saved := p.snapshot()
		p.skipSpaces()
		if p.peekByte() == '.' {
			p.bump()
			continue
		}
		p.restore(saved)
		return parts, nil
	}
}

type snap struct{ i, line, col int }

func (p *parser) snapshot() snap { return snap{p.i, p.line, p.col} }
func (p *parser) restore(s snap) { p.i, p.line, p.col = s.i, s.line, s.col }

func (p *parser) parseKeyOne() (string, error) {
	if p.eof() {
		return "", p.errf("expected key")
	}
	c := p.peekByte()
	if c == '"' || c == '\'' {
		if p.i+2 < len(p.s) && p.s[p.i] == c && p.s[p.i+1] == c && p.s[p.i+2] == c {
			return "", p.errf("multiline strings are not valid keys")
		}
		v, err := p.parseString()
		if err != nil {
			return "", err
		}
		return v, nil
	}
	start := p.i
	for !p.eof() {
		c = p.peekByte()
		if isBareKeyByte(c) {
			p.bump()
			continue
		}
		break
	}
	if p.i == start {
		return "", p.errf("invalid key")
	}
	return p.s[start:p.i], nil
}

func isBareKeyByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
}

func (p *parser) parseValue() (any, error) {
	if p.eof() {
		return nil, p.errf("expected value")
	}
	c := p.peekByte()
	switch c {
	case '"', '\'':
		return p.parseString()
	case '[':
		return p.parseArray()
	case '{':
		return p.parseInlineTable()
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

func (p *parser) consumeWord(w string) bool {
	if strings.HasPrefix(p.s[p.i:], w) {
		rest := p.i + len(w)
		if rest < len(p.s) && isBareKeyByte(p.s[rest]) {
			return false
		}
		for range len(w) {
			p.bump()
		}
		return true
	}
	return false
}

func (p *parser) parseString() (string, error) {
	quote := p.peekByte()
	if p.i+2 < len(p.s) && p.s[p.i] == quote && p.s[p.i+1] == quote && p.s[p.i+2] == quote {
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
			return "", p.errf("unterminated string")
		}
		if c == quote {
			p.bump()
			return b.String(), nil
		}
		if quote == '"' && c == '\\' {
			esc, err := p.parseEscape(false)
			if err != nil {
				return "", err
			}
			b.WriteString(esc)
			continue
		}
		b.WriteRune(p.bump())
	}
	return "", p.errf("unterminated string")
}

func (p *parser) parseMultiString(quote byte) (string, error) {
	if !p.eof() && (p.peekByte() == '\n' || p.peekByte() == '\r') {
		p.bump()
	}
	var b strings.Builder
	for !p.eof() {
		if p.i+2 < len(p.s) && p.s[p.i] == quote && p.s[p.i+1] == quote && p.s[p.i+2] == quote {
			// Count consecutive quotes (up to 5: 1 or 2 quotes belong to content before closing 3).
			count := 0
			for p.i+count < len(p.s) && p.s[p.i+count] == quote && count < 6 {
				count++
			}
			if count >= 6 {
				return "", p.errf("too many consecutive quotes in multiline string")
			}
			extra := count - 3
			for range extra {
				b.WriteByte(quote)
				p.bump()
			}
			p.bump()
			p.bump()
			p.bump()
			return b.String(), nil
		}
		c := p.peekByte()
		if quote == '"' && c == '\\' {
			if p.isLineEndingBackslash() {
				p.bump() // backslash
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
				return "", err
			}
			b.WriteString(esc)
			continue
		}
		b.WriteRune(p.bump())
	}
	return "", p.errf("unterminated multiline string")
}

func (p *parser) isLineEndingBackslash() bool {
	j := p.i + 1
	for j < len(p.s) && (p.s[j] == ' ' || p.s[j] == '\t') {
		j++
	}
	return j < len(p.s) && (p.s[j] == '\n' || p.s[j] == '\r')
}

func (p *parser) parseEscape(multi bool) (string, error) {
	p.bump() // backslash
	if p.eof() {
		return "", p.errf("unterminated escape")
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
		return p.parseUnicodeEscape(4)
	case 'U':
		return p.parseUnicodeEscape(8)
	case '\n':
		if multi {
			return "", nil
		}
		return "", p.errf("invalid escape")
	default:
		return "", p.errf("invalid escape \\%c", c)
	}
}

func (p *parser) parseUnicodeEscape(n int) (string, error) {
	if p.i+n > len(p.s) {
		return "", p.errf("short unicode escape")
	}
	hexStr := p.s[p.i : p.i+n]
	for range n {
		p.bump()
	}
	v, err := strconv.ParseInt(hexStr, 16, 32)
	if err != nil || !utf8.ValidRune(rune(v)) {
		return "", p.errf("invalid unicode escape")
	}
	return string(rune(v)), nil
}

func (p *parser) parseArray() ([]any, error) {
	p.bump() // '['
	arr := []any{}
	for {
		p.skipTrivia()
		if p.eof() {
			return nil, p.errf("unterminated array")
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
		return nil, p.errf("expected ',' or ']' in array")
	}
}

func (p *parser) parseInlineTable() (*table, error) {
	p.bump() // '{'
	t := &table{v: map[string]any{}, inline: true, explicit: true}
	for {
		p.skipSpaces()
		if p.eof() {
			return nil, p.errf("unterminated inline table")
		}
		if p.peekByte() == '}' {
			p.bump()
			return t, nil
		}
		if err := p.parseKV(t); err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.peekByte() == ',' {
			p.bump()
			continue
		}
		if p.peekByte() == '}' {
			p.bump()
			return t, nil
		}
		return nil, p.errf("expected ',' or '}' in inline table")
	}
}

func (p *parser) parseAtom() (any, error) {
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
		return nil, p.errf("expected value")
	}
	tok := p.s[start:p.i]
	// Handle space-separated datetime like `2026-01-02 03:04:05Z`.
	if isDatePrefix(tok) && !p.eof() && p.peekByte() == ' ' {
		saved := p.snapshot()
		p.bump()
		timeStart := p.i
		for !p.eof() {
			c := p.peekByte()
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '#' || c == ',' || c == ']' || c == '}' {
				break
			}
			p.bump()
		}
		part2 := p.s[timeStart:p.i]
		if isTimeToken(part2) {
			tok = tok + " " + part2
		} else {
			p.restore(saved)
		}
	}
	if isDateToken(tok) {
		if tm, ok := parseTOMLTime(tok); ok {
			return tm, nil
		}
		return Date{Raw: tok}, nil
	}
	if v, ok := parseNumber(tok); ok {
		return v, nil
	}
	return nil, &DecodeError{
		Msg: fmt.Sprintf("invalid value %q (at line %d, column %d)", tok, startLine, startCol),
	}
}

func isDatePrefix(tok string) bool {
	return len(tok) == 10 && tok[4] == '-' && tok[7] == '-' && isDigits(tok[:4]) && isDigits(tok[5:7]) && isDigits(tok[8:10])
}

func isTimeToken(tok string) bool {
	return len(tok) >= 8 && tok[2] == ':' && tok[5] == ':' && isDigits(tok[:2]) && isDigits(tok[3:5]) && isDigits(tok[6:8])
}

func isDateToken(tok string) bool {
	if len(tok) >= 10 && tok[4] == '-' && tok[7] == '-' && isDigits(tok[:4]) && isDigits(tok[5:7]) && isDigits(tok[8:10]) {
		return true
	}
	return isTimeToken(tok)
}

func parseTOMLTime(tok string) (time.Time, bool) {
	norm := strings.Replace(tok, " ", "T", 1)
	norm = strings.Replace(norm, "t", "T", 1)
	norm = strings.Replace(norm, "z", "Z", 1)
	if tm, err := time.Parse(time.RFC3339Nano, norm); err == nil {
		return tm, true
	}
	return time.Time{}, false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func parseNumber(tok string) (any, bool) {
	switch tok {
	case "inf", "+inf":
		return math.Inf(1), true
	case "-inf":
		return math.Inf(-1), true
	case "nan", "+nan", "-nan":
		return math.NaN(), true
	}
	sign := ""
	body := tok
	if strings.HasPrefix(body, "+") || strings.HasPrefix(body, "-") {
		sign = body[:1]
		body = body[1:]
	}
	base := 10
	switch {
	case strings.HasPrefix(body, "0x"):
		if sign != "" {
			return nil, false
		}
		base = 16
		body = body[2:]
	case strings.HasPrefix(body, "0o"):
		if sign != "" {
			return nil, false
		}
		base = 8
		body = body[2:]
	case strings.HasPrefix(body, "0b"):
		if sign != "" {
			return nil, false
		}
		base = 2
		body = body[2:]
	}
	if base == 10 && (strings.Contains(body, ".") || strings.ContainsAny(body, "eE")) {
		if strings.Contains(tok, "_") && !validUnderscores(tok) {
			return nil, false
		}
		clean := sign + strings.ReplaceAll(body, "_", "")
		f, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			return nil, false
		}
		return f, true
	}
	if strings.Contains(body, "_") && !validUnderscores(body) {
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
	trimmed := strings.TrimLeft(s, "+-")
	if strings.Contains(s, "__") || strings.HasSuffix(s, "_") || strings.HasPrefix(trimmed, "_") {
		return false
	}
	return true
}

// Reflection-based assignment from parsed TOML tree into typed Go destinations.

func assignValue(dst reflect.Value, src any) error {
	if !dst.CanSet() {
		return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot set value of type %s", dst.Type())}
	}
	if src == nil {
		return nil
	}
	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return assignValue(dst.Elem(), src)
	}
	if dst.Kind() == reflect.Interface && dst.NumMethod() == 0 {
		dst.Set(reflect.ValueOf(src))
		return nil
	}
	if dst.Type() == reflect.TypeOf(time.Time{}) {
		if tm, ok := src.(time.Time); ok {
			dst.Set(reflect.ValueOf(tm))
			return nil
		}
		return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into time.Time", src)}
	}
	switch dst.Kind() {
	case reflect.Map:
		return assignMap(dst, src)
	case reflect.Struct:
		return assignStruct(dst, src)
	case reflect.Slice:
		return assignSlice(dst, src)
	case reflect.String:
		s, ok := src.(string)
		if !ok {
			return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into string", src)}
		}
		dst.SetString(s)
		return nil
	case reflect.Bool:
		b, ok := src.(bool)
		if !ok {
			return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into bool", src)}
		}
		dst.SetBool(b)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := toInt64(src)
		if !ok {
			return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into %s", src, dst.Type())}
		}
		if dst.OverflowInt(n) {
			return &DecodeError{Msg: fmt.Sprintf("tomlx: integer %d overflows %s", n, dst.Type())}
		}
		dst.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := toInt64(src)
		if !ok || n < 0 {
			return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into %s", src, dst.Type())}
		}
		u := uint64(n)
		if dst.OverflowUint(u) {
			return &DecodeError{Msg: fmt.Sprintf("tomlx: integer %d overflows %s", u, dst.Type())}
		}
		dst.SetUint(u)
		return nil
	case reflect.Float32, reflect.Float64:
		f, ok := toFloat64(src)
		if !ok {
			return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into %s", src, dst.Type())}
		}
		dst.SetFloat(f)
		return nil
	default:
		return &DecodeError{Msg: fmt.Sprintf("tomlx: unsupported destination type %s", dst.Type())}
	}
}

func assignMap(dst reflect.Value, src any) error {
	m, ok := src.(map[string]any)
	if !ok {
		return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into %s", src, dst.Type())}
	}
	if dst.Type().Key().Kind() != reflect.String {
		return &DecodeError{Msg: fmt.Sprintf("tomlx: map key type %s must be string", dst.Type().Key())}
	}
	if dst.IsNil() {
		dst.Set(reflect.MakeMapWithSize(dst.Type(), len(m)))
	}
	valType := dst.Type().Elem()
	for k, v := range m {
		elem := reflect.New(valType).Elem()
		if err := assignValue(elem, v); err != nil {
			return fmt.Errorf("key %q: %w", k, err)
		}
		kv := reflect.ValueOf(k).Convert(dst.Type().Key())
		dst.SetMapIndex(kv, elem)
	}
	return nil
}

func assignStruct(dst reflect.Value, src any) error {
	m, ok := src.(map[string]any)
	if !ok {
		return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into struct %s", src, dst.Type())}
	}
	exact, fold := structFields(dst.Type())
	for k, v := range m {
		idx, ok := exact[k]
		if !ok {
			idx, ok = fold[strings.ToLower(k)]
		}
		if !ok {
			continue
		}
		field := dst.FieldByIndex(idx)
		if err := assignValue(field, v); err != nil {
			return fmt.Errorf("field %q: %w", k, err)
		}
	}
	return nil
}

func structFields(t reflect.Type) (map[string][]int, map[string][]int) {
	exact := map[string][]int{}
	fold := map[string][]int{}
	var walk func(typ reflect.Type, prefix []int)
	walk = func(typ reflect.Type, prefix []int) {
		for i := range typ.NumField() {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			idx := append(append([]int{}, prefix...), i)
			tag := f.Tag.Get("toml")
			if tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if name == "" && f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type, idx)
				continue
			}
			if name == "" {
				name = f.Name
			}
			if _, ok := exact[name]; !ok {
				exact[name] = idx
			}
			low := strings.ToLower(name)
			if _, ok := fold[low]; !ok {
				fold[low] = idx
			}
		}
	}
	walk(t, nil)
	return exact, fold
}

func assignSlice(dst reflect.Value, src any) error {
	items, ok := src.([]any)
	if !ok {
		return &DecodeError{Msg: fmt.Sprintf("tomlx: cannot decode %T into %s", src, dst.Type())}
	}
	out := reflect.MakeSlice(dst.Type(), len(items), len(items))
	for i, item := range items {
		if err := assignValue(out.Index(i), item); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}
	dst.Set(out)
	return nil
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
