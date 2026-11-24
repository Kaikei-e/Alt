package screener

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCorpusSignals(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	corpus := filepath.Join(root, "scripts", "harness", "corpus")
	want := map[string]string{
		"C-01": "go.condition_operator_change",
		"C-02": "go.condition_operator_change",
		"C-03": "go.test_logger_discard_change",
		"C-06": "python.control_flow_return_change",
		"C-07": "python.finally_body_change",
		"C-10": "ts.playwright_wait_change",
		"C-11": "go.http_server_fields_change",
		"C-12": "go.file_mode_change",
		"C-13": "go.output_escaping_change",
		"C-14": "go.reverse_proxy_rewrite_change",
		"C-15": "go.body_limit_change",
		"C-17": "rust.fallible_error_swallow",
	}
	for id, kind := range want {
		id, kind := id, kind
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			before := corpusFile(t, corpus, id, "before")
			after := corpusFile(t, corpus, id, "after")
			code, cands, _, stderr := runCLI(t,
				"--before", before, "--after", before,
				"--logical-path", "corpus/"+id+"/"+filepath.Base(before),
				"--fail-on-candidates",
			)
			if code != exitOK || len(cands) != 0 {
				t.Fatalf("before vs before: code=%d cands=%v stderr=%s", code, kindsOf(cands), stderr)
			}
			code, cands, stdout, stderr := runCLI(t,
				"--before", before, "--after", after,
				"--logical-path", "corpus/"+id+"/"+filepath.Base(after),
			)
			if code != exitOK || !hasKind(cands, kind) {
				t.Fatalf("before vs after: code=%d kinds=%v stderr=%s stdout=%s", code, kindsOf(cands), stderr, stdout)
			}
			for _, c := range cands {
				if c.ID == "" || c.File == "" || c.Function == "" || c.Kind == "" || c.Before == "" || c.After == "" {
					t.Fatalf("incomplete candidate: %+v", c)
				}
			}
			code, _, stdout, stderr = runCLI(t,
				"--before", before, "--after", after,
				"--logical-path", "corpus/"+id+"/"+filepath.Base(after),
				"--fail-on-candidates",
			)
			if code != exitCandidates || !strings.Contains(stdout, kind) {
				t.Fatalf("fail-on-candidates: code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
		})
	}
	code, cands, _, stderr := runCLI(t,
		"--before", filepath.Join(corpus, "C-07", "before.py"),
		"--after", filepath.Join(corpus, "C-07", "after.py"),
	)
	got := map[string]bool{}
	for _, c := range cands {
		got[c.Kind] = true
	}
	if code != exitOK || !got["python.context_manager_change"] || !got["python.finally_body_change"] {
		t.Fatalf("C-07 kinds=%v stderr=%s", kindsOf(cands), stderr)
	}
}

func TestAllCorpusBeforeQuiet(t *testing.T) {
	t.Parallel()
	corpus := filepath.Join(repoRoot(t), "scripts", "harness", "corpus")
	entries, err := os.ReadDir(corpus)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "C-") {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	if len(dirs) < 22 {
		t.Fatalf("corpus cases=%d", len(dirs))
	}
	var bad []string
	for _, name := range dirs {
		matches, _ := filepath.Glob(filepath.Join(corpus, name, "before.*"))
		sort.Strings(matches)
		if len(matches) == 0 {
			continue
		}
		code, cands, _, _ := runCLI(t, "--before", matches[0], "--after", matches[0], "--fail-on-candidates")
		if code != exitOK || len(cands) != 0 {
			bad = append(bad, name)
		}
	}
	if len(bad) != 0 {
		t.Fatalf("noisy before-vs-before: %v", bad)
	}
}

