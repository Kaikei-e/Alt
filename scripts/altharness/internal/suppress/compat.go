package suppress

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

func utf8Valid(b []byte) bool { return utf8.Valid(b) }

func sortViolations(vs []violation) {
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].Path != vs[j].Path {
			return vs[i].Path < vs[j].Path
		}
		if vs[i].Line != vs[j].Line {
			return vs[i].Line < vs[j].Line
		}
		if vs[i].Rule != vs[j].Rule {
			return vs[i].Rule < vs[j].Rule
		}
		return vs[i].Message < vs[j].Message
	})
}

func splitAssign(a string) (name, val string, hasVal, ok bool) {
	if a == "-" || !strings.HasPrefix(a, "-") {
		return "", "", false, false
	}
	if strings.HasPrefix(a, "--") {
		body := a[2:]
		if i := strings.IndexByte(body, '='); i >= 0 {
			return "--" + body[:i], body[i+1:], true, body[:i] != ""
		}
		return a, "", false, len(body) > 0
	}
	if a == "-h" {
		return a, "", false, true
	}
	return a, "", false, false
}

func takeValue(argv []string, i int, val string, hasVal bool, name string) (string, int, error) {
	if hasVal {
		return val, i, nil
	}
	if i+1 >= len(argv) {
		return "", i, fmt.Errorf("argument %s: expected one argument", name)
	}
	return argv[i+1], i + 1, nil
}

func allowlistRel(path, repo string) string {
	if rel, ok := relTo(path, repo); ok && rel != "." {
		return rel
	}
	return toSlash(resolvePath(path))
}

func pyStr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		return t
	case bool:
		if t {
			return "True"
		}
		return "False"
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	default:
		return fmt.Sprint(t)
	}
}

func pyRepr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return pyReprString(t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%v", t)
	case []any:
		parts := make([]string, len(t))
		for i, el := range t {
			parts[i] = pyRepr(el)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		return pyList(t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = pyReprString(k) + ": " + pyRepr(t[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case orderedMap:
		parts := make([]string, len(t.pairs))
		for i, p := range t.pairs {
			parts[i] = pyReprString(p.k) + ": " + pyRepr(p.v)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return pyReprString(fmt.Sprint(t))
	}
}
