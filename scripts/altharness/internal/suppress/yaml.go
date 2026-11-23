package suppress

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type yline struct {
	indent int
	text   string
}

type yparser struct {
	lines []yline
	i     int
}

type orderedPair struct {
	k string
	v any
}

type orderedMap struct{ pairs []orderedPair }

type yamlDoc struct{ v any }

func (y yamlDoc) pyValue() any { return y.v }

func parseYAML(src string) (yamlDoc, error) {
	var lines []yline
	for _, raw := range strings.Split(src, "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		stripped, err := stripYAMLComment(raw)
		if err != nil {
			return yamlDoc{}, err
		}
		if strings.TrimSpace(stripped) == "" {
			continue
		}
		indent := 0
		for indent < len(stripped) && stripped[indent] == ' ' {
			indent++
		}
		if indent < len(stripped) && stripped[indent] == '\t' {
			return yamlDoc{}, fmt.Errorf("yaml: tab indentation is not supported")
		}
		lines = append(lines, yline{indent, strings.TrimRightFunc(stripped[indent:], unicode.IsSpace)})
	}
	if len(lines) == 0 {
		return yamlDoc{orderedMap{}}, nil
	}
	p := &yparser{lines: lines}
	v, err := p.parseNode(0)
	if err != nil {
		return yamlDoc{}, err
	}
	if p.i != len(lines) {
		return yamlDoc{}, fmt.Errorf("yaml: unconsumed input near %q", lines[p.i].text)
	}
	return yamlDoc{v}, nil
}

func stripYAMLComment(line string) (string, error) {
	inSingle, inDouble := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '\'' && !inDouble {
			if inSingle && i+1 < len(line) && line[i+1] == '\'' {
				i++
				continue
			}
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			if inDouble && i > 0 && line[i-1] == '\\' {
				continue
			}
			inDouble = !inDouble
			continue
		}
		if c == '#' && !inSingle && !inDouble {
			return line[:i], nil
		}
	}
	if inSingle || inDouble {
		return "", fmt.Errorf("yaml: unterminated quote")
	}
	return line, nil
}

func (p *yparser) parseNode(min int) (any, error) {
	if p.i >= len(p.lines) || p.lines[p.i].indent < min {
		return nil, nil
	}
	if isSeqItem(p.lines[p.i].text) {
		return p.parseSeq(p.lines[p.i].indent)
	}
	return p.parseMap(p.lines[p.i].indent)
}

func (p *yparser) parseMap(indent int) (any, error) {
	var pairs []orderedPair
	for p.i < len(p.lines) && p.lines[p.i].indent == indent && !isSeqItem(p.lines[p.i].text) {
		key, val, err := splitYAMLKey(p.lines[p.i].text)
		if err != nil {
			return nil, err
		}
		p.i++
		var stored any
		if strings.TrimSpace(val) == "" {
			if p.i < len(p.lines) && p.lines[p.i].indent > indent {
				nested, err := p.parseNode(p.lines[p.i].indent)
				if err != nil {
					return nil, err
				}
				stored = nested
			}
		} else {
			scalar, err := parseYAMLScalar(val)
			if err != nil {
				return nil, err
			}
			stored = scalar
		}
		pairs = append(pairs, orderedPair{k: key, v: stored})
	}
	return orderedMap{pairs}, nil
}

func (p *yparser) parseSeq(indent int) (any, error) {
	var seq []any
	for p.i < len(p.lines) && p.lines[p.i].indent == indent && isSeqItem(p.lines[p.i].text) {
		rest := seqItemRest(p.lines[p.i].text)
		p.i++
		if rest == "" {
			if p.i < len(p.lines) && p.lines[p.i].indent > indent {
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
		if key, val, err := splitYAMLKey(rest); err == nil && !scalarQuoted(rest) {
			var pairs []orderedPair
			if strings.TrimSpace(val) == "" {
				if p.i < len(p.lines) && p.lines[p.i].indent > indent {
					nested, err := p.parseNode(p.lines[p.i].indent)
					if err != nil {
						return nil, err
					}
					pairs = append(pairs, orderedPair{k: key, v: nested})
				} else {
					pairs = append(pairs, orderedPair{k: key, v: nil})
				}
			} else {
				scalar, err := parseYAMLScalar(val)
				if err != nil {
					return nil, err
				}
				pairs = append(pairs, orderedPair{k: key, v: scalar})
			}
			if p.i < len(p.lines) && p.lines[p.i].indent > indent && !isSeqItem(p.lines[p.i].text) {
				sib, err := p.parseMap(p.lines[p.i].indent)
				if err != nil {
					return nil, err
				}
				if sm, ok := sib.(orderedMap); ok {
					pairs = append(pairs, sm.pairs...)
				}
			}
			seq = append(seq, orderedMap{pairs: pairs})
			continue
		}
		scalar, err := parseYAMLScalar(rest)
		if err != nil {
			return nil, err
		}
		seq = append(seq, scalar)
	}
	return seq, nil
}

func isSeqItem(text string) bool {
	return text == "-" || strings.HasPrefix(text, "- ")
}

func seqItemRest(text string) string {
	if text == "-" {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(text, "-"))
}

func scalarQuoted(s string) bool {
	return len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"'))
}

func splitYAMLKey(s string) (string, string, error) {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inDouble {
			if inSingle && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if c == ':' && !inSingle && !inDouble {
			key := strings.TrimSpace(s[:i])
			if key == "" || strings.ContainsAny(key, " \t") && !scalarQuoted(key) {
				return "", "", fmt.Errorf("yaml: invalid key %q", key)
			}
			return key, strings.TrimSpace(s[i+1:]), nil
		}
	}
	return "", "", fmt.Errorf("yaml: expected key")
}

func parseYAMLScalar(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "~" || s == "null" || s == "Null" || s == "NULL" {
		return nil, nil
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return unescapeYAMLDouble(s[1 : len(s)-1]), nil
	}
	switch s {
	case "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		return true, nil
	case "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF":
		return false, nil
	}
	if n, ok := yamlPlainInt(s); ok {
		return n, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && strings.ContainsAny(s, ".eE") {
		return f, nil
	}
	return s, nil
}

func yamlPlainInt(s string) (int64, bool) {
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

func unescapeYAMLDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '\\' && i+size < len(s) {
			n, nsize := utf8.DecodeRuneInString(s[i+size:])
			switch n {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\', '"', '\'':
				b.WriteRune(n)
			default:
				b.WriteRune(n)
			}
			i += size + nsize
			continue
		}
		b.WriteRune(r)
		i += size
	}
	return b.String()
}