func TestGoSafeAndStructural(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	beforeIs := "package p\nimport \"io\"\nfunc checkEOF(err error) bool {\n    if err == io.EOF {\n        return true\n    }\n    return false\n}\n"
	afterIs := "package p\nimport (\n    \"errors\"\n    \"io\"\n)\nfunc checkEOF(err error) bool {\n    if errors.Is(err, io.EOF) {\n        return true\n    }\n    return false\n}\n"
	code, cands, _, stderr := runPair(t, dir, "go_errors_is.go", beforeIs, afterIs, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("errors.Is safe: %+v %s", cands, stderr)
	}
	beforeRev := "package p\nimport \"io\"\nfunc checkEOF(err error) bool {\n    if io.EOF == err {\n        return true\n    }\n    return false\n}\n"
	code, cands, _, stderr = runPair(t, dir, "go_errors_is_rev.go", beforeRev, afterIs, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("reversed errors.Is: %+v %s", cands, stderr)
	}
	beforeNot := "package p\nimport \"io\"\nfunc notEOF(err error) bool {\n    if err != io.EOF {\n        return true\n    }\n    return false\n}\n"
	afterNot := "package p\nimport (\n    \"errors\"\n    \"io\"\n)\nfunc notEOF(err error) bool {\n    if !errors.Is(err, io.EOF) {\n        return true\n    }\n    return false\n}\n"
	code, cands, _, stderr = runPair(t, dir, "go_not_errors_is.go", beforeNot, afterNot, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("negated errors.Is: %+v %s", cands, stderr)
	}
	afterBug := "package p\nimport (\n    \"errors\"\n    \"io\"\n)\nfunc notEOF(err error) bool {\n    if errors.Is(err, io.EOF) {\n        return true\n    }\n    return false\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_bug_not_errors_is.go", beforeNot, afterBug, "")
	if code != exitOK || !hasKind(cands, "go.condition_operator_change") {
		t.Fatalf("dropped negation: %+v", cands)
	}
	beforeAs := "package p\ntype MyErr struct{ Code int }\nfunc (e *MyErr) Error() string { return \"x\" }\nfunc f(err error) int {\n    if v, ok := err.(*MyErr); ok {\n        return v.Code\n    }\n    return 0\n}\n"
	afterAs := "package p\nimport \"errors\"\ntype MyErr struct{ Code int }\nfunc (e *MyErr) Error() string { return \"x\" }\nfunc f(err error) int {\n    var v *MyErr\n    if errors.As(err, &v) {\n        return v.Code\n    }\n    return 0\n}\n"
	code, cands, _, stderr = runPair(t, dir, "go_errors_as.go", beforeAs, afterAs, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("errors.As: %+v %s", cands, stderr)
	}
	beforeAnd := "package p\ntype HTTPError struct{ Code int }\nfunc (e *HTTPError) Error() string { return \"x\" }\nfunc f(err error, rec func()) {\n    if he, ok := err.(*HTTPError); ok && he.Code >= 500 {\n        rec()\n    }\n}\n"
	afterAnd := "package p\nimport \"errors\"\ntype HTTPError struct{ Code int }\nfunc (e *HTTPError) Error() string { return \"x\" }\nfunc f(err error, rec func()) {\n    var he *HTTPError\n    if errors.As(err, &he) && he.Code >= 500 {\n        rec()\n    }\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_errors_as_and.go", beforeAnd, afterAnd, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("errors.As && : %+v", cands)
	}
	beforeSrv := "package main\nimport (\n    \"net/http\"\n    \"time\"\n)\nfunc newSrv(addr string, h http.Handler) *http.Server {\n    return &http.Server{\n        Addr: addr,\n        Handler: h,\n        ReadHeaderTimeout: 5 * time.Second,\n    }\n}\n"
	afterSrv := "package main\nimport (\n    \"net/http\"\n    \"time\"\n    \"alt/httpserver\"\n)\nfunc newSrv(addr string, h http.Handler) *http.Server {\n    return httpserver.New(httpserver.Config{\n        Addr: addr,\n        Handler: h,\n        ReadHeaderTimeout: 5 * time.Second,\n    })\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_http_srv_same.go", beforeSrv, afterSrv, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("http server same fields: %+v", cands)
	}
	beforeRet := "package p\nimport \"os\"\nfunc read(path string) error {\n    f, err := os.Open(path)\n    if err != nil {\n        return err\n    }\n    defer f.Close()\n    return nil\n}\n"
	afterRet := "package p\nimport \"os\"\nfunc read(path string) error {\n    f, err := os.Open(path)\n    if err != nil {\n        return err\n    }\n    _ = f\n    return nil\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_defer_drop.go", beforeRet, afterRet, "")
	if code != exitOK || !hasKind(cands, "go.return_or_defer_count_change") {
		t.Fatalf("defer drop: %+v", cands)
	}
	beforeTest := "package p\nimport \"testing\"\nfunc TestAdd(t *testing.T) {\n    if 1+1 != 2 {\n        t.Fatalf(\"expected 2\")\n    }\n}\n"
	afterTest := "package p\nimport \"testing\"\nfunc TestAdd(t *testing.T) {\n    if 1+1 != 2 {\n        t.Fatalf(\"expected something else\")\n    }\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_test_expect.go", beforeTest, afterTest, "pkg/math_test.go")
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("go expectation: %+v", cands)
	}
}

func TestPythonSafeAndStructural(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := "def compute(x, y):\n    try:\n        return x + y\n    except ValueError:\n        return 0\n    finally:\n        cleanup(x)\n"
	afterSafe := "\"\"\"Module docstring added.\"\"\"\n\n\ndef compute(x: int, y: int) -> int:\n    \"\"\"Function docstring added.\"\"\"\n    # formatting and annotations only\n    try:\n        return x + y\n    except ValueError:\n        return 0\n    finally:\n        cleanup(x)\n"
	code, cands, _, stderr := runPair(t, dir, "py_safe.py", before, afterSafe, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("python safe: %+v %s", cands, stderr)
	}
	afterExc := "def compute(x: int, y: int) -> int:\n    try:\n        return x + y\n    except Exception:\n        return 0\n    finally:\n        cleanup(x)\n"
	code, cands, _, stderr = runPair(t, dir, "py_exc.py", before, afterExc, "")
	if code != exitOK || !hasKind(cands, "python.except_types_change") {
		t.Fatalf("except widen: %+v %s", cands, stderr)
	}
	beforeT := "def test_value():\n    result = 42\n    assert result == 42\n"
	afterT := "def test_value():\n    result = 42\n    assert result >= 0\n"
	code, cands, _, _ = runPair(t, dir, "py_test_expect.py", beforeT, afterT, "tests/test_value.py")
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("py expectation: %+v", cands)
	}
}

