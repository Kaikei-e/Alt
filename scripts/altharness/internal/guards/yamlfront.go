package guards

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

// parsePacketFile reads a markdown packet (YAML frontmatter) or a JSON object.
func parsePacketFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	if strings.HasSuffix(strings.ToLower(path), ".json") || strings.HasPrefix(strings.TrimSpace(text), "{") {
		var data map[string]any
		if err := jsonUnmarshalObject([]byte(text), &data); err != nil {
			return nil, err
		}
		return data, nil
	}
	body, ok := frontmatter(text)
	if !ok {
		return nil, fmt.Errorf("missing YAML frontmatter")
	}
	return parseYAMLMap(body)
}

func jsonUnmarshalObject(raw []byte, dest *map[string]any) error {
	dec := jsonNewDecoder(raw)
	return dec(dest)
}

func frontmatter(text string) (string, bool) {
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n"), true
		}
	}
	return "", false
}

type yamlLine struct {
	indent int
	text   string
}

func parseYAMLMap(text string) (map[string]any, error) {
	lines, err := prepareYAML(text)
	if err != nil {
		return nil, err
	}
	m, _, err := parseYAMLMapping(lines, 0, 0)
	return m, err
}

func prepareYAML(text string) ([]yamlLine, error) {
	var out []yamlLine
	for _, raw := range strings.Split(text, "\n") {
		if i := indexUnquoted(raw, " #"); i >= 0 {
			raw = raw[:i]
		}
		trimmedRight := strings.TrimRightFunc(raw, unicode.IsSpace)
		if strings.TrimSpace(trimmedRight) == "" || strings.HasPrefix(strings.TrimSpace(trimmedRight), "#") {
			continue
		}
		indent := 0
		for indent < len(trimmedRight) && trimmedRight[indent] == ' ' {
			indent++
		}
		if strings.Contains(trimmedRight[:indent], "\t") {
			return nil, fmt.Errorf("tabs are not allowed for YAML indentation")
		}
		out = append(out, yamlLine{indent: indent, text: trimmedRight[indent:]})
	}
	return out, nil
}

func parseYAMLMapping(lines []yamlLine, start, indent int) (map[string]any, int, error) {
	out := map[string]any{}
	i := start
	for i < len(lines) {
		line := lines[i]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, i, fmt.Errorf("unexpected YAML indentation: %s", line.text)
		}
		if strings.HasPrefix(line.text, "- ") || line.text == "-" {
			return nil, i, fmt.Errorf("unexpected sequence in mapping: %s", line.text)
		}
		key, rest, err := splitYAMLKey(line.text)
		if err != nil {
			return nil, i, err
		}
		if rest != "" {
			val, err := parseYAMLScalar(rest)
			if err != nil {
				return nil, i, err
			}
			out[key] = val
			i++
			continue
		}
		if i+1 < len(lines) && lines[i+1].indent > indent {
			next := lines[i+1]
			if strings.HasPrefix(next.text, "- ") || next.text == "-" {
				seq, ni, err := parseYAMLSequence(lines, i+1, next.indent)
				if err != nil {
					return nil, i, err
				}
				out[key] = seq
				i = ni
				continue
			}
			child, ni, err := parseYAMLMapping(lines, i+1, next.indent)
			if err != nil {
				return nil, i, err
			}
			out[key] = child
			i = ni
			continue
		}
		out[key] = nil
		i++
	}
	return out, i, nil
}

