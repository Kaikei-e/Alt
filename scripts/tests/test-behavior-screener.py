#!/usr/bin/env python3
"""TDD unit, corpus, and git-diff tests for CHECK-01 (AST Behavior-Change Screener).

Run:
    python3 scripts/tests/test-behavior-screener.py
"""

from __future__ import annotations

import json
import pathlib
import shutil
import subprocess
import sys
import tempfile
from typing import Any

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCREENER_PATH = ROOT / "scripts" / "lint" / "behavior_screener.py"
GOAST_MAIN_PATH = ROOT / "scripts" / "lint" / "screener" / "goast" / "main.go"
CORPUS_DIR = ROOT / "scripts" / "harness" / "corpus"

EXIT_OK = 0
EXIT_CANDIDATES = 1
EXIT_USAGE = 2
MIN_CORPUS_CASES = 22

EXPECTED_CORPUS_KINDS: dict[str, str] = {
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


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        suffix = f" ({detail})" if detail else ""
        sys.stdout.write(f"  FAIL  {name}{suffix}\n")
        RESULTS.failed += 1


def run_screener(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 -- fixed argv invoking screener with sys.executable
        [sys.executable, str(SCREENER_PATH), *args],
        capture_output=True,
        text=True,
        check=False,
    )


def parse_candidates(res: subprocess.CompletedProcess[str]) -> list[dict[str, Any]]:
    raw = res.stdout.strip()
    if not raw:
        return []
    parsed = json.loads(raw)
    if not isinstance(parsed, list):
        msg = f"expected JSON list, got {type(parsed).__name__}"
        raise TypeError(msg)
    return parsed


def run_pair(
    tmp: pathlib.Path,
    filename: str,
    before_src: str,
    after_src: str,
    *,
    logical_path: str | None = None,
) -> tuple[subprocess.CompletedProcess[str], list[dict[str, Any]]]:
    stem = pathlib.Path(filename).stem
    ext = pathlib.Path(filename).suffix
    before_path = tmp / f"{stem}_before{ext}"
    after_path = tmp / f"{stem}_after{ext}"
    before_path.write_text(before_src, encoding="utf-8")
    after_path.write_text(after_src, encoding="utf-8")
    args = ["--before", str(before_path), "--after", str(after_path)]
    if logical_path is not None:
        args.extend(["--logical-path", logical_path])
    res = run_screener(*args)
    candidates = parse_candidates(res) if res.returncode in (EXIT_OK, EXIT_CANDIDATES) else []
    return res, candidates


def test_layout_invariants() -> None:
    sys.stdout.write("\n[1] File layout & go.mod-free invariant\n")
    check("behavior_screener.py exists", SCREENER_PATH.is_file(), str(SCREENER_PATH))
    check("goast/main.go exists", GOAST_MAIN_PATH.is_file(), str(GOAST_MAIN_PATH))
    screener_dir = ROOT / "scripts" / "lint" / "screener"
    go_mods = list(screener_dir.rglob("go.mod")) if screener_dir.is_dir() else []
    check("no go.mod inside scripts/lint/screener/", len(go_mods) == 0, str(go_mods))


def test_real_corpus_cases() -> None:
    sys.stdout.write("\n[2] Real CORPUS-01 12 target cases (before vs before & before vs after)\n")
    for case_id, expected_kind in EXPECTED_CORPUS_KINDS.items():
        case_dir = CORPUS_DIR / case_id
        before_files = list(case_dir.glob("before.*"))
        after_files = list(case_dir.glob("after.*"))
        # Filter out support files like before.pyproject.toml
        before_file = next(
            (p for p in before_files if p.suffix in (".go", ".py", ".rs", ".ts")),
            None,
        )
        after_file = next(
            (p for p in after_files if p.suffix in (".go", ".py", ".rs", ".ts")),
            None,
        )
        if before_file is None or after_file is None:
            check(f"{case_id} corpus files present", False, f"dir={case_dir}")
            continue

        # 1. before vs before => 0 candidates, exit 0 even with --fail-on-candidates
        res_base = run_screener(
            "--before",
            str(before_file),
            "--after",
            str(before_file),
            "--logical-path",
            f"corpus/{case_id}/{before_file.name}",
            "--fail-on-candidates",
        )
        base_cands = (
            parse_candidates(res_base) if res_base.returncode in (EXIT_OK, EXIT_CANDIDATES) else []
        )
        check(
            f"{case_id} before vs before produces 0 candidates (exit 0)",
            res_base.returncode == EXIT_OK and len(base_cands) == 0,
            f"rc={res_base.returncode} cands={base_cands} err={res_base.stderr!r}",
        )

        # 2. before vs after (default exit 0)
        res_default = run_screener(
            "--before",
            str(before_file),
            "--after",
            str(after_file),
            "--logical-path",
            f"corpus/{case_id}/{after_file.name}",
        )
        default_cands = (
            parse_candidates(res_default)
            if res_default.returncode in (EXIT_OK, EXIT_CANDIDATES)
            else []
        )
        kinds = [c.get("kind") for c in default_cands]
        required_keys = {"id", "file", "function", "kind", "before", "after"}
        schema_ok = all(required_keys.issubset(c.keys()) for c in default_cands)
        check(
            f"{case_id} before vs after emits {expected_kind} (default exit 0)",
            res_default.returncode == EXIT_OK and expected_kind in kinds and schema_ok,
            f"rc={res_default.returncode} kinds={kinds} err={res_default.stderr!r}",
        )

        # 3. before vs after (--fail-on-candidates => exit 1, expected_kind in stdout)
        res_fail = run_screener(
            "--before",
            str(before_file),
            "--after",
            str(after_file),
            "--logical-path",
            f"corpus/{case_id}/{after_file.name}",
            "--fail-on-candidates",
        )
        check(
            f"{case_id} --fail-on-candidates exits 1 with {expected_kind}",
            res_fail.returncode == EXIT_CANDIDATES and expected_kind in res_fail.stdout,
            f"rc={res_fail.returncode} out={res_fail.stdout[:200]!r} err={res_fail.stderr!r}",
        )

    # Specifically verify C-07 emits BOTH python.context_manager_change and python.finally_body_change
    c07_dir = CORPUS_DIR / "C-07"
    res_c07 = run_screener(
        "--before",
        str(c07_dir / "before.py"),
        "--after",
        str(c07_dir / "after.py"),
    )
    c07_kinds = (
        {c.get("kind") for c in parse_candidates(res_c07)} if res_c07.returncode == 0 else set()
    )
    check(
        "C-07 emits both python.context_manager_change and python.finally_body_change",
        {"python.context_manager_change", "python.finally_body_change"}.issubset(c07_kinds),
        f"kinds={sorted(c07_kinds)}",
    )

    # Verify before vs before produces 0 candidates for ALL corpus cases (C-01..C-NN)
    all_case_dirs = sorted(
        p for p in CORPUS_DIR.iterdir() if p.is_dir() and p.name.startswith("C-")
    )
    non_zero_self: list[str] = []
    for cdir in all_case_dirs:
        bfile = next((p for p in sorted(cdir.glob("before.*")) if p.is_file()), None)
        if bfile is None:
            continue
        res_self = run_screener(
            "--before", str(bfile), "--after", str(bfile), "--fail-on-candidates"
        )
        if res_self.returncode != EXIT_OK or parse_candidates(res_self):
            non_zero_self.append(f"{cdir.name}:rc={res_self.returncode}")
    check(
        f"before vs before produces 0 candidates across all {len(all_case_dirs)} corpus cases",
        len(all_case_dirs) >= MIN_CORPUS_CASES and not non_zero_self,
        f"cases={len(all_case_dirs)} bad={non_zero_self}",
    )


def test_go_safe_rewrites_and_changes(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[3] Go safe rewrites (0 candidates) & structural changes\n")

    # Safe: err == io.EOF <-> errors.Is(err, io.EOF)
    before_is = """package p
import "io"
func checkEOF(err error) bool {
    if err == io.EOF {
        return true
    }
    return false
}
"""
    after_is = """package p
import (
    "errors"
    "io"
)
func checkEOF(err error) bool {
    if errors.Is(err, io.EOF) {
        return true
    }
    return false
}
"""
    res, cands = run_pair(tmp, "go_errors_is.go", before_is, after_is)
    check(
        "Go safe rewrite: err == io.EOF -> errors.Is(err, io.EOF) emits 0 candidates",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands} err={res.stderr!r}",
    )

    # Safe: target == err <-> errors.Is(err, target)
    before_is_rev = """package p
import "io"
func checkEOF(err error) bool {
    if io.EOF == err {
        return true
    }
    return false
}
"""
    res, cands = run_pair(tmp, "go_errors_is_rev.go", before_is_rev, after_is)
    check(
        "Go safe rewrite: io.EOF == err -> errors.Is(err, io.EOF) emits 0 candidates",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands} err={res.stderr!r}",
    )

    # Safe: err != io.EOF <-> !errors.Is(err, io.EOF)
    before_not_is = """package p
import "io"
func notEOF(err error) bool {
    if err != io.EOF {
        return true
    }
    return false
}
"""
    after_not_is = """package p
import (
    "errors"
    "io"
)
func notEOF(err error) bool {
    if !errors.Is(err, io.EOF) {
        return true
    }
    return false
}
"""
    res, cands = run_pair(tmp, "go_not_errors_is.go", before_not_is, after_not_is)
    check(
        "Go safe rewrite: err != io.EOF -> !errors.Is(err, io.EOF) emits 0 candidates",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands} err={res.stderr!r}",
    )

    # Bug: err != io.EOF -> errors.Is(err, io.EOF) (dropped negation!)
    after_bug_is = """package p
import (
    "errors"
    "io"
)
func notEOF(err error) bool {
    if errors.Is(err, io.EOF) {
        return true
    }
    return false
}
"""
    res, cands = run_pair(tmp, "go_bug_not_errors_is.go", before_not_is, after_bug_is)
    check(
        "Go bug: err != io.EOF -> errors.Is(err, io.EOF) emits go.condition_operator_change",
        res.returncode == EXIT_OK
        and any(c.get("kind") == "go.condition_operator_change" for c in cands),
        f"cands={cands}",
    )

    # Safe: if v, ok := err.(*MyErr); ok <-> var v *MyErr; if errors.As(err, &v)
    before_as = """package p
type MyErr struct{ Code int }
func (e *MyErr) Error() string { return "x" }
func f(err error) int {
    if v, ok := err.(*MyErr); ok {
        return v.Code
    }
    return 0
}
"""
    after_as = """package p
import "errors"
type MyErr struct{ Code int }
func (e *MyErr) Error() string { return "x" }
func f(err error) int {
    var v *MyErr
    if errors.As(err, &v) {
        return v.Code
    }
    return 0
}
"""
    res, cands = run_pair(tmp, "go_errors_as.go", before_as, after_as)
    check(
        "Go safe rewrite: type assert ok -> errors.As emits 0 candidates",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands} err={res.stderr!r}",
    )

    # Safe: preserved && he.Code >= 500 in errors.As rewrite
    before_as_and = """package p
type HTTPError struct{ Code int }
func (e *HTTPError) Error() string { return "x" }
func f(err error, rec func()) {
    if he, ok := err.(*HTTPError); ok && he.Code >= 500 {
        rec()
    }
}
"""
    after_as_and = """package p
import "errors"
type HTTPError struct{ Code int }
func (e *HTTPError) Error() string { return "x" }
func f(err error, rec func()) {
    var he *HTTPError
    if errors.As(err, &he) && he.Code >= 500 {
        rec()
    }
}
"""
    res, cands = run_pair(tmp, "go_errors_as_and.go", before_as_and, after_as_and)
    check(
        "Go safe rewrite: type assert ok && he.Code >= 500 -> errors.As && he.Code >= 500 emits 0",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands}",
    )

    # Safe: migrating &http.Server{...} to httpserver.New(httpserver.Config{...}) with identical fields
    before_srv = """package main
import (
    "net/http"
    "time"
)
func newSrv(addr string, h http.Handler) *http.Server {
    return &http.Server{
        Addr: addr,
        Handler: h,
        ReadHeaderTimeout: 5 * time.Second,
    }
}
"""
    after_srv = """package main
import (
    "net/http"
    "time"
    "alt/httpserver"
)
func newSrv(addr string, h http.Handler) *http.Server {
    return httpserver.New(httpserver.Config{
        Addr: addr,
        Handler: h,
        ReadHeaderTimeout: 5 * time.Second,
    })
}
"""
    res, cands = run_pair(tmp, "go_http_srv_same.go", before_srv, after_srv)
    check(
        "Go safe rewrite: http.Server -> httpserver.Config with identical fields emits 0",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands}",
    )

    # Return / defer count change
    before_ret = """package p
import "os"
func read(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()
    return nil
}
"""
    after_ret = """package p
import "os"
func read(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    _ = f
    return nil
}
"""
    res, cands = run_pair(tmp, "go_defer_drop.go", before_ret, after_ret)
    check(
        "Go dropped defer emits go.return_or_defer_count_change",
        res.returncode == EXIT_OK
        and any(c.get("kind") == "go.return_or_defer_count_change" for c in cands),
        f"cands={cands}",
    )

    # Go test expectation line change
    before_test = """package p
import "testing"
func TestAdd(t *testing.T) {
    if 1+1 != 2 {
        t.Fatalf("expected 2")
    }
}
"""
    after_test = """package p
import "testing"
func TestAdd(t *testing.T) {
    if 1+1 != 2 {
        t.Fatalf("expected something else")
    }
}
"""
    res, cands = run_pair(
        tmp,
        "go_test_expect.go",
        before_test,
        after_test,
        logical_path="pkg/math_test.go",
    )
    check(
        "Go test file expectation change emits test.expectation_change",
        res.returncode == EXIT_OK
        and any(c.get("kind") == "test.expectation_change" for c in cands),
        f"cands={cands}",
    )