func TestRustAndTS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := "pub fn parse(input: &str) -> Result<usize, std::num::ParseIntError> {\n    input.parse::<usize>()\n}\n"
	afterComment := "// Note: do not use Option<Regex> or .ok() or .unwrap_or_default() here.\npub fn parse(input: &str) -> Result<usize, std::num::ParseIntError> {\n    input.parse::<usize>()\n}\n"
	code, cands, _, _ := runPair(t, dir, "rs_comment.rs", before, afterComment, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("rust comment: %+v", cands)
	}
	afterSwallow := "pub fn parse(input: &str) -> usize {\n    input.parse::<usize>().unwrap_or_default()\n}\n"
	code, cands, _, _ = runPair(t, dir, "rs_unwrap_default.rs", before, afterSwallow, "")
	if code != exitOK || !hasKind(cands, "rust.fallible_error_swallow") {
		t.Fatalf("unwrap_or_default: %+v", cands)
	}
	beforeTS := "export function run(expect: any) {\n  expect(true).toBe(true);\n}\n"
	afterTS := "export function run(expect: any) {\n  expect(true).toBeTruthy();\n}\n"
	code, cands, _, _ = runPair(t, dir, "ts_test_expect.ts", beforeTS, afterTS, "alt-frontend-sv/tests/e2e/sample.spec.ts")
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("ts expectation: %+v", cands)
	}
	scriptBefore := "<script lang=\"ts\">\nexport async function check(page) { await page.waitForTimeout(500); }\n</script>\n<div>page.waitForTimeout(999)</div>\n"
	scriptAfter := "<script lang=\"ts\">\nexport async function check(page) { await page.waitForLoadState(\"load\"); }\n</script>\n<div>page.waitForTimeout(999)</div>\n"
	code, cands, _, _ = runPair(t, dir, "view.svelte", scriptBefore, scriptAfter, "")
	if code != exitOK || !hasKind(cands, "ts.playwright_wait_change") {
		t.Fatalf("svelte script: %+v", cands)
	}
	markupBefore := "<script>\nexport const n = 1;\n</script>\n<div>page.waitForTimeout(500)</div>\n"
	markupAfter := "<script>\nexport const n = 1;\n</script>\n<div>page.waitForLoadState(\"load\")</div>\n"
	code, cands, _, _ = runPair(t, dir, "markup.svelte", markupBefore, markupAfter, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("svelte markup should not screen: %+v", cands)
	}
}

func TestGitDiffAndOutput(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "mini_repo")
	if err := os.Mkdir(repo, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(repo, "app.py"), readCorpus(t, root, "C-06", "before.py"))
	writeFile(t, filepath.Join(repo, "handler.go"), readCorpus(t, root, "C-01", "before.go"))
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "baseline")
	code, cands, _, stderr := runCLI(t, "--repo-root", repo, "--base", "HEAD", "--fail-on-candidates")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("clean repo: code=%d cands=%v stderr=%s", code, cands, stderr)
	}
	writeFile(t, filepath.Join(repo, "app.py"), readCorpus(t, root, "C-06", "after.py"))
	writeFile(t, filepath.Join(repo, "handler.go"), readCorpus(t, root, "C-01", "after.go"))
	out := filepath.Join(dir, "report.json")
	code, _, _, stderr = runCLI(t, "--repo-root", repo, "--base", "HEAD", "--output", out, "--fail-on-candidates")
	saved := readCandidates(t, out)
	if code != exitCandidates || !hasPair(saved, "app.py", "python.control_flow_return_change") || !hasPair(saved, "handler.go", "go.condition_operator_change") {
		t.Fatalf("dirty repo: code=%d pairs=%v stderr=%s", code, pairsOf(saved), stderr)
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "regression")
	code, cands, _, stderr = runCLI(t, "--repo-root", repo, "--base", "HEAD~1", "--head", "HEAD")
	if code != exitOK || !hasPair(cands, "app.py", "python.control_flow_return_change") || !hasPair(cands, "handler.go", "go.condition_operator_change") {
		t.Fatalf("rev range: code=%d pairs=%v stderr=%s", code, pairsOf(cands), stderr)
	}
}

