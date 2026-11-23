package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ymap is an insertion-ordered YAML mapping.
type ymap struct {
	k []string
	v []any
}

func (m *ymap) set(key string, val any) {
	for i, k := range m.k {
		if k == key {
			m.v[i] = val
			return
		}
	}
	m.k = append(m.k, key)
	m.v = append(m.v, val)
}

func (m ymap) get(key string) (any, bool) {
	for i, k := range m.k {
		if k == key {
			return m.v[i], true
		}
	}
	return nil, false
}

func (m *ymap) merge(o ymap) {
	for i, k := range o.k {
		m.set(k, o.v[i])
	}
}

type rawLine struct {
	no       int
	raw      string
	indent   int
	allSpace bool
	comment  bool
	tab      bool
}

type yparser struct {
	lines []rawLine
	i     int
}

func parseYAML(src string) (any, error) {
	src = strings.TrimPrefix(src, "\uFEFF")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	if strings.HasSuffix(src, "\n") {
		src = strings.TrimSuffix(src, "\n")
	}
	if src == "" {
		return nil, nil
	}
	parts := strings.Split(src, "\n")
	lines := make([]rawLine, 0, len(parts))
	for i, raw := range parts {
		lines = append(lines, classifyLine(i+1, raw))
	}
	p := &yparser{lines: lines}
	p.skip()
	if p.i >= len(p.lines) {
		return nil, nil
	}
	v, err := p.parseNode(0)
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i < len(p.lines) {
		ln := p.lines[p.i]
		if strings.TrimSpace(ln.raw) != "..." {
			return nil, fmt.Errorf("yaml:%d: trailing input %q", ln.no, strings.TrimSpace(ln.raw))
		}
	}
	return v, nil
}

func classifyLine(no int, raw string) rawLine {
	ln := rawLine{no: no, raw: raw}
	i := 0
	for i < len(raw) {
		if raw[i] == ' ' {
			i++
			continue
		}
		if raw[i] == '\t' {
			ln.tab = true
			i++
			continue
		}
		break
	}
	spaces := 0
	for spaces < len(raw) && raw[spaces] == ' ' {
		spaces++
	}
	ln.indent = spaces
	if strings.TrimSpace(raw) == "" {
		ln.allSpace = true
		return ln
	}
	trimmed := strings.TrimLeft(raw, " \t")
	if strings.HasPrefix(trimmed, "#") {
		ln.comment = true
	}
	return ln
}

func (p *yparser) skip() {
	for p.i < len(p.lines) {
		ln := p.lines[p.i]
		trim := strings.TrimSpace(ln.raw)
		if ln.allSpace || ln.comment || trim == "---" || trim == "..." {
			p.i++
			continue
		}
		return
	}
}

func (p *yparser) parseNode(min int) (any, error) {
	p.skip()
	if p.i >= len(p.lines) {
		return nil, nil
	}
	ln := p.lines[p.i]
	if ln.tab {
		return nil, fmt.Errorf("yaml:%d: tab indentation is not supported", ln.no)
	}
	if ln.indent < min {
		return nil, nil
	}
	content := lineContent(ln)
	if content == "-" || strings.HasPrefix(content, "- ") {
		return p.parseSeq(ln.indent)
	}
	return p.parseMap(ln.indent)
}

func lineContent(ln rawLine) string {
	if ln.indent > len(ln.raw) {
		return ""
	}
	return stripYAMLComment(ln.raw[ln.indent:])
}

func (p *yparser) parseMap(indent int) (ymap, error) {
	var m ymap
	for {
		p.skip()
		if p.i >= len(p.lines) {
			break
		}
		ln := p.lines[p.i]
		if ln.tab {
			return ymap{}, fmt.Errorf("yaml:%d: tab indentation is not supported", ln.no)
		}
		if ln.indent < indent {
			break
		}
		if ln.indent > indent {
			return ymap{}, fmt.Errorf("yaml:%d: unexpected indent %q", ln.no, strings.TrimSpace(ln.raw))
		}
		content := lineContent(ln)
		if content == "-" || strings.HasPrefix(content, "- ") {
			break
		}
		key, val, keyIdx, err := splitKey(content)
		if err != nil {
			return ymap{}, fmt.Errorf("yaml:%d: %w", ln.no, err)
		}
		p.i++
		parent := ln.indent + keyIdx
		parsed, err := p.parseValue(val, parent)
		if err != nil {
			return ymap{}, err
		}
		m.set(key, parsed)
	}
	return m, nil
}

