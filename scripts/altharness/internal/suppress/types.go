package suppress

import (
	"time"
	"unicode/utf8"
)

type violation struct {
	Path    string
	Line    int
	Rule    string
	Message string
}

func (v violation) loc() string {
	if v.Line > 0 {
		return v.Path + ":" + itoa(v.Line)
	}
	return v.Path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type banditAllow struct {
	Path        string
	Skips       map[string]struct{}
	ExcludeDirs map[string]struct{}
}

type golangciEntry struct {
	Config     string
	Path       *string
	PathExcept *string
	Text       *string
	Source     *string
	Linters    map[string]struct{}
}

type cargoKey struct {
	path    string
	section string
}

type allowlist struct {
	RuffGlobal      map[string]struct{}
	RuffTestPerFile map[string]struct{}
	Bandit          map[string]banditAllow
	Golangci        map[string][]golangciEntry
	Cargo           map[cargoKey]map[string]struct{}
}

var frozenBaselineMaxExpires = time.Date(2027, 5, 22, 0, 0, 0, 0, time.UTC)

func parseISODate(raw string) (time.Time, bool) {
	s := trimSpace(raw)
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil || t.Format("2006-01-02") != s {
		return time.Time{}, false
	}
	return t, true
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func utcToday() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func todayUTC(now func() time.Time) time.Time {
	if now == nil {
		return utcToday()
	}
	n := now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func subset(a, b map[string]struct{}) bool {
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func subsetSet(a, b map[string]struct{}) bool { return subset(a, b) }

func pyTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int:
		return t != 0
	case int64:
		return t != 0
	case uint64:
		return t != 0
	case float64:
		return t != 0
	case []any:
		return len(t) != 0
	case map[string]any:
		return len(t) != 0
	case orderedMap:
		return len(t.pairs) != 0
	default:
		return true
	}
}

func asMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case orderedMap:
		m := make(map[string]any, len(t.pairs))
		for _, p := range t.pairs {
			m[p.k] = p.v
		}
		return m, true
	default:
		return nil, false
	}
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func validUTF8(b []byte) bool { return utf8.Valid(b) }

func fileExists(path string) bool { return isRegularFile(path) }