func parseYAMLSequence(lines []yamlLine, start, indent int) ([]any, int, error) {
	var items []any
	i := start
	for i < len(lines) {
		line := lines[i]
		if line.indent < indent {
			break
		}
		if line.indent != indent || (!strings.HasPrefix(line.text, "- ") && line.text != "-") {
			break
		}
		rest := ""
		if line.text != "-" {
			rest = strings.TrimSpace(line.text[2:])
		}
		if rest == "" {
			if i+1 < len(lines) && lines[i+1].indent > indent {
				child, ni, err := parseYAMLMapping(lines, i+1, lines[i+1].indent)
				if err != nil {
					return nil, i, err
				}
				items = append(items, child)
				i = ni
				continue
			}
			items = append(items, nil)
			i++
			continue
		}
		if colon := indexUnquoted(rest, ":"); colon >= 0 && looksLikeKey(rest[:colon]) {
			item := map[string]any{}
			key := strings.TrimSpace(rest[:colon])
			valText := strings.TrimSpace(rest[colon+1:])
			ni := i + 1
			if valText != "" {
				val, err := parseYAMLScalar(valText)
				if err != nil {
					return nil, i, err
				}
				item[key] = val
			} else if ni < len(lines) && lines[ni].indent > indent {
				child, n2, err := parseYAMLMapping(lines, ni, lines[ni].indent)
				if err != nil {
					return nil, i, err
				}
				item[key] = child
				ni = n2
			} else {
				item[key] = nil
			}
			if ni < len(lines) && lines[ni].indent > indent && !strings.HasPrefix(lines[ni].text, "- ") {
				extra, n2, err := parseYAMLMapping(lines, ni, lines[ni].indent)
				if err != nil {
					return nil, i, err
				}
				for k, v := range extra {
					item[k] = v
				}
				ni = n2
			}
			items = append(items, item)
			i = ni
			continue
		}
		val, err := parseYAMLScalar(rest)
		if err != nil {
			return nil, i, err
		}
		items = append(items, val)
		i++
	}
	return items, i, nil
}

func splitYAMLKey(text string) (string, string, error) {
	idx := indexUnquoted(text, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("expected key: value, got %s", text)
	}
	key := strings.TrimSpace(text[:idx])
	rest := strings.TrimSpace(text[idx+1:])
	keyVal, err := parseYAMLScalar(key)
	if err != nil {
		return "", "", err
	}
	s, ok := keyVal.(string)
	if !ok {
		return "", "", fmt.Errorf("mapping key must be a string")
	}
	return s, rest, nil
}

func looksLikeKey(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	return !strings.HasPrefix(s, "\"") && !strings.HasPrefix(s, "'") && !strings.HasPrefix(s, "[") && !strings.HasPrefix(s, "{")
}

func parseYAMLScalar(text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" || text == "~" || text == "null" || text == "Null" || text == "NULL" {
		return nil, nil
	}
	switch text {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	}
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		var s string
		if err := jsonUnmarshalString(text, &s); err != nil {
			return nil, fmt.Errorf("invalid double-quoted YAML string %q: %w", text, err)
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
		parts := splitFlow(inner)
		out := make([]any, 0, len(parts))
		for _, part := range parts {
			val, err := parseYAMLScalar(part)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	}
	if isNumber(text) {
		if strings.Contains(text, ".") {
			var f float64
			fmt.Sscan(text, &f)
			return f, nil
		}
		var n int
		fmt.Sscan(text, &n)
		return n, nil
	}
	return text, nil
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' || s[0] == '+' {
		i++
	}
	dot := false
	digits := false
	for ; i < len(s); i++ {
		if s[i] == '.' && !dot {
			dot = true
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		digits = true
	}
	return digits
}

func splitFlow(inner string) []string {
	var parts []string
	var buf strings.Builder
	quote := byte(0)
	depth := 0
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if quote != 0 {
			buf.WriteByte(ch)
			if ch == quote && (i == 0 || inner[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			buf.WriteByte(ch)
			continue
		}
		if ch == '[' || ch == '{' {
			depth++
			buf.WriteByte(ch)
			continue
		}
		if ch == ']' || ch == '}' {
			if depth > 0 {
				depth--
			}
			buf.WriteByte(ch)
			continue
		}
		if ch == ',' && depth == 0 {
			parts = append(parts, strings.TrimSpace(buf.String()))
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	if tail := strings.TrimSpace(buf.String()); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func indexUnquoted(s, sep string) int {
	quote := byte(0)
	for i := 0; i+len(sep) <= len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == quote && (i == 0 || s[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