func (p *yparser) parseSeq(indent int) ([]any, error) {
	var seq []any
	for {
		p.skip()
		if p.i >= len(p.lines) {
			break
		}
		ln := p.lines[p.i]
		if ln.tab {
			return nil, fmt.Errorf("yaml:%d: tab indentation is not supported", ln.no)
		}
		if ln.indent < indent {
			break
		}
		if ln.indent > indent {
			return nil, fmt.Errorf("yaml:%d: unexpected indent inside sequence %q", ln.no, strings.TrimSpace(ln.raw))
		}
		content := lineContent(ln)
		rest := ""
		switch {
		case content == "-":
			rest = ""
		case strings.HasPrefix(content, "- "):
			rest = strings.TrimSpace(content[2:])
		default:
			return seq, nil
		}
		p.i++
		if rest == "" {
			p.skip()
			if p.i < len(p.lines) && p.lines[p.i].indent > indent && !p.lines[p.i].comment && !p.lines[p.i].allSpace {
				nested, err := p.parseNode(p.lines[p.i].indent)
				if err != nil {
					return nil, err
				}
				seq = append(seq, nested)
			} else {
				seq = append(seq, nil)
			}
			continue
		}
		if _, _, _, ok := parseBlockHeader(rest); ok {
			parent := ln.indent
			val, err := p.readBlock(parent, rest)
			if err != nil {
				return nil, err
			}
			seq = append(seq, val)
			continue
		}
		if key, val, keyIdx, err := splitKey(rest); err == nil {
			// content is "- rest"; key sits after the dash prefix inside content.
			prefix := 0
			if i := strings.Index(content, key); i >= 0 {
				prefix = i
			} else {
				prefix = 2 + keyIdx
			}
			parent := ln.indent + prefix
			item := ymap{}
			parsed, err := p.parseValue(val, parent)
			if err != nil {
				return nil, err
			}
			item.set(key, parsed)
			p.skip()
			if p.i < len(p.lines) {
				n := p.lines[p.i]
				ncontent := ""
				if !n.allSpace && !n.comment && n.indent <= len(n.raw) {
					ncontent = lineContent(n)
				}
				if n.indent > indent && ncontent != "" && ncontent != "-" && !strings.HasPrefix(ncontent, "- ") {
					sib, err := p.parseMap(n.indent)
					if err != nil {
						return nil, err
					}
					item.merge(sib)
				}
			}
			seq = append(seq, item)
			continue
		}
		if strings.HasPrefix(rest, "[") || strings.HasPrefix(rest, "{") {
			flow, err := parseFlow(rest)
			if err != nil {
				return nil, fmt.Errorf("yaml:%d: %w", ln.no, err)
			}
			seq = append(seq, flow)
			continue
		}
		seq = append(seq, parseScalar(rest))
	}
	return seq, nil
}

func (p *yparser) parseValue(val string, parent int) (any, error) {
	val = strings.TrimSpace(val)
	if _, _, _, ok := parseBlockHeader(val); ok {
		return p.readBlock(parent, val)
	}
	if val == "" {
		p.skip()
		if p.i >= len(p.lines) {
			return nil, nil
		}
		ln := p.lines[p.i]
		if ln.allSpace || ln.comment || ln.indent <= parent {
			if ln.indent == parent {
				content := lineContent(ln)
				if content == "-" || strings.HasPrefix(content, "- ") {
					return p.parseSeq(ln.indent)
				}
			}
			return nil, nil
		}
		return p.parseNode(ln.indent)
	}
	if strings.HasPrefix(val, "[") || strings.HasPrefix(val, "{") {
		if flow, err := parseFlow(val); err == nil {
			return flow, nil
		}
	}
	return parseScalar(val), nil
}