func TestFailClosedAndPrecision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	code, _, stdout, stderr := runPair(t, dir, "broken_syntax.go", "package p\nfunc F() int { return 1 }\n", "package p\nfunc F( { broken syntax\n", "")
	if code != exitUsage {
		t.Fatalf("go syntax: code=%d out=%s err=%s", code, stdout, stderr)
	}
	code, _, stdout, stderr = runPair(t, dir, "broken_syntax.py", "def f():\n    return 1\n", "def f(:\n    return\n", "")
	if code != exitUsage {
		t.Fatalf("py syntax: code=%d out=%s err=%s", code, stdout, stderr)
	}
	beforeML := "export async function run(page: any, expect: any) {\n  await expect(\n    page.locator(\"h1\")\n  ).toHaveText(\"Strict Title\");\n}\n"
	afterML := "export async function run(page: any, expect: any) {\n  await expect(\n    page.locator(\"h1\")\n  ).toBeDefined();\n}\n"
	code, cands, _, _ := runPair(t, dir, "ml_playwright.spec.ts", beforeML, afterML, "alt-frontend-sv/tests/e2e/chat.spec.ts")
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("multiline expect: %+v", cands)
	}
	beforeWrap := "export function run(expect: any, x: any) {\n  expect(foo(x, \"Keep  Spaces\")).toBe(false);\n}\n"
	afterWrap := "export function run(expect: any, x: any) {\n  expect(\n    foo(\n      x,\n      \"Keep  Spaces\",\n    ),\n  ).toBe(\n    false,\n  );\n}\n"
	code, cands, _, _ = runPair(t, dir, "wrap_ts.spec.ts", beforeWrap, afterWrap, "alt-frontend-sv/tests/e2e/wrap.spec.ts")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("ts wrap: %+v", cands)
	}
	beforeGo := "package p\nimport \"testing\"\nfunc TestWrap(t *testing.T) {\n    require.Equal(t, a, b)\n}\n"
	afterGo := "package p\nimport \"testing\"\nfunc TestWrap(t *testing.T) {\n    require.Equal(\n        t,\n        a,\n        b,\n    )\n}\n"
	code, cands, _, _ = runPair(t, dir, "wrap_go_test.go", beforeGo, afterGo, "pkg/wrap_test.go")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("go wrap: %+v", cands)
	}
	beforePy := "def test_wrap():\n    assert compute(a, b) == 42\n"
	afterPy := "def test_wrap():\n    assert (\n        compute(\n            a,\n            b,\n        )\n        == 42\n    )\n"
	code, cands, _, stderr = runPair(t, dir, "wrap_py_test.py", beforePy, afterPy, "tests/test_wrap.py")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("py wrap: %+v %s", cands, stderr)
	}
	beforeMT := "def multi(x, y):\n    try:\n        a = int(x)\n    except (ValueError, KeyError):\n        a = 0\n    try:\n        b = y[\"k\"]\n    except KeyError:\n        b = 0\n    return a + b\n"
	afterMT := "def multi(x, y):\n    try:\n        a = int(x)\n    except ValueError:\n        a = 0\n    try:\n        b = y[\"k\"]\n    except KeyError:\n        b = 0\n    return a + b\n"
	code, cands, _, stderr = runPair(t, dir, "py_multi_try.py", beforeMT, afterMT, "")
	if code != exitOK || !hasKind(cands, "python.except_types_change") {
		t.Fatalf("multi try: %+v %s", cands, stderr)
	}
	beforeCC := "package p\nfunc check(v bool) bool { return v }\nfunc run(a, b bool) int {\n    if check(a && b) {\n        return 1\n    }\n    return 0\n}\n"
	afterCC := "package p\nfunc check(v bool) bool { return v }\nfunc run(a, b bool) int {\n    if check(a) {\n        return 1\n    }\n    return 0\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_call_cond.go", beforeCC, afterCC, "")
	if code != exitOK || !hasKind(cands, "go.condition_operator_change") {
		t.Fatalf("call cond: %+v", cands)
	}
	beforeRecv := "package p\ntype Server struct{}\nfunc cleanup() {}\nfunc (s Server) Foo(a, b bool) int {\n    defer cleanup()\n    if a && b {\n        return 1\n    }\n    return 0\n}\n"
	afterRecv := "package p\ntype Server struct{}\nfunc cleanup() {}\nfunc (s *Server) Foo(a, b bool) int {\n    if a {\n        return 1\n    }\n    return 0\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_recv_change.go", beforeRecv, afterRecv, "")
	if code != exitOK || !hasKind(cands, "go.condition_operator_change") || !hasKind(cands, "go.return_or_defer_count_change") || !hasKind(cands, "go.receiver_pointer_change") {
		t.Fatalf("receiver: %v", kindsOf(cands))
	}
}

