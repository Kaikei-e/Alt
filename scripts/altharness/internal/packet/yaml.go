package packet

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	intPattern   = regexp.MustCompile(`^-?[0-9]+$`)
	floatPattern = regexp.MustCompile(`^-?(?:[0-9]+\.[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$|^-?[0-9]+[eE][+-]?[0-9]+$`)
)

type yLine struct {
	indent  int
	content string
}

func stripYAMLComment(raw string) string {
	var quote byte
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '#' && (i == 0 || prevRuneSpace(raw, i)) {
			return strings.TrimRightFunc(raw[:i], unicode.IsSpace)
		}
	}
	return strings.TrimRightFunc(raw, unicode.IsSpace)
}

func prevRuneSpace(s string, i int) bool {
	if i <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return unicode.IsSpace(r)
}

func findUnquotedColon(text string) int {
	var quote byte
	escaped := false
	depth := 0
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '[', '{':
			depth++
		case ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && (i+1 == len(text) || isASCIISpace(text[i+1])) {
				return i
			}
		}
	}
	return -1
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

func splitFlowItems(inner string) []string {
	var items []string
	var buf []byte
	var quote byte
	escaped := false
	depth := 0
	flush := func() {
		item := strings.TrimSpace(string(buf))
		if item != "" {
			items = append(items, item)
		}
		buf = buf[:0]
	}
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if escaped {
			buf = append(buf, ch)
			escaped = false
			continue
		}
		if quote == '"' && ch == '\\' {
			buf = append(buf, ch)
			escaped = true
			continue
		}
		if quote != 0 {
			buf = append(buf, ch)
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
			buf = append(buf, ch)
		case '[', '{':
			depth++
			buf = append(buf, ch)
		case ']', '}':
			if depth > 0 {
				depth--
			}
			buf = append(buf, ch)
		case ',':
			if depth == 0 {
				flush()
				continue
			}
			buf = append(buf, ch)
		default:
			buf = append(buf, ch)
		}
	}
	flush()
	return items
}

func parseYAMLFlowMapping(inner string) (map[string]any, error) {
	mapping := map[string]any{}
	if strings.TrimSpace(inner) == "" {
		return mapping, nil
	}
	for _, entry := range splitFlowItems(inner) {
		colon := findUnquotedColon(entry)
		if colon == -1 {
			return nil, pErr("invalid YAML flow mapping entry: %q", entry)
		}
		keyVal, err := parseYAMLScalar(strings.TrimSpace(entry[:colon]))
		if err != nil {
			return nil, err
		}
		val, err := parseYAMLScalar(strings.TrimSpace(entry[colon+1:]))
		if err != nil {
			return nil, err
		}
		mapping[pyStr(keyVal)] = val
	}
	return mapping, nil
}

func parseYAMLPrimitive(text string) any {
	switch strings.ToLower(text) {
	case "true":
		return true
	case "false":
		return false
	case "null", "~":
		return nil
	}
	if intPattern.MatchString(text) {
		n, err := parseInt64(text)
		if err == nil {
			return n
		}
	}
	if floatPattern.MatchString(text) {
		f, err := strconvParseFloat(text)
		if err == nil {
			return f
		}
	}
	return text
}

func parseYAMLScalar(raw string) (any, error) {
	text := strings.TrimSpace(stripYAMLComment(raw))
	if text == "" {
		return "", nil
	}
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		var s string
		if err := json.Unmarshal([]byte(text), &s); err != nil {
			return nil, pErr("invalid double-quoted YAML string %q: %s", text, err.Error())
		}
		return s, nil
	}
	if len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'' {
		return strings.ReplaceAll(text[1:len(text)-1], "''", "'"), nil
	}
	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		inner := strings.TrimSpace(text[1 : len(text)-1])
		if inner == "" {
			return []any{}, nil
		}
		parts := splitFlowItems(inner)
		out := make([]any, 0, len(parts))
		for _, item := range parts {
			v, err := parseYAMLScalar(item)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		return parseYAMLFlowMapping(text[1 : len(text)-1])
	}
	return parseYAMLPrimitive(text), nil
}

func prepareYAMLLines(raw []string) ([]yLine, error) {
	var prepared []yLine
	for _, line := range raw {
		stripped := stripYAMLComment(line)
		if strings.TrimSpace(stripped) == "" {
			continue
		}
		trimLeft := strings.TrimLeft(stripped, " ")
		indent := len(stripped) - len(trimLeft)
		if strings.Contains(stripped[:indent], "\t") {
			return nil, pErr("tabs are not allowed for YAML indentation: %q", line)
		}
		prepared = append(prepared, yLine{indent: indent, content: trimLeft})
	}
	return prepared, nil
}