func parseBlockHeader(s string) (literal bool, chomp int, ind int, ok bool) {
	if s == "" || (s[0] != '|' && s[0] != '>') {
		return false, 0, 0, false
	}
	literal = s[0] == '|'
	i := 1
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		if s[i] == '+' {
			chomp = 1
		} else {
			chomp = -1
		}
		i++
	}
	if i < len(s) && s[i] >= '1' && s[i] <= '9' {
		ind = int(s[i] - '0')
		i++
	}
	if i < len(s) && chomp == 0 && (s[i] == '+' || s[i] == '-') {
		if s[i] == '+' {
			chomp = 1
		} else {
			chomp = -1
		}
		i++
	}
	if i != len(s) {
		return false, 0, 0, false
	}
	return literal, chomp, ind, true
}

func (p *yparser) readBlock(parent int, header string) (string, error) {
	literal, chomp, explicit, ok := parseBlockHeader(header)
	if !ok {
		return "", fmt.Errorf("yaml: bad block header %q", header)
	}
	contentIndent := 0
	if explicit > 0 {
		contentIndent = parent + explicit
	}
	var content []string
	saw := false
	for p.i < len(p.lines) {
		ln := p.lines[p.i]
		if ln.allSpace {
			content = append(content, "")
			p.i++
			continue
		}
		if ln.indent <= parent {
			break
		}
		if contentIndent == 0 {
			contentIndent = ln.indent
		}
		if ln.indent < contentIndent {
			return "", fmt.Errorf("yaml:%d: block scalar indent is too small", ln.no)
		}
		if len(ln.raw) >= contentIndent {
			content = append(content, ln.raw[contentIndent:])
		} else {
			content = append(content, "")
		}
		saw = true
		p.i++
	}
	if !saw {
		content = nil
	}
	return finishBlock(content, literal, chomp), nil
}

func finishBlock(lines []string, literal bool, chomp int) string {
	var body string
	if literal {
		var b strings.Builder
		for _, line := range lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		body = b.String()
	} else {
		body = foldBody(lines)
		if len(lines) > 0 {
			body += "\n"
		}
	}
	switch chomp {
	case -1:
		return strings.TrimRight(body, "\n")
	case 1:
		return body
	default:
		trimmed := strings.TrimRight(body, "\n")
		if trimmed == "" {
			return ""
		}
		return trimmed + "\n"
	}
}

func foldBody(lines []string) string {
	var b strings.Builder
	started := false
	prevBlank := false
	for _, line := range lines {
		if line == "" {
			if started {
				prevBlank = true
			}
			continue
		}
		more := line[0] == ' ' || line[0] == '\t'
		if started {
			if more || prevBlank {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(line)
		started = true
		prevBlank = false
	}
	return b.String()
}

func splitKey(s string) (key, val string, keyIdx int, err error) {
	inSingle, inDouble := false, false
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '[', '{':
			depth++
		case ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && (i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\t') {
				rawKey := strings.TrimSpace(s[:i])
				if rawKey == "" {
					return "", "", 0, fmt.Errorf("missing key")
				}
				keyIdx = strings.Index(s, rawKey)
				if keyIdx < 0 {
					keyIdx = 0
				}
				uk, uerr := unquoteKey(rawKey)
				if uerr != nil {
					return "", "", 0, uerr
				}
				return uk, strings.TrimSpace(s[i+1:]), keyIdx, nil
			}
		}
	}
	return "", "", 0, fmt.Errorf("expected key")
}

func unquoteKey(s string) (string, error) {
	if len(s) >= 2 && s[0] == '\'' {
		v, ok := unquoteSingle(s)
		if !ok {
			return "", fmt.Errorf("bad quoted key %q", s)
		}
		return v, nil
	}
	if len(s) >= 2 && s[0] == '"' {
		v, err := strconv.Unquote(s)
		if err != nil {
			return "", fmt.Errorf("bad quoted key %q", s)
		}
		return v, nil
	}
	return s, nil
}

func stripYAMLComment(line string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		if c == '\'' {
			inSingle = true
			continue
		}
		if c == '"' {
			inDouble = true
			continue
		}
		if c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return strings.TrimRightFunc(line[:i], unicode.IsSpace)
		}
	}
	return strings.TrimRightFunc(line, unicode.IsSpace)
}