func TestGitSecurity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo := filepath.Join(dir, "sec_repo")
	if err := os.Mkdir(repo, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(repo, "app.py"), "def f():\n    return 1\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "base")
	clobber := filepath.Join(dir, "should_not_exist.txt")
	code, _, _, _ := runCLI(t, "--repo-root", repo, "--base=--output="+clobber)
	if code != exitUsage {
		t.Fatalf("dash base code=%d", code)
	}
	if _, err := os.Stat(clobber); err == nil {
		t.Fatal("clobber file was created")
	}
	code, _, _, _ = runCLI(t, "--repo-root", repo, "--base", "HEAD", "--head=--stat")
	if code != exitUsage {
		t.Fatalf("dash head code=%d", code)
	}
	outside := filepath.Join(dir, "outside_secret.py")
	writeFile(t, outside, "def f():\n    return 2\n")
	if err := os.Remove(filepath.Join(repo, "app.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "app.py")); err != nil {
		t.Fatal(err)
	}
	code, _, stdout, stderr := runCLI(t, "--repo-root", repo, "--base", "HEAD")
	if code != exitUsage {
		t.Fatalf("symlink code=%d out=%s err=%s", code, stdout, stderr)
	}
}

func TestWave2Go(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "del_test_repo")
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "tests"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(repo, "pkg", "calc_test.go"), "package pkg\nimport \"testing\"\nfunc TestCalc(t *testing.T) { t.Fatal(\"x\") }\n")
	writeFile(t, filepath.Join(repo, "tests", "test_calc.py"), "def test_calc():\n    assert 1 == 1\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "base")
	os.Remove(filepath.Join(repo, "pkg", "calc_test.go"))
	os.Remove(filepath.Join(repo, "tests", "test_calc.py"))
	code, cands, _, stderr := runCLI(t, "--repo-root", repo, "--base", "HEAD", "--fail-on-candidates")
	if code != exitCandidates || !hasPair(cands, "pkg/calc_test.go", "test.file_deleted") || !hasPair(cands, "tests/test_calc.py", "test.file_deleted") {
		t.Fatalf("deleted tests: code=%d pairs=%v stderr=%s", code, pairsOf(cands), stderr)
	}
	dir := t.TempDir()
	before := "package p\nfunc Keep() int { return 1 }\nfunc DropMe() int { return 2 }\n"
	after := "package p\nfunc Keep() int { return 1 }\nfunc Renamed() int { return 2 }\n"
	code, cands, _, _ = runPair(t, dir, "go_fn_rem.go", before, after, "")
	if code != exitOK || !funcRemoved(cands, "DropMe") {
		t.Fatalf("removed func: %+v", cands)
	}
	code, cands, _, _ = runPair(t, dir, "go_fn_add.go", before, before+"func BrandNew() int { return 3 }\n", "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("added func: %+v", cands)
	}
	beforeCl := "package p\nfunc Middleware(cleanup func()) func() error {\n    return func() error {\n        defer cleanup()\n        if true {\n            return nil\n        }\n        return nil\n    }\n}\n"
	afterCl := "package p\nfunc Middleware(cleanup func()) func() error {\n    return func() error {\n        _ = cleanup\n        return nil\n    }\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_closure_drop.go", beforeCl, afterCl, "")
	if code != exitOK || !hasKind(cands, "go.return_or_defer_count_change") {
		t.Fatalf("closure: %+v", cands)
	}
	beforeVar := "package p\nvar handler = func(a, b bool, cleanup func()) int {\n    defer cleanup()\n    if a && b {\n        return 1\n    }\n    return 0\n}\n"
	afterVar := "package p\nvar handler = func(a, b bool, cleanup func()) int {\n    _ = cleanup\n    if a {\n        return 1\n    }\n    return 0\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_pkg_var_fn.go", beforeVar, afterVar, "")
	if code != exitOK || !hasKind(cands, "go.condition_operator_change") || !hasKind(cands, "go.return_or_defer_count_change") {
		t.Fatalf("var fn: %v", kindsOf(cands))
	}
	beforeOK := "package p\ntype MyErr struct{}\nfunc (e *MyErr) Error() string { return \"x\" }\nfunc f(err error) bool {\n    if _, ok := err.(*MyErr); ok == false {\n        return false\n    }\n    return true\n}\n"
	afterOK := "package p\nimport \"errors\"\ntype MyErr struct{}\nfunc (e *MyErr) Error() string { return \"x\" }\nfunc f(err error) bool {\n    var v *MyErr\n    if !errors.As(err, &v) {\n        return false\n    }\n    return true\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_as_neg_safe.go", beforeOK, afterOK, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("ok == false: %+v", cands)
	}
	beforeDrop := "package p\ntype MyErr struct{}\nfunc (e *MyErr) Error() string { return \"x\" }\nfunc f(err error) bool {\n    if _, ok := err.(*MyErr); ok {\n        return true\n    }\n    return false\n}\n"
	afterDrop := "package p\ntype MyErr struct{}\nfunc (e *MyErr) Error() string { return \"x\" }\nfunc f(err error) bool {\n    if true {\n        return true\n    }\n    return false\n}\n"
	code, cands, _, _ = runPair(t, dir, "go_as_drop.go", beforeDrop, afterDrop, "")
	if code != exitOK || !hasKind(cands, "go.condition_operator_change") {
		t.Fatalf("drop assert: %+v", cands)
	}
}

func TestWave2RustPyTS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	beforeChar := "pub fn parse<'a>(s: &'a str) -> Result<usize, std::num::ParseIntError> {\n    let q = '\"';\n    let sq = '\\'';\n    let b = br#\"has \"quote\" inside\"#;\n    let c = cr#\"another \"quote\"\"#;\n    let _ = (q, sq, b, c);\n    s.parse::<usize>()\n}\n"
	afterChar := "pub fn parse<'a>(s: &'a str) -> Option<usize> {\n    let q = '\"';\n    let sq = '\\'';\n    let b = br#\"has \"quote\" inside\"#;\n    let c = cr#\"another \"quote\"\"#;\n    let _ = (q, sq, b, c);\n    s.parse::<usize>().ok()\n}\n"
	code, cands, _, _ := runPair(t, dir, "rs_char_lit.rs", beforeChar, afterChar, "")
	if code != exitOK || !hasKind(cands, "rust.fallible_error_swallow") {
		t.Fatalf("char lit: %+v", cands)
	}
	beforeCancel := "pub fn prod(s: &str) -> Result<usize, std::num::ParseIntError> {\n    s.parse::<usize>()\n}\n\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn test_old() {\n        let _ = \"1\".parse::<usize>().ok();\n    }\n}\n"
	afterCancel := "pub fn prod(s: &str) -> Option<usize> {\n    s.parse::<usize>().ok()\n}\n\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn test_old() {\n        let _ = \"1\".parse::<usize>();\n    }\n}\n"
	code, cands, _, _ = runPair(t, dir, "rs_cancel.rs", beforeCancel, afterCancel, "")
	if code != exitOK || !hasKind(cands, "rust.fallible_error_swallow") {
		t.Fatalf("cancel: %+v", cands)
	}
	beforeRS := "#[tokio::test]\nasync fn checks_prop() {\n    prop_assert_eq!(1 + 1, 2);\n    assert_matches!(Ok::<i32, ()>(1), Ok(1));\n}\n"
	afterRS := "#[tokio::test]\nasync fn checks_prop() {\n    prop_assert_eq!(1 + 1, 3);\n    assert_matches!(Ok::<i32, ()>(1), Ok(1));\n}\n"
	for _, path := range []string{
		"recap-worker/recap-worker/src/store/dao/tests.rs",
		"recap-worker/recap-worker/src/pki/issuer_http_tests.rs",
		"recap-worker/recap-worker/src/inline_tokio.rs",
	} {
		code, cands, _, _ = runPair(t, dir, "rt_"+filepath.Base(path), beforeRS, afterRS, path)
		if code != exitOK || !hasKind(cands, "test.expectation_change") {
			t.Fatalf("%s: %+v", path, cands)
		}
	}
	beforeSV := "export async function check(page) { await page.waitForTimeout(500); }\n"
	afterSV := "export async function check(page) { await page.waitForLoadState(\"networkidle\"); }\n"
	for _, ext := range []string{".svelte", ".mjs", ".cjs"} {
		code, cands, _, _ = runPair(t, dir, "sample"+ext, beforeSV, afterSV, "")
		if code != exitOK || !hasKind(cands, "ts.playwright_wait_change") {
			t.Fatalf("%s: %+v", ext, cands)
		}
	}
	beforePy := "def check_status(ok: bool, code: int) -> bool:\n    if not ok and code >= 500:\n        return False\n    return True\n\ndef load_val(x: str) -> int:\n    try:\n        return int(x)\n    except ValueError:\n        raise\n\ndef helper_removed() -> int:\n    return 1\n"
	afterPy := "def check_status(ok: bool, code: int) -> bool:\n    if ok:\n        return False\n    return True\n\ndef load_val(x: str) -> int:\n    try:\n        return int(x)\n    except ValueError:\n        return 0\n"
	code, cands, _, stderr := runPair(t, dir, "py_wave2_ops.py", beforePy, afterPy, "")
	if code != exitOK || !hasKind(cands, "python.condition_operator_change") || !hasKind(cands, "python.except_raise_change") || !hasKind(cands, "python.function_removed") {
		t.Fatalf("py wave2: %v %s", kindsOf(cands), stderr)
	}
}

