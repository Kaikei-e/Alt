package corpus

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCatalogContracts(t *testing.T) {
	if len(UnimplementedCorpusChecks) != 0 {
		t.Fatalf("UNIMPLEMENTED_CORPUS_CHECKS = %v", UnimplementedCorpusChecks)
	}
	if !has(ProcessChecks, "VERIFY-01") || !has(ProcessChecks, "EXEC-01") {
		t.Fatal("PROCESS_CHECKS must include VERIFY-01 and EXEC-01")
	}
	for id := range UnimplementedCorpusChecks {
		if has(ProcessChecks, id) {
			t.Fatalf("partition overlap %s", id)
		}
	}
	if len(pyreflyFallbackServices) != 9 {
		t.Fatalf("fallback services = %d", len(pyreflyFallbackServices))
	}
}

func TestSchemaRejections(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()

	t.Run("non-contiguous", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "gap")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"before.py": "a = 1\n", "after.py": "a = 2\n"})
		writeCase(t, dir, "C-03", meta(metaOpt{id: "C-03"}), map[string]string{"before.py": "a = 1\n", "after.py": "a = 2\n"})
		code, _, errOut := runCLI(ctx, dir, repo, 2, false)
		if code != ExitFail || !strings.Contains(errOut, "contiguous") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("identical", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "same")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"before.py": "same = 1\n", "after.py": "same = 1\n"})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "byte-identical") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("traversal", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "escape.py"), []byte("escaped = 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "traversal")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01", before: "../escape.py"}), map[string]string{"after.py": "a = 2\n"})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "plain relative filename") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		escape := filepath.Join(root, "escape.py")
		if err := os.WriteFile(escape, []byte("escaped = 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "symlink")
		caseDir := writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"after.py": "a = 2\n"})
		if err := os.Symlink(escape, filepath.Join(caseDir, "before.py")); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "symlink") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("too many lines", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < MaxSnippetLines+1; i++ {
			fmtLine(&b, i)
		}
		dir := filepath.Join(t.TempDir(), "lines")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"before.py": "x = 1\n", "after.py": b.String()})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "exceeds 120 lines") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("too many bytes", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "bytes")
		big := append([]byte{0x7f, 'E', 'L', 'F'}, bytes.Repeat([]byte{'x'}, MaxSnippetBytes)...)
		writeCaseBytes(t, dir, "C-01", meta(metaOpt{id: "C-01", language: "binary", before: "before.bin", after: "after.bin"}), map[string][]byte{
			"before.bin": []byte("a"),
			"after.bin":  big,
		})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "exceeds 8192 bytes") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("missing runner fails closed", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "norunner")
		writeCase(t, dir, "C-01", meta(metaOpt{
			id: "C-01", planned: []string{"ARCH-01"}, active: []string{"ARCH-01"}, caught: true, signal: "missing-runner",
		}), map[string]string{"before.py": "x = 1\n", "after.py": "x = 2\n"})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(out+errOut, "no runner registered") {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
		}
	})

	t.Run("expected-miss capture rate", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "miss")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01", planned: []string{"VERIFY-01", "EXEC-01"}}), map[string]string{"before.py": "x = 1\n", "after.py": "x = 2\n"})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		want := "CORPUS CAPTURE RATE: 0/1 (0.0%) [caught=0: none | expected-miss=1 | failures=0]"
		if code != ExitOK || !strings.Contains(out, want) {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
		}
	})

	t.Run("empty corpus", func(t *testing.T) {
		dir := t.TempDir()
		code, _, _ := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail {
			t.Fatalf("default empty code=%d", code)
		}
		code, out, _ := runCLI(ctx, dir, repo, 1, true)
		if code != ExitOK || !strings.Contains(out, "CORPUS CAPTURE RATE: 0/0 (0.0%)") {
			t.Fatalf("allow empty code=%d out=%s", code, out)
		}
	})

	t.Run("negative min-cases", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run(ctx, []string{"--min-cases", "-1"}, &stdout, &stderr)
		if code != ExitUsage || !strings.Contains(stderr.String(), "--min-cases must be >= 0") {
			t.Fatalf("code=%d stderr=%s", code, stderr.String())
		}
	})
}

func TestRealCorpusExpectedMissStaysZero(t *testing.T) {
	repo := findRepo(t)
	corpusDir := filepath.Join(repo, "scripts", "harness", "corpus")
	cases, errs := DiscoverCases(corpusDir, DefaultMinCases, false)
	if len(errs) > 0 {
		t.Fatalf("real corpus validation errors:\n%s", strings.Join(errs, "\n"))
	}
	if len(cases) < DefaultMinCases {
		t.Fatalf("cases=%d", len(cases))
	}
	miss := 0
	for _, c := range cases {
		if len(c.Detection.ActiveChecks) == 0 {
			miss++
		}
	}
	if miss != 0 {
		t.Fatalf("expected-miss candidates=%d", miss)
	}
}

func TestTOMLMatchesPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 missing")
	}
	repo := findRepo(t)
	paths := []string{filepath.Join(repo, "scripts", "lint", "ratchet", "suppressions.toml")}
	matches, _ := filepath.Glob(filepath.Join(repo, "scripts", "harness", "corpus", "C-*", "meta.toml"))
	paths = append(paths, matches...)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseTOML(string(raw))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		want := pythonTOML(t, path)
		if !tomlEqual(got, want) {
			t.Fatalf("%s mismatch\ngot=%#v\nwant=%#v", path, got, want)
		}
	}
}

func pythonTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	cmd := exec.Command("python3", "-c", `
import json, tomllib, pathlib, sys
data = tomllib.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
def conv(o):
    if isinstance(o, dict):
        return {k: conv(v) for k, v in o.items()}
    if isinstance(o, list):
        return [conv(v) for v in o]
    if isinstance(o, bool):
        return o
    if isinstance(o, int):
        return o
    if isinstance(o, str):
        return o
    raise SystemExit("unsupported " + type(o).__name__)
json.dump(conv(data), sys.stdout)
`, path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("python toml %s: %v", path, err)
	}
	return jsonUnmarshal(t, out)
}

func jsonUnmarshal(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		t.Fatal(err)
	}
	norm := normalizeJSON(generic)
	m, ok := norm.(map[string]any)
	if !ok {
		t.Fatalf("root %T", norm)
	}
	return m
}

func normalizeJSON(v any) any {
	switch n := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(n))
		for k, child := range n {
			out[k] = normalizeJSON(child)
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i := range n {
			out[i] = normalizeJSON(n[i])
		}
		return out
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
		f, err := n.Float64()
		if err != nil {
			return n.String()
		}
		return f
	default:
		return v
	}
}

func tomlEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !valueEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func valueEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		return ok && tomlEqual(av, bv)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valueEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case string:
		bs, ok := b.(string)
		return ok && av == bs
	case bool:
		bb, ok := b.(bool)
		return ok && av == bb
	case int64:
		switch bv := b.(type) {
		case int64:
			return av == bv
		case int:
			return av == int64(bv)
		default:
			return false
		}
	default:
		return false
	}
}

type metaOpt struct {
	id       string
	language string
	before   string
	after    string
	planned  []string
	active   []string
	caught   bool
	signal   string
}

func meta(opt metaOpt) string {
	if opt.language == "" {
		opt.language = "python"
	}
	if opt.before == "" {
		opt.before = "before.py"
	}
	if opt.after == "" {
		opt.after = "after.py"
	}
	if opt.planned == nil {
		opt.planned = []string{"VERIFY-01"}
	}
	if opt.active == nil {
		opt.active = []string{}
	}
	caught := "false"
	if opt.caught {
		caught = "true"
	}
	return strings.Join([]string{
		"id = " + pyRepr(opt.id),
		"title = \"Sample failure case\"",
		"category = \"autofix-logic-corruption\"",
		"language = " + pyRepr(opt.language),
		"before_file = " + pyRepr(opt.before),
		"after_file = " + pyRepr(opt.after),
		"evaluation_mode = \"after\"",
		"",
		"[origin]",
		"branch = \"chore/lint-rollout\"",
		"base_commit = \"6cae3ef6f\"",
		"files = [\"scripts/sample.py\"]",
		"commits = [\"d8ce79dec\"]",
		"source_ref = \"docs/review/lint-rollout-harness-learnings-2026-11-21.md#2.1\"",
		"notes = \"Minimal deterministic failure reproduction snippet.\"",
		"",
		"[detection]",
		"planned_checks = " + tomlStringList(opt.planned),
		"active_checks = " + tomlStringList(opt.active),
		"currently_caught = " + caught,
		"current_method = \"diff-review-only\"",
		"expected_signal = " + pyRepr(opt.signal),
		"",
	}, "\n")
}

func tomlStringList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = strconvQuote(item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func strconvQuote(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

func writeCase(t *testing.T, corpusDir, id, metaText string, files map[string]string) string {
	t.Helper()
	raw := map[string][]byte{}
	for name, body := range files {
		raw[name] = []byte(body)
	}
	return writeCaseBytes(t, corpusDir, id, metaText, raw)
}

func writeCaseBytes(t *testing.T, corpusDir, id, metaText string, files map[string][]byte) string {
	t.Helper()
	dir := filepath.Join(corpusDir, id)
	if err := os.MkdirAll(dir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.toml"), []byte(metaText), fsperm.File); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, fsperm.File); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCLI(ctx context.Context, corpusDir, repo string, min int, allowEmpty bool) (int, string, string) {
	args := []string{"--corpus-dir", corpusDir, "--repo-root", repo, "--min-cases", itoa(min)}
	if allowEmpty {
		args = append(args, "--allow-empty-corpus")
	}
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func fmtLine(b *strings.Builder, i int) {
	b.WriteString("x_")
	b.WriteString(itoa(i))
	b.WriteString(" = ")
	b.WriteString(itoa(i))
	b.WriteByte('\n')
}

func findRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "harness", "corpus.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