def test_python_safe_and_changes(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[4] Python safe rewrites (0 candidates) & structural changes\n")

    before_py = """def compute(x, y):
    try:
        return x + y
    except ValueError:
        return 0
    finally:
        cleanup(x)
"""
    after_py_safe = '''"""Module docstring added."""


def compute(x: int, y: int) -> int:
    """Function docstring added."""
    # formatting and annotations only
    try:
        return x + y
    except ValueError:
        return 0
    finally:
        cleanup(x)
'''
    res, cands = run_pair(tmp, "py_safe.py", before_py, after_py_safe)
    check(
        "Python comments/docstrings/formatting/type annotations emit 0 candidates",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands} err={res.stderr!r}",
    )

    after_py_exc = """def compute(x: int, y: int) -> int:
    try:
        return x + y
    except Exception:
        return 0
    finally:
        cleanup(x)
"""
    res, cands = run_pair(tmp, "py_exc.py", before_py, after_py_exc)
    check(
        "Python except type widening emits python.except_types_change",
        res.returncode == EXIT_OK
        and any(c.get("kind") == "python.except_types_change" for c in cands),
        f"cands={cands}",
    )

    # Python test expectation change
    before_py_test = """def test_value():
    result = 42
    assert result == 42
"""
    after_py_test = """def test_value():
    result = 42
    assert result >= 0
"""
    res, cands = run_pair(
        tmp,
        "py_test_expect.py",
        before_py_test,
        after_py_test,
        logical_path="tests/test_value.py",
    )
    check(
        "Python test file assert change emits test.expectation_change",
        res.returncode == EXIT_OK
        and any(c.get("kind") == "test.expectation_change" for c in cands),
        f"cands={cands}",
    )