func TestParseAllTrackedPythonFiles(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	cmd := exec.Command("git", "-C", root, "ls-files", "--", "*.py")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	if len(files) < 1000 {
		t.Fatalf("expected >= 1000 tracked .py files, got %d", len(files))
	}
	var failures []string
	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		src, err := readText(full)
		if err != nil {
			failures = append(failures, rel+": read error: "+err.Error())
			continue
		}
		if _, err := parsePython(src); err != nil {
			failures = append(failures, rel+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		limit := len(failures)
		if limit > 30 {
			limit = 30
		}
		t.Fatalf("failed to parse %d / %d tracked .py files (first %d):\n%s",
			len(failures), len(files), limit, strings.Join(failures[:limit], "\n"))
	}
}

func TestPythonLexerAndParserEdgeCases(t *testing.T) {
	t.Parallel()
	validSnippets := []struct {
		name string
		src  string
	}{
		{"implicit concat str + fstr", "def f(version):\n    return (\"FIX: \" f\"{version}\" \"!\")\n"},
		{"implicit concat fstr + fstr", "def f(a, b):\n    return (f\"{a}\" f\"{b}\")\n"},
		{"implicit concat tstr", "def f(x):\n    return (t\"a={x}\" \" b=\" t\"{x}\")\n"},
		{"rf-string backslash before brace and unicode escape", "def f(x):\n    return (rf\"\\{x}\", f\"\\N{GREEK SMALL LETTER ALPHA}{x}\", f\"{x if'a' else 'b'}\")\n"},
		{"parenthesized with as", "def f(a, b):\n    with (\n        aclosing(a) as s,\n        open(b) as t,\n    ):\n        return s, t\n"},
		{"parenthesized with no as", "def f(a, b):\n    with (\n        ctx1(a),\n        ctx2(b),\n    ):\n        pass\n"},
		{"pep758 except no as", "def f(x):\n    try:\n        return int(x)\n    except TypeError, ValueError:\n        return 0\n"},
		{"pep758 except star no as", "def f(x):\n    try:\n        return int(x)\n    except* TypeError, ValueError:\n        return 0\n"},
		{"dict unpacking after kv", "def f(prompt, kwargs):\n    return {\"prompt\": prompt, **kwargs, \"x\": 1}\n"},
		{"fstr leading whitespace dict", "def f(n, u):\n    return f\"{ {n: u} }\"\n"},
		{"fstr neq operator", "def f(a, b):\n    return f\"{a != b} {a != b!r} {a != b:10} {a=}\"\n"},
		{"raw string backslash parity", "def f():\n    a = r\"\\\\\"\n    b = r'\\\\'\n    c = r\"\\\\\\\"\"\n    d = r'don\\'t'\n    e = r\"\"\"\\\\\"\"\"\n    return (a, b, c, d, e)\n"},
		{"for tuple iterable", "def f(a, b):\n    for x in a, b:\n        print(x)\n"},
		{"pep695 type alias", "type Alias = list[int]\ntype Box[T] = dict[str, T]\ndef f(x: Alias) -> Alias:\n    return x\n"},
		{"pep750 tstring", "def f(name):\n    return t\"hello {name!r}\" + rt\"\\\\{name}\"\n"},
		{"pep701 multiline fstr comment", "def f(x):\n    return f\"{\n        x + 1  # comment with } and '\n    }\"\n"},
	}
	for _, tc := range validSnippets {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parsePython(tc.src); err != nil {
				t.Fatalf("parsePython failed: %v\nsrc:\n%s", err, tc.src)
			}
		})
	}

	// PEP 758: unparenthesized multiple exceptions WITH `as` target remains invalid syntax.
	for _, bad := range []string{
		"try:\n    x()\nexcept TypeError, ValueError as e:\n    pass\n",
		"try:\n    x()\nexcept* TypeError, ValueError as e:\n    pass\n",
	} {
		if _, err := parsePython(bad); err == nil {
			t.Fatalf("expected parse error for PEP 758 invalid `as` form:\n%s", bad)
		}
	}

	// Parenthesized with formatting equivalence: ruff format wrapping parens must not cause false positive.
	dir := t.TempDir()
	beforeWith := "def f():\n    with ctx1(), ctx2():\n        work()\n"
	afterWithParen := "def f():\n    with (\n        ctx1(),\n        ctx2(),\n    ):\n        work()\n"
	code, cands, _, stderr := runPair(t, dir, "py_with_paren_eq.py", beforeWith, afterWithParen, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("ruff-formatted parenthesized with should emit 0 candidates: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}
	afterWithDropped := "def f():\n    with (\n        ctx1(),\n    ):\n        work()\n"
	code, cands, _, stderr = runPair(t, dir, "py_with_paren_drop.py", afterWithParen, afterWithDropped, "")
	if code != exitOK || !hasKind(cands, "python.context_manager_change") {
		t.Fatalf("dropping ctx2 from parenthesized with must emit context_manager_change: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}
	// PEP 758 except equivalence: (A, B) vs A, B has identical exception set.
	beforeExc := "def f(x):\n    try:\n        return int(x)\n    except (TypeError, ValueError):\n        return 0\n"
	afterExc := "def f(x):\n    try:\n        return int(x)\n    except TypeError, ValueError:\n        return 0\n"
	code, cands, _, stderr = runPair(t, dir, "py_pep758_eq.py", beforeExc, afterExc, "")
	if code != exitOK || len(cands) != 0 {
		t.Fatalf("PEP 758 same exception types should emit 0 candidates: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}
}

func TestPythonFinallyBodyPrecision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cases := []struct {
		name   string
		before string
		after  string
	}{
		{
			name:   "del replaced with pass",
			before: "def f(self, sid):\n    try:\n        work()\n    finally:\n        del self._active[sid]\n",
			after:  "def f(self, sid):\n    try:\n        work()\n    finally:\n        pass\n",
		},
		{
			name:   "break inserted in finally",
			before: "def f(xs):\n    for x in xs:\n        try:\n            work(x)\n        finally:\n            pass\n",
			after:  "def f(xs):\n    for x in xs:\n        try:\n            work(x)\n        finally:\n            break\n",
		},
		{
			name:   "continue vs break in finally",
			before: "def f(xs):\n    for x in xs:\n        try:\n            work(x)\n        finally:\n            continue\n",
			after:  "def f(xs):\n    for x in xs:\n        try:\n            work(x)\n        finally:\n            break\n",
		},
		{
			name:   "aug assign operator flip",
			before: "def f(self):\n    try:\n        work()\n    finally:\n        self._inflight -= 1\n",
			after:  "def f(self):\n    try:\n        work()\n    finally:\n        self._inflight += 1\n",
		},
		{
			name:   "assign lhs target changed",
			before: "def f(self):\n    try:\n        work()\n    finally:\n        self._closed = True\n",
			after:  "def f(self):\n    try:\n        work()\n    finally:\n        unused = True\n",
		},
		{
			name:   "annotated assign lhs target changed",
			before: "def f():\n    try:\n        work()\n    finally:\n        done: bool = True\n",
			after:  "def f():\n    try:\n        work()\n    finally:\n        other: bool = True\n",
		},
		{
			name:   "raise exception changed",
			before: "def f(x):\n    try:\n        work()\n    finally:\n        raise CleanupError(x)\n",
			after:  "def f(x):\n    try:\n        work()\n    finally:\n        raise OtherError(x)\n",
		},
		{
			name:   "raise cause changed",
			before: "def f(x, y):\n    try:\n        work()\n    finally:\n        raise CleanupError(x) from y\n",
			after:  "def f(x, y):\n    try:\n        work()\n    finally:\n        raise CleanupError(x) from None\n",
		},
		{
			name:   "for loop iterable changed in finally",
			before: "def f(a, b):\n    try:\n        work()\n    finally:\n        for item in a:\n            close(item)\n",
			after:  "def f(a, b):\n    try:\n        work()\n    finally:\n        for item in b:\n            close(item)\n",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			safe := strings.ReplaceAll(tc.name, " ", "_")
			code, cands, _, stderr := runPair(t, dir, "fin_"+safe+".py", tc.before, tc.after, "")
			if code != exitOK || !hasKind(cands, "python.finally_body_change") {
				t.Fatalf("%s: expected python.finally_body_change, got code=%d kinds=%v stderr=%s", tc.name, code, kindsOf(cands), stderr)
			}
		})
	}
}

func TestExpectationAndConditionPrecision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Slice bounds: res[1:] vs res[:1] and res[1:2:3] vs res[1:2]
	code, cands, _, stderr := runPair(t, dir, "py_slice.py",
		"def test_s(res, x):\n    assert res[1:] == x\n",
		"def test_s(res, x):\n    assert res[:1] == x\n",
		"tests/test_slice.py",
	)
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("slice lower/upper swap: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}
	code, cands, _, stderr = runPair(t, dir, "py_slice_step.py",
		"def test_s(res, x):\n    assert res[1:2:3] == x\n",
		"def test_s(res, x):\n    assert res[1:2] == x\n",
		"tests/test_slice_step.py",
	)
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("slice step drop: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}

	// Lambda body and parameter changes inside assertion
	code, cands, _, stderr = runPair(t, dir, "py_lambda_body.py",
		"def test_l(fn):\n    assert fn(lambda x: x + 1) == 2\n",
		"def test_l(fn):\n    assert fn(lambda x: x - 1) == 2\n",
		"tests/test_lambda.py",
	)
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("lambda body mutation: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}
	code, cands, _, stderr = runPair(t, dir, "py_lambda_param.py",
		"def test_l(fn):\n    assert fn(lambda x: x + 1) == 2\n",
		"def test_l(fn):\n    assert fn(lambda x, y=0: x + 1) == 2\n",
		"tests/test_lambda_param.py",
	)
	if code != exitOK || !hasKind(cands, "test.expectation_change") {
		t.Fatalf("lambda param mutation: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}

	// Svelte test file with apostrophe in markup before <script> expect(...)
	beforeSvelte := "<div>don't break the scanner</div>\n<script lang=\"ts\">\n  expect(value).toBe(42);\n</script>\n"
	afterSvelte := "<div>don't break the scanner</div>\n<script lang=\"ts\">\n  expect(value).toBeTruthy();\n</script>\n"
	for _, logical := range []string{"alt-frontend-sv/src/card.spec.svelte", "alt-frontend-sv/src/card.test.svelte"} {
		code, cands, _, stderr = runPair(t, dir, filepath.Base(logical), beforeSvelte, afterSvelte, logical)
		if code != exitOK || !hasKind(cands, "test.expectation_change") {
			t.Fatalf("%s: expected test.expectation_change, got code=%d cands=%+v stderr=%s", logical, code, cands, stderr)
		}
	}

	// collectCond traverses keyword args and star args inside if-condition calls
	code, cands, _, stderr = runPair(t, dir, "py_cond_kw.py",
		"def f(a, b):\n    if check(strict=a and b):\n        return 1\n    return 0\n",
		"def f(a, b):\n    if check(strict=a):\n        return 1\n    return 0\n",
		"",
	)
	if code != exitOK || !hasKind(cands, "python.condition_operator_change") {
		t.Fatalf("kwarg condition operator drop: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}
	code, cands, _, stderr = runPair(t, dir, "py_cond_star.py",
		"def f(a, b):\n    if check(*(a and b,)):\n        return 1\n    return 0\n",
		"def f(a, b):\n    if check(*(a,)):\n        return 1\n    return 0\n",
		"",
	)
	if code != exitOK || !hasKind(cands, "python.condition_operator_change") {
		t.Fatalf("star-arg condition operator drop: code=%d cands=%+v stderr=%s", code, cands, stderr)
	}
}

func TestContextAndUsage(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, err bytes.Buffer
	if code := Run(ctx, nil, &out, &err); code != exitUsage {
		t.Fatalf("canceled code=%d", code)
	}
	code, _, _, stderr := runCLI(t)
	if code != exitUsage || !strings.Contains(stderr, "specify either") {
		t.Fatalf("usage code=%d stderr=%s", code, stderr)
	}
	code, _, stdout, _ := runCLI(t, "--help")
	if code != exitOK || !strings.Contains(stdout, "--before") {
		t.Fatalf("help code=%d out=%s", code, stdout)
	}
}

func TestDeterministic(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	before := filepath.Join(root, "scripts", "harness", "corpus", "C-01", "before.go")
	after := filepath.Join(root, "scripts", "harness", "corpus", "C-01", "after.go")
	_, _, a, _ := runCLI(t, "--before", before, "--after", after, "--logical-path", "corpus/C-01/after.go")
	_, _, b, _ := runCLI(t, "--before", before, "--after", after, "--logical-path", "corpus/C-01/after.go")
	if a != b {
		t.Fatalf("nondeterministic\n%s\n%s", a, b)
	}
}

func runCLI(t *testing.T, args ...string) (int, []Candidate, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	var cands []Candidate
	raw := strings.TrimSpace(stdout.String())
	if (code == exitOK || code == exitCandidates) && strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal(stdout.Bytes(), &cands); err != nil {
			t.Fatalf("json: %v\n%s", err, stdout.String())
		}
	}
	return code, cands, stdout.String(), stderr.String()
}

func runPair(t *testing.T, dir, name, before, after, logical string) (int, []Candidate, string, string) {
	t.Helper()
	b := filepath.Join(dir, "b_"+name)
	a := filepath.Join(dir, "a_"+name)
	writeFile(t, b, before)
	writeFile(t, a, after)
	args := []string{"--before", b, "--after", a}
	if logical != "" {
		args = append(args, "--logical-path", logical)
	}
	return runCLI(t, args...)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "lint", "behavior_screener.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func corpusFile(t *testing.T, corpus, id, which string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(corpus, id, which+".*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		switch filepath.Ext(m) {
		case ".go", ".py", ".rs", ".ts":
			return m
		}
	}
	t.Fatalf("%s %s missing", id, which)
	return ""
}

func readCorpus(t *testing.T, root, id, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "scripts", "harness", "corpus", id, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func readCandidates(t *testing.T, path string) []Candidate {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cands []Candidate
	if err := json.Unmarshal(b, &cands); err != nil {
		t.Fatal(err)
	}
	return cands
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func hasKind(cands []Candidate, kind string) bool {
	for _, c := range cands {
		if c.Kind == kind {
			return true
		}
	}
	return false
}

func funcRemoved(cands []Candidate, name string) bool {
	for _, c := range cands {
		if c.Kind == "go.function_removed" && c.Function == name {
			return true
		}
	}
	return false
}

func hasPair(cands []Candidate, file, kind string) bool {
	for _, c := range cands {
		if c.File == file && c.Kind == kind {
			return true
		}
	}
	return false
}

func kindsOf(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Kind
	}
	return out
}

func pairsOf(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.File + ":" + c.Kind
	}
	return out
}