func parseYAMLSequenceItem(lines []yLine, idx, seqIndent int, rest string) (any, int, error) {
	if rest == "" {
		if idx+1 < len(lines) && lines[idx+1].indent > seqIndent {
			return parseYAMLNode(lines, idx+1, lines[idx+1].indent)
		}
		return nil, idx + 1, nil
	}
	colon := findUnquotedColon(rest)
	if colon == -1 {
		v, err := parseYAMLScalar(rest)
		if err != nil {
			return nil, 0, err
		}
		return v, idx + 1, nil
	}
	keyVal, err := parseYAMLScalar(strings.TrimSpace(rest[:colon]))
	if err != nil {
		return nil, 0, err
	}
	key := pyStr(keyVal)
	valText := strings.TrimSpace(rest[colon+1:])
	item := map[string]any{}
	next := idx + 1
	if valText != "" {
		v, err := parseYAMLScalar(valText)
		if err != nil {
			return nil, 0, err
		}
		item[key] = v
	} else if next < len(lines) && lines[next].indent > seqIndent+2 {
		v, n, err := parseYAMLNode(lines, next, lines[next].indent)
		if err != nil {
			return nil, 0, err
		}
		item[key] = v
		next = n
	} else {
		item[key] = nil
	}
	if next < len(lines) && lines[next].indent > seqIndent && !strings.HasPrefix(lines[next].content, "-") {
		extra, n, err := parseYAMLMapping(lines, next, lines[next].indent)
		if err != nil {
			return nil, 0, err
		}
		for k, v := range extra {
			item[k] = v
		}
		next = n
	}
	return item, next, nil
}

func parseYAMLSequence(lines []yLine, start, seqIndent int) ([]any, int, error) {
	var items []any
	idx := start
	for idx < len(lines) {
		indent, content := lines[idx].indent, lines[idx].content
		if indent < seqIndent {
			break
		}
		if indent > seqIndent {
			return nil, 0, pErr("unexpected YAML indentation inside sequence: %q", content)
		}
		var rest string
		switch {
		case content == "-":
			rest = ""
		case strings.HasPrefix(content, "- "):
			rest = strings.TrimSpace(content[2:])
		default:
			return items, idx, nil
		}
		item, n, err := parseYAMLSequenceItem(lines, idx, seqIndent, rest)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
		idx = n
	}
	if items == nil {
		items = []any{}
	}
	return items, idx, nil
}

func parseYAMLMapping(lines []yLine, start, mapIndent int) (map[string]any, int, error) {
	mapping := map[string]any{}
	idx := start
	for idx < len(lines) {
		indent, content := lines[idx].indent, lines[idx].content
		if indent < mapIndent {
			break
		}
		if indent > mapIndent {
			return nil, 0, pErr("unexpected YAML indentation inside mapping: %q", content)
		}
		if content == "-" || strings.HasPrefix(content, "- ") {
			break
		}
		colon := findUnquotedColon(content)
		if colon == -1 {
			return nil, 0, pErr("expected 'key: value' in YAML frontmatter, got: %q", content)
		}
		keyVal, err := parseYAMLScalar(strings.TrimSpace(content[:colon]))
		if err != nil {
			return nil, 0, err
		}
		key := pyStr(keyVal)
		valText := strings.TrimSpace(content[colon+1:])
		idx++
		if valText != "" {
			v, err := parseYAMLScalar(valText)
			if err != nil {
				return nil, 0, err
			}
			mapping[key] = v
			continue
		}
		if idx < len(lines) && lines[idx].indent > mapIndent {
			v, n, err := parseYAMLNode(lines, idx, lines[idx].indent)
			if err != nil {
				return nil, 0, err
			}
			mapping[key] = v
			idx = n
			continue
		}
		if idx < len(lines) && lines[idx].indent == mapIndent && (lines[idx].content == "-" || strings.HasPrefix(lines[idx].content, "- ")) {
			v, n, err := parseYAMLSequence(lines, idx, mapIndent)
			if err != nil {
				return nil, 0, err
			}
			mapping[key] = v
			idx = n
			continue
		}
		mapping[key] = nil
	}
	return mapping, idx, nil
}

func parseYAMLNode(lines []yLine, start, expected int) (any, int, error) {
	content := lines[start].content
	if content == "-" || strings.HasPrefix(content, "- ") {
		return parseYAMLAnySeq(lines, start, expected)
	}
	m, n, err := parseYAMLMapping(lines, start, expected)
	return m, n, err
}

func parseYAMLAnySeq(lines []yLine, start, expected int) (any, int, error) {
	seq, n, err := parseYAMLSequence(lines, start, expected)
	return seq, n, err
}

func parseYAMLSubset(raw []string) (map[string]any, error) {
	prepared, err := prepareYAMLLines(raw)
	if err != nil {
		return nil, err
	}
	if len(prepared) == 0 {
		return map[string]any{}, nil
	}
	root, next, err := parseYAMLNode(prepared, 0, prepared[0].indent)
	if err != nil {
		return nil, err
	}
	if next != len(prepared) {
		return nil, pErr("unparsed trailing YAML line: %q", prepared[next].content)
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, pErr("YAML frontmatter root must be a mapping")
	}
	return m, nil
}
