// Package packet is the Go port of scripts/agent-harness/task_packet.py.
// It parses, validates, measures, and verifies Alt task packets and handoffs.
package packet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	exitOK      = 0
	exitInvalid = 1
	exitUsage   = 2

	schemaDraft202012        = "https://json-schema.org/draft/2020-12/schema"
	defaultPacketID          = "EXAMPLE-TASK-001"
	defaultTarget            = "scripts"
	defaultCommandBudgetS    = 180
	measureTimeoutSlackS     = 30
	timeoutExitCode          = 124
	sequenceMinCommits       = 2
	harnessStatePrefix       = ".harness/"
	subprocessTimeoutSeconds = 60
	maxBudgetSeconds         = 600
	generatedHeaderScanLines = 40
	gitExecMode              = "100755"
	gitSymlinkMode           = "120000"
	defaultTemplateVerifyCmd = "./scripts/lint/run.sh " + defaultTarget
	detachedSHALen           = 12
	rustAttrLookbackLines    = 12
	minQualifiedParts        = 2
)

var placeholderStrings = map[string]struct{}{
	"": {}, "tbd": {}, "n/a": {}, "na": {}, "-": {}, "--": {}, "—": {},
	"todo": {}, "none": {}, "null": {}, "なし": {}, "無し": {}, "未記入": {},
	"未定": {}, "xxx": {}, "...": {},
}

var structuredPayloadKeys = map[string]struct{}{
	"packet_version": {}, "packet_id": {}, "scope": {}, "execution_policy": {},
	"handoff_requirements": {}, "status": {}, "modified_files": {},
	"modified_functions_count": {}, "runtime_behavior_impact": {},
	"rationale": {}, "verification": {},
}

var excludedGeneratedPrefixes = []string{"stopped-using-k8s/", "scripts/harness/corpus/"}

var excludedTestScanDirs = map[string]struct{}{
	".git": {}, ".venv": {}, "__pycache__": {}, "build": {}, "dist": {},
	"node_modules": {}, "target": {}, "vendor": {}, "stopped-using-k8s": {},
}

// Issue is one validation finding. Code is the stable token emitted to users.
type Issue struct {
	Code    string
	Path    string
	Message string
}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

func pErr(format string, args ...any) error {
	return &parseError{msg: fmt.Sprintf(format, args...)}
}

func isParseErr(err error) bool {
	var pe *parseError
	return errors.As(err, &pe)
}

type harnessPaths struct {
	root        string
	schema      string
	packetTmpl  string
	handoffTmpl string
}

func locateHarness() (harnessPaths, error) {
	root, err := locateRepoRoot()
	if err != nil {
		return harnessPaths{}, err
	}
	base := filepath.Join(root, "scripts", "agent-harness")
	return harnessPaths{
		root:        root,
		schema:      filepath.Join(base, "task_packet.schema.json"),
		packetTmpl:  filepath.Join(base, "templates", "task-packet.md.tmpl"),
		handoffTmpl: filepath.Join(base, "templates", "task-handoff.md.tmpl"),
	}, nil
}

func locateRepoRoot() (string, error) {
	var starts []string
	if env := strings.TrimSpace(os.Getenv("ALT_HARNESS_REPO_ROOT")); env != "" {
		starts = append(starts, env)
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	seen := map[string]struct{}{}
	for _, start := range starts {
		dir := start
		for {
			if _, ok := seen[dir]; ok {
				break
			}
			seen[dir] = struct{}{}
			marker := filepath.Join(dir, "scripts", "agent-harness", "task_packet.schema.json")
			if st, err := os.Stat(marker); err == nil && !st.IsDir() {
				abs, err := filepath.Abs(dir)
				if err != nil {
					return "", fmt.Errorf("locate repo root: %w", err)
				}
				return abs, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("repo root with scripts/agent-harness/task_packet.schema.json not found")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

func commandSHA256(command string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(command)))
	return hex.EncodeToString(sum[:])
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asInt(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	default:
		return 0, false
	}
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	default:
		return 0, false
	}
}

func isBool(v any) bool {
	_, ok := v.(bool)
	return ok
}

func strictEqual(left, right any) bool {
	if isBool(left) != isBool(right) {
		return false
	}
	lf, lok := asFloat(left)
	rf, rok := asFloat(right)
	if lok && rok && !isBool(left) && !isBool(right) {
		return lf == rf
	}
	return fmt.Sprint(left) == fmt.Sprint(right) && sameKind(left, right)
}

func sameKind(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch a.(type) {
	case string:
		_, ok := b.(string)
		return ok
	case bool:
		_, ok := b.(bool)
		return ok
	case map[string]any:
		_, ok := b.(map[string]any)
		return ok
	case []any:
		_, ok := b.([]any)
		return ok
	default:
		_, aNum := asFloat(a)
		_, bNum := asFloat(b)
		return aNum && bNum
	}
}

func pyStr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func formatPacketID(v any) string {
	if v == nil {
		return "None"
	}
	return pyStr(v)
}

func parseJSONBytes(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return normalizeJSON(v), nil
}

func normalizeJSON(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil && !strings.ContainsAny(t.String(), ".eE") {
			return i
		}
		f, err := t.Float64()
		if err != nil {
			return t.String()
		}
		return f
	case map[string]any:
		for k, child := range t {
			t[k] = normalizeJSON(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = normalizeJSON(child)
		}
		return t
	default:
		return v
	}
}

func writeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}

func canonicalJSON(v any) string {
	var buf bytes.Buffer
	writeCanonical(&buf, v)
	return buf.String()
}

func writeCanonical(buf *bytes.Buffer, v any) {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case int:
		buf.WriteString(strconv.Itoa(t))
	case float64:
		buf.WriteString(strconv.FormatFloat(t, 'g', -1, 64))
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			buf.WriteString(strconv.Quote(t))
			return
		}
		buf.Write(b)
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteString(", ")
			}
			writeCanonical(buf, item)
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteString(", ")
			}
			b, _ := json.Marshal(k)
			buf.Write(b)
			buf.WriteString(": ")
			writeCanonical(buf, t[k])
		}
		buf.WriteByte('}')
	default:
		b, err := json.Marshal(t)
		if err != nil {
			buf.WriteString("null")
			return
		}
		buf.Write(b)
	}
}

func isPlaceholder(text string) bool {
	_, ok := placeholderStrings[strings.ToLower(strings.TrimSpace(text))]
	return ok
}

func normalizeRepoRel(filePath string) string {
	var parts []string
	for _, seg := range strings.Split(strings.TrimSpace(filePath), "/") {
		if seg == "" || seg == "." {
			continue
		}
		parts = append(parts, seg)
	}
	return strings.Join(parts, "/")
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func isSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("abs %s: %w", path, err)
	}
	var missing []string
	cur := abs
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve %s: %w", path, err)
		}
		base := filepath.Base(cur)
		missing = append(missing, base)
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(abs), nil
		}
		cur = parent
	}
}

func withinDir(path, dir string) bool {
	rp, err1 := resolvePath(path)
	rd, err2 := resolvePath(dir)
	if err1 != nil || err2 != nil {
		return false
	}
	if rp == rd {
		return true
	}
	rel, err := filepath.Rel(rd, rp)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func runeTail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

func validUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	return string(bytes.ToValidUTF8(b, []byte("\uFFFD")))
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