def test_rust_and_ts_edge_cases(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[5] Rust & TypeScript safe comments and behavior changes\n")

    before_rs = """pub fn parse(input: &str) -> Result<usize, std::num::ParseIntError> {
    input.parse::<usize>()
}
"""
    after_rs_comment = """// Note: do not use Option<Regex> or .ok() or .unwrap_or_default() here.
pub fn parse(input: &str) -> Result<usize, std::num::ParseIntError> {
    input.parse::<usize>()
}
"""
    res, cands = run_pair(tmp, "rs_comment.rs", before_rs, after_rs_comment)
    check(
        "Rust comment mentioning Option<Regex>/.ok() emits 0 candidates",
        res.returncode == EXIT_OK and len(cands) == 0,
        f"cands={cands}",
    )

    after_rs_swallow = """pub fn parse(input: &str) -> usize {
    input.parse::<usize>().unwrap_or_default()
}
"""
    res, cands = run_pair(tmp, "rs_unwrap_default.rs", before_rs, after_rs_swallow)
    check(
        "Rust newly added unwrap_or_default() emits rust.fallible_error_swallow",
        res.returncode == EXIT_OK
        and any(c.get("kind") == "rust.fallible_error_swallow" for c in cands),
        f"cands={cands}",
    )

    before_ts_test = """export function run(expect: any) {
  expect(true).toBe(true);
}
"""
    after_ts_test = """export function run(expect: any) {
  expect(true).toBeTruthy();
}
"""
    res, cands = run_pair(
        tmp,
        "ts_test_expect.ts",
        before_ts_test,
        after_ts_test,
        logical_path="alt-frontend-sv/tests/e2e/sample.spec.ts",
    )
    check(
        "TypeScript test file expect() change emits test.expectation_change",
        res.returncode == EXIT_OK
        and any(c.get("kind") == "test.expectation_change" for c in cands),
        f"cands={cands}",
    )


def test_git_diff_mode_and_output_file(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[6] Git diff mode & --output flag\n")
    repo = tmp / "mini_repo"
    repo.mkdir()
    git_bin = shutil.which("git")
    assert git_bin is not None

    def git(*gargs: str) -> None:
        subprocess.run(  # noqa: S603 -- test helper running resolved git binary in temp repo
            [git_bin, "-C", str(repo), *gargs],
            check=True,
            capture_output=True,
            text=True,
        )

    git("init", "-q")
    git("config", "user.email", "test@example.com")
    git("config", "user.name", "Test User")

    py_file = repo / "app.py"
    py_file.write_text(
        (CORPUS_DIR / "C-06" / "before.py").read_text(encoding="utf-8"), encoding="utf-8"
    )
    go_file = repo / "handler.go"
    go_file.write_text(
        (CORPUS_DIR / "C-01" / "before.go").read_text(encoding="utf-8"), encoding="utf-8"
    )
    git("add", ".")
    git("commit", "-q", "-m", "baseline")

    # Clean repo => 0 candidates
    res_clean = run_screener("--repo-root", str(repo), "--base", "HEAD", "--fail-on-candidates")
    check(
        "git diff mode on clean repo emits 0 candidates (exit 0)",
        res_clean.returncode == EXIT_OK and parse_candidates(res_clean) == [],
        f"rc={res_clean.returncode} out={res_clean.stdout!r} err={res_clean.stderr!r}",
    )

    # Modify both files in working tree
    py_file.write_text(
        (CORPUS_DIR / "C-06" / "after.py").read_text(encoding="utf-8"), encoding="utf-8"
    )
    go_file.write_text(
        (CORPUS_DIR / "C-01" / "after.go").read_text(encoding="utf-8"), encoding="utf-8"
    )

    out_json = tmp / "report.json"
    res_dirty = run_screener(
        "--repo-root",
        str(repo),
        "--base",
        "HEAD",
        "--output",
        str(out_json),
        "--fail-on-candidates",
    )
    saved = json.loads(out_json.read_text(encoding="utf-8")) if out_json.is_file() else []
    saved_pairs = {(c.get("file"), c.get("kind")) for c in saved}
    check(
        "git diff mode detects working-tree Go & Python regressions and writes --output",
        res_dirty.returncode == EXIT_CANDIDATES
        and ("app.py", "python.control_flow_return_change") in saved_pairs
        and ("handler.go", "go.condition_operator_change") in saved_pairs,
        f"rc={res_dirty.returncode} pairs={saved_pairs} err={res_dirty.stderr!r}",
    )

    # Commit and compare HEAD~1 .. HEAD
    git("add", ".")
    git("commit", "-q", "-m", "regression")
    res_rev = run_screener(
        "--repo-root",
        str(repo),
        "--base",
        "HEAD~1",
        "--head",
        "HEAD",
    )
    rev_cands = parse_candidates(res_rev) if res_rev.returncode == EXIT_OK else []
    rev_pairs = {(c.get("file"), c.get("kind")) for c in rev_cands}
    check(
        "git diff mode --base HEAD~1 --head HEAD detects committed regressions",
        res_rev.returncode == EXIT_OK
        and ("app.py", "python.control_flow_return_change") in rev_pairs
        and ("handler.go", "go.condition_operator_change") in rev_pairs,
        f"rc={res_rev.returncode} pairs={rev_pairs}",
    )


def main() -> int:
    sys.stdout.write("CHECK-01 AST Behavior-Change Screener tests\n")
    test_layout_invariants()
    if SCREENER_PATH.is_file():
        test_real_corpus_cases()
        with tempfile.TemporaryDirectory(prefix="alt-screener-test-") as tmpdir:
            tmp = pathlib.Path(tmpdir)
            test_go_safe_rewrites_and_changes(tmp)
            test_python_safe_and_changes(tmp)
            test_rust_and_ts_edge_cases(tmp)
            test_git_diff_mode_and_output_file(tmp)

    sys.stdout.write(f"\nSummary: {RESULTS.passed} passed, {RESULTS.failed} failed\n")
    return EXIT_OK if RESULTS.failed == 0 else EXIT_CANDIDATES


if __name__ == "__main__":
    sys.exit(main())