func parseScalar(s string) any {
	if v, ok := unquoteSingle(s); ok {
		return v
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if v, err := strconv.Unquote(s); err == nil {
			return v
		}
	}
	switch s {
	case "", "~", "null", "Null", "NULL":
		if s == "" {
			return nil
		}
		return nil
	case "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		return true
	case "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF":
		return false
	}
	if n, ok := yamlInt(s); ok {
		return n
	}
	if f, ok := yamlFloat(s); ok {
		return f
	}
	return s
}

func unquoteSingle(s string) (string, bool) {
	if len(s) < 2 || s[0] != '\'' {
		return "", false
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			if i == len(s)-1 {
				return b.String(), true
			}
			return "", false
		}
		b.WriteByte(s[i])
	}
	return "", false
}

func yamlInt(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		if len(s) == 1 {
			return 0, false
		}
		i = 1
	}
	if s[i] == '0' && i+1 != len(s) {
		return 0, false
	}
	for _, c := range s[i:] {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func yamlFloat(s string) (float64, bool) {
	if !strings.ContainsAny(s, ".eE") {
		return 0, false
	}
	if strings.Count(s, ".") > 1 {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	// Require the whole string to be a float, not a prefix.
	if strconv.FormatFloat(f, 'f', -1, 64) == "" {
		return 0, false
	}
	rest := s
	if strings.ContainsAny(rest, "eE") || strings.Contains(rest, ".") {
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return 0, false
		}
		// Reject values ParseFloat accepts only as a prefix. ParseFloat consumes all or errors.
		return f, true
	}
	return 0, false
}

func parseFlow(s string) (any, error) {
	s = strings.TrimSpace(s)
	if !flowWrapped(s) {
		return nil, fmt.Errorf("invalid flow %q", s)
	}
	if s[0] == '[' {
		items := splitFlowItems(s[1 : len(s)-1])
		out := make([]any, 0, len(items))
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			v, err := parseFlowItem(item)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	items := splitFlowItems(s[1 : len(s)-1])
	var m ymap
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		colon := findFlowColon(item)
		if colon < 0 {
			keyVal, err := parseFlowItem(item)
			if err != nil {
				return nil, err
			}
			m.set(flowKey(keyVal), nil)
			continue
		}
		keyPart := strings.TrimSpace(item[:colon])
		valPart := strings.TrimSpace(item[colon+1:])
		keyVal, err := parseFlowItem(keyPart)
		if err != nil {
			return nil, err
		}
		val, err := parseFlowItem(valPart)
		if err != nil {
			return nil, err
		}
		m.set(flowKey(keyVal), val)
	}
	return m, nil
}

func parseFlowItem(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if flowWrapped(s) {
		return parseFlow(s)
	}
	return parseScalar(s), nil
}

func flowKey(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return fmt.Sprint(t)
	}
}

func flowWrapped(s string) bool {
	if len(s) < 2 {
		return false
	}
	var close byte
	switch s[0] {
	case '[':
		close = ']'
	case '{':
		close = '}'
	default:
		return false
	}
	if s[len(s)-1] != close {
		return false
	}
	depth := 0
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return i == len(s)-1
			}
			if depth < 0 {
				return false
			}
		}
	}
	return false
}

func findFlowColon(s string) int {
	inSingle, inDouble := false, false
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '[', '{':
			depth++
		case ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && i+1 < len(s) && (s[i+1] == ' ' || s[i+1] == '\t') {
				return i
			}
		}
	}
	return -1
}

func splitFlowItems(inner string) []string {
	var items []string
	var buf []byte
	inSingle, inDouble := false, false
	depth := 0
	flush := func() {
		items = append(items, string(buf))
		buf = buf[:0]
	}
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if inSingle {
			buf = append(buf, c)
			if c == '\'' {
				if i+1 < len(inner) && inner[i+1] == '\'' {
					buf = append(buf, inner[i+1])
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			buf = append(buf, c)
			if c == '\\' && i+1 < len(inner) {
				buf = append(buf, inner[i+1])
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
			buf = append(buf, c)
		case '"':
			inDouble = true
			buf = append(buf, c)
		case '[', '{':
			depth++
			buf = append(buf, c)
		case ']', '}':
			if depth > 0 {
				depth--
			}
			buf = append(buf, c)
		case ',':
			if depth == 0 {
				flush()
				continue
			}
			buf = append(buf, c)
		default:
			buf = append(buf, c)
		}
	}
	flush()
	return items
}
