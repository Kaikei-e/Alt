#!/usr/bin/env python3
"""CHECK-01: AST Behavior-Change Screener (GUARD-02 redesign).

Screens before/after file pairs or git diffs for structural runtime-behavior
change candidates across Go, Python, Rust, and TypeScript, while ignoring safe
lint refactors (formatting, comments, docstrings, type annotations, and
idiomatic Go errors.Is / errors.As rewrites).

CLI modes:
  Pair mode:
    python3 scripts/lint/behavior_screener.py \
      --before <file> --after <file> \
      [--logical-path <rel_path>] [--language go|python|rust|typescript] \
      [--output <path>] [--fail-on-candidates]

  Git diff mode:
    python3 scripts/lint/behavior_screener.py \
      --repo-root <dir> --base <git_rev> [--head <git_rev>] \
      [--output <path>] [--fail-on-candidates]

Exit codes:
  0  Clean (or candidates emitted without --fail-on-candidates)
  1  Candidates detected when --fail-on-candidates is enabled
  2  CLI usage, file, git, or AST helper error
"""

from __future__ import annotations

import argparse
import ast
import hashlib
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
from collections import Counter
from dataclasses import asdict, dataclass
from typing import Any

EXIT_OK = 0
EXIT_CANDIDATES = 1
EXIT_USAGE = 2
PAIR_FILE_COUNT = 2

SCRIPT_DIR = pathlib.Path(__file__).resolve().parent
GOAST_SOURCE = SCRIPT_DIR / "screener" / "goast" / "main.go"
SUPPORTED_LANGUAGES = frozenset({"go", "python", "rust", "typescript"})
EXT_TO_LANG: dict[str, str] = {
    ".go": "go",
    ".py": "python",
    ".rs": "rust",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".mts": "typescript",
    ".cts": "typescript",
    ".js": "typescript",
    ".jsx": "typescript",
}

RUST_OPTION_REGEX_RE = re.compile(r"\bOption\s*<\s*[^>]*\bRegex\b[^>]*>")
RUST_DOT_OK_RE = re.compile(r"\.\s*ok\s*\(\s*\)")
RUST_UNWRAP_OR_DEFAULT_RE = re.compile(r"\.\s*unwrap_or_default\s*\(\s*\)")

TS_WAIT_FOR_TIMEOUT_RE = re.compile(r"\.\s*waitForTimeout\s*\(([^)]*)\)")
TS_WAIT_FOR_LOAD_STATE_RE = re.compile(r"\.\s*waitForLoadState\s*\(([^)]*)\)")

TEST_PATH_RE = re.compile(
    r"(?:^|/)(?:tests?/|e2e/|__tests__/|test_[^/]+\.py$|test-[^/]+\.py$|[^/]+_test\.(?:go|py|rs)$|"
    r"[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?)$)",
    re.IGNORECASE,
)

EXPECTATION_PATTERNS: dict[str, re.Pattern[str]] = {
    "go": re.compile(
        r"\b(?:require|assert)\.\w+\s*\(|\bt\.(?:Fatal|Fatalf|Fatalln|Error|Errorf)\s*\("
    ),
    "python": re.compile(r"^\s*assert\b|\bself\.assert\w+\s*\(|\bpytest\.raises\s*\("),
    "rust": re.compile(r"\b(?:debug_)?assert(?:_eq|_ne)?!\s*\("),
    "typescript": re.compile(r"\bexpect\s*\("),
}


@dataclass(frozen=True)
class Candidate:
    id: str
    file: str
    function: str
    kind: str
    before: str
    after: str


class ScreenerError(RuntimeError):
    """Raised when an external helper or git operation fails."""


def _stable_id(file_path: str, function: str, kind: str) -> str:
    raw = f"{file_path}|{function}|{kind}".encode()
    digest = hashlib.sha256(raw).hexdigest()[:12]
    return f"{kind}:{digest}"


def _make_candidate(
    file_path: str,
    function: str,
    kind: str,
    before: str,
    after: str,
) -> Candidate:
    return Candidate(
        id=_stable_id(file_path, function, kind),
        file=file_path,
        function=function,
        kind=kind,
        before=before,
        after=after,
    )


def infer_language(path_hint: str, explicit: str | None = None) -> str | None:
    if explicit is not None:
        return explicit.lower()
    suffix = pathlib.Path(path_hint).suffix.lower()
    return EXT_TO_LANG.get(suffix)


# ---------------------------------------------------------------------------
# Go AST helper build/cache & structural comparison
# ---------------------------------------------------------------------------


def _goast_cache_dir() -> pathlib.Path:
    xdg = os.environ.get("XDG_CACHE_HOME", "").strip()
    base = pathlib.Path(xdg) if xdg else pathlib.Path.home() / ".cache"
    return base / "alt-lint" / "screener-goast"


def _ensure_goast_binary() -> pathlib.Path:
    if not GOAST_SOURCE.is_file():
        msg = f"missing Go AST helper source: {GOAST_SOURCE}"
        raise ScreenerError(msg)

    src_hash = hashlib.sha256(GOAST_SOURCE.read_bytes()).hexdigest()[:16]
    cache_dir = _goast_cache_dir()
    bin_path = cache_dir / f"goast-{src_hash}"
    if bin_path.is_file() and os.access(bin_path, os.X_OK):
        return bin_path

    go_bin = shutil.which("go")
    if go_bin is None:
        msg = "go toolchain not found in PATH (required for Go AST behavior screening)"
        raise ScreenerError(msg)

    cache_dir.mkdir(parents=True, exist_ok=True)
    tmp_bin = cache_dir / f"goast-{src_hash}.{os.getpid()}.tmp"
    env = dict(os.environ)
    env["GO111MODULE"] = "off"
    res = subprocess.run(  # noqa: S603 -- compiles repo-owned stdlib Go AST helper
        [go_bin, "build", "-o", str(tmp_bin), str(GOAST_SOURCE)],
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    if res.returncode != 0:
        if tmp_bin.exists():
            tmp_bin.unlink()
        msg = f"failed to compile Go AST helper ({GOAST_SOURCE}): {res.stderr.strip()}"
        raise ScreenerError(msg)
    tmp_bin.replace(bin_path)
    return bin_path


def _run_goast(
    before_path: pathlib.Path, after_path: pathlib.Path
) -> tuple[dict[str, Any], dict[str, Any]]:
    goast_bin = _ensure_goast_binary()
    res = subprocess.run(  # noqa: S603 -- executes cached repo-owned Go AST binary
        [str(goast_bin), str(before_path), str(after_path)],
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode != 0:
        msg = f"goast helper failed (rc={res.returncode}): {res.stderr.strip()}"
        raise ScreenerError(msg)
    try:
        parsed = json.loads(res.stdout)
    except json.JSONDecodeError as exc:
        msg = f"invalid JSON from goast helper: {exc}"
        raise ScreenerError(msg) from exc
    if not isinstance(parsed, list) or len(parsed) != PAIR_FILE_COUNT:
        msg = f"unexpected goast output shape: {parsed!r}"
        raise ScreenerError(msg)
    return parsed[0], parsed[1]


def _fmt_counter_map(data: dict[str, int]) -> str:
    if not data:
        return "{}"
    return ", ".join(f"{k}={data[k]}" for k in sorted(data))


def _canonical_http_configs(configs: list[dict[str, str]]) -> list[str]:
    return sorted("{" + ", ".join(f"{k}:{cfg[k]}" for k in sorted(cfg)) + "}" for cfg in configs)


def _canonical_file_modes(items: list[dict[str, str]]) -> list[str]:
    return sorted(f"{item.get('func', '')}({item.get('value', '')})" for item in items)


def _compare_go_function(
    logical_path: str,
    func_name: str,
    before_fn: dict[str, Any],
    after_fn: dict[str, Any],
) -> list[Candidate]:
    candidates: list[Candidate] = []

    b_ops: dict[str, int] = before_fn.get("cond_ops", {})
    a_ops: dict[str, int] = after_fn.get("cond_ops", {})
    if b_ops != a_ops:
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.condition_operator_change",
                _fmt_counter_map(b_ops),
                _fmt_counter_map(a_ops),
            )
        )

    b_ret = int(before_fn.get("return_count", 0))
    a_ret = int(after_fn.get("return_count", 0))
    b_def = int(before_fn.get("defer_count", 0))
    a_def = int(after_fn.get("defer_count", 0))
    if (b_ret, b_def) != (a_ret, a_def):
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.return_or_defer_count_change",
                f"return={b_ret}, defer={b_def}",
                f"return={a_ret}, defer={a_def}",
            )
        )

    b_srv = _canonical_http_configs(before_fn.get("http_server_configs", []))
    a_srv = _canonical_http_configs(after_fn.get("http_server_configs", []))
    if b_srv != a_srv:
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.http_server_fields_change",
                "; ".join(b_srv) or "none",
                "; ".join(a_srv) or "none",
            )
        )

    b_modes = _canonical_file_modes(before_fn.get("file_modes", []))
    a_modes = _canonical_file_modes(after_fn.get("file_modes", []))
    if b_modes != a_modes:
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.file_mode_change",
                ", ".join(b_modes) or "none",
                ", ".join(a_modes) or "none",
            )
        )

    b_limits = sorted(str(x) for x in before_fn.get("body_limits", []))
    a_limits = sorted(str(x) for x in after_fn.get("body_limits", []))
    if b_limits != a_limits:
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.body_limit_change",
                ", ".join(b_limits) or "none",
                ", ".join(a_limits) or "none",
            )
        )

    b_rp = sorted(str(x) for x in before_fn.get("reverse_proxy_modes", []))
    a_rp = sorted(str(x) for x in after_fn.get("reverse_proxy_modes", []))
    if b_rp != a_rp:
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.reverse_proxy_rewrite_change",
                ", ".join(b_rp) or "none",
                ", ".join(a_rp) or "none",
            )
        )

    b_esc: dict[str, int] = before_fn.get("output_escapers", {})
    a_esc: dict[str, int] = after_fn.get("output_escapers", {})
    if b_esc != a_esc:
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.output_escaping_change",
                _fmt_counter_map(b_esc),
                _fmt_counter_map(a_esc),
            )
        )

    b_discard = int(before_fn.get("slog_discard_count", 0))
    a_discard = int(after_fn.get("slog_discard_count", 0))
    if a_discard > b_discard:
        candidates.append(
            _make_candidate(
                logical_path,
                func_name,
                "go.test_logger_discard_change",
                f"slog.DiscardHandler={b_discard}, other_handlers={before_fn.get('slog_other_handlers', 0)}",
                f"slog.DiscardHandler={a_discard}, other_handlers={after_fn.get('slog_other_handlers', 0)}",
            )
        )

    return candidates


def screen_go(
    before_path: pathlib.Path,
    after_path: pathlib.Path,
    logical_path: str,
) -> list[Candidate]:
    before_summary, after_summary = _run_goast(before_path, after_path)
    if before_summary.get("parse_err") or after_summary.get("parse_err"):
        return []

    before_funcs: dict[str, dict[str, Any]] = before_summary.get("functions", {})
    after_funcs: dict[str, dict[str, Any]] = after_summary.get("functions", {})
    candidates: list[Candidate] = []

    for func_name in sorted(set(before_funcs) & set(after_funcs)):
        candidates.extend(
            _compare_go_function(
                logical_path,
                func_name,
                before_funcs[func_name],
                after_funcs[func_name],
            )
        )
    return candidates


# ---------------------------------------------------------------------------
# Python AST structural comparison
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class PyFuncFacts:
    if_count: int
    if_else_count: int
    try_else_count: int
    loop_else_count: int
    return_count: int
    context_managers: tuple[str, ...]
    finally_hashes: tuple[str, ...]
    except_types: tuple[str, ...]


def _is_docstring_stmt(stmt: ast.stmt) -> bool:
    return (
        isinstance(stmt, ast.Expr)
        and isinstance(stmt.value, ast.Constant)
        and isinstance(stmt.value.value, str)
    )


def _hash_stmt_list(stmts: list[ast.stmt]) -> str:
    filtered = [s for s in stmts if not _is_docstring_stmt(s)]
    dumped = "\n".join(ast.dump(s, include_attributes=False) for s in filtered)
    return hashlib.sha256(dumped.encode("utf-8")).hexdigest()[:16]


def _extract_except_names(handler: ast.ExceptHandler) -> list[str]:
    if handler.type is None:
        return ["<bare-except>"]
    if isinstance(handler.type, ast.Tuple):
        return [ast.unparse(elt) for elt in handler.type.elts]
    return [ast.unparse(handler.type)]


class _PyFuncCollector:
    def __init__(self) -> None:
        self.if_count = 0
        self.if_else_count = 0
        self.try_else_count = 0
        self.loop_else_count = 0
        self.return_count = 0
        self.context_managers: list[str] = []
        self.finally_hashes: list[str] = []
        self.except_types: set[str] = set()

    def walk_body(self, stmts: list[ast.stmt]) -> None:
        for stmt in stmts:
            self._visit_stmt(stmt)

    def _visit_try(self, stmt: ast.Try | ast.TryStar) -> None:
        if stmt.orelse:
            self.try_else_count += 1
        if stmt.finalbody:
            self.finally_hashes.append(_hash_stmt_list(stmt.finalbody))
        for handler in stmt.handlers:
            self.except_types.update(_extract_except_names(handler))
            self.walk_body(handler.body)
        self.walk_body(stmt.body)
        self.walk_body(stmt.orelse)
        self.walk_body(stmt.finalbody)

    def _visit_stmt(self, stmt: ast.stmt) -> None:
        if isinstance(stmt, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            return
        if isinstance(stmt, ast.If):
            self.if_count += 1
            if stmt.orelse:
                self.if_else_count += 1
            self.walk_body(stmt.body)
            self.walk_body(stmt.orelse)
        elif isinstance(stmt, (ast.Try, ast.TryStar)):
            self._visit_try(stmt)
        elif isinstance(stmt, (ast.For, ast.AsyncFor, ast.While)):
            if stmt.orelse:
                self.loop_else_count += 1
            self.walk_body(stmt.body)
            self.walk_body(stmt.orelse)
        elif isinstance(stmt, (ast.With, ast.AsyncWith)):
            prefix = "async with" if isinstance(stmt, ast.AsyncWith) else "with"
            self.context_managers.extend(
                f"{prefix}:{ast.unparse(item.context_expr)}" for item in stmt.items
            )
            self.walk_body(stmt.body)
        elif isinstance(stmt, ast.Match):
            for case in stmt.cases:
                self.walk_body(case.body)
        elif isinstance(stmt, ast.Return):
            self.return_count += 1

    def to_facts(self) -> PyFuncFacts:
        return PyFuncFacts(
            if_count=self.if_count,
            if_else_count=self.if_else_count,
            try_else_count=self.try_else_count,
            loop_else_count=self.loop_else_count,
            return_count=self.return_count,
            context_managers=tuple(sorted(self.context_managers)),
            finally_hashes=tuple(self.finally_hashes),
            except_types=tuple(sorted(self.except_types)),
        )


def _analyze_python_function(fn: ast.FunctionDef | ast.AsyncFunctionDef) -> PyFuncFacts:
    collector = _PyFuncCollector()
    collector.walk_body(fn.body)
    return collector.to_facts()


def _extract_python_functions(source: str) -> dict[str, PyFuncFacts] | None:
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return None

    results: dict[str, PyFuncFacts] = {}

    def visit_block(stmts: list[ast.stmt], prefix: str) -> None:
        for stmt in stmts:
            if isinstance(stmt, ast.ClassDef):
                cls_prefix = f"{prefix}.{stmt.name}" if prefix else stmt.name
                visit_block(stmt.body, cls_prefix)
            elif isinstance(stmt, (ast.FunctionDef, ast.AsyncFunctionDef)):
                qual = f"{prefix}.{stmt.name}" if prefix else stmt.name
                results[qual] = _analyze_python_function(stmt)
                visit_block(stmt.body, qual)

    visit_block(tree.body, "")
    return results


def screen_python(before_src: str, after_src: str, logical_path: str) -> list[Candidate]:
    before_funcs = _extract_python_functions(before_src)
    after_funcs = _extract_python_functions(after_src)
    if before_funcs is None or after_funcs is None:
        return []

    candidates: list[Candidate] = []
    for func_name in sorted(set(before_funcs) & set(after_funcs)):
        bf = before_funcs[func_name]
        af = after_funcs[func_name]

        b_cf = (
            bf.if_count,
            bf.if_else_count,
            bf.try_else_count,
            bf.loop_else_count,
            bf.return_count,
        )
        a_cf = (
            af.if_count,
            af.if_else_count,
            af.try_else_count,
            af.loop_else_count,
            af.return_count,
        )
        if b_cf != a_cf:
            candidates.append(
                _make_candidate(
                    logical_path,
                    func_name,
                    "python.control_flow_return_change",
                    (
                        f"if={bf.if_count}, if_else={bf.if_else_count}, "
                        f"try_else={bf.try_else_count}, loop_else={bf.loop_else_count}, "
                        f"return={bf.return_count}"
                    ),
                    (
                        f"if={af.if_count}, if_else={af.if_else_count}, "
                        f"try_else={af.try_else_count}, loop_else={af.loop_else_count}, "
                        f"return={af.return_count}"
                    ),
                )
            )

        if bf.context_managers != af.context_managers:
            candidates.append(
                _make_candidate(
                    logical_path,
                    func_name,
                    "python.context_manager_change",
                    ", ".join(bf.context_managers) or "none",
                    ", ".join(af.context_managers) or "none",
                )
            )

        if bf.finally_hashes != af.finally_hashes:
            candidates.append(
                _make_candidate(
                    logical_path,
                    func_name,
                    "python.finally_body_change",
                    ", ".join(bf.finally_hashes) or "none",
                    ", ".join(af.finally_hashes) or "none",
                )
            )

        if bf.except_types != af.except_types:
            candidates.append(
                _make_candidate(
                    logical_path,
                    func_name,
                    "python.except_types_change",
                    ", ".join(bf.except_types) or "none",
                    ", ".join(af.except_types) or "none",
                )
            )

    return candidates


# ---------------------------------------------------------------------------
# Comment/string stripping for Rust & TypeScript structural checks
# ---------------------------------------------------------------------------


def _skip_block_comment(src: str, start: int, *, nested: bool, out: list[str]) -> int:
    depth = 1
    idx = start + 2
    n = len(src)
    while idx < n and depth > 0:
        if nested and src[idx : idx + 2] == "/*":
            depth += 1
            idx += 2
        elif src[idx : idx + 2] == "*/":
            depth -= 1
            idx += 2
        else:
            if src[idx] == "\n":
                out.append("\n")
            idx += 1
    out.append(" ")
    return idx


def _try_skip_rust_raw_string(src: str, idx: int, out: list[str]) -> int | None:
    prev_is_ident = idx > 0 and (src[idx - 1].isalnum() or src[idx - 1] == "_")
    if prev_is_ident:
        return None
    n = len(src)
    j = idx + 1
    while j < n and src[j] == "#":
        j += 1
    if j >= n or src[j] != '"':
        return None
    hashes = j - (idx + 1)
    end_delim = '"' + ("#" * hashes)
    end_pos = src.find(end_delim, j + 1)
    out.append('""')
    return n if end_pos == -1 else end_pos + len(end_delim)


def _skip_quoted_string(src: str, start: int, out: list[str]) -> int:
    quote = src[start]
    out.append(quote)
    idx = start + 1
    n = len(src)
    while idx < n:
        c = src[idx]
        if c == "\\":
            idx += 2
            continue
        if c == quote:
            out.append(quote)
            return idx + 1
        if c == "\n":
            out.append("\n")
        idx += 1
    return idx


def _strip_c_style_comments_and_strings(src: str, *, rust_raw_strings: bool = False) -> str:
    """Replace comments and string literal bodies with spaces, preserving code structure."""
    out: list[str] = []
    i = 0
    n = len(src)
    while i < n:
        ch = src[i]
        nxt = src[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "/":
            nl = src.find("\n", i + 2)
            i = n if nl == -1 else nl
        elif ch == "/" and nxt == "*":
            i = _skip_block_comment(src, i, nested=rust_raw_strings, out=out)
        elif rust_raw_strings and ch == "r":
            next_i = _try_skip_rust_raw_string(src, i, out)
            if next_i is not None:
                i = next_i
            else:
                out.append(ch)
                i += 1
        elif ch in ('"', "`") or (not rust_raw_strings and ch == "'"):
            i = _skip_quoted_string(src, i, out)
        else:
            out.append(ch)
            i += 1
    return "".join(out)


def screen_rust(before_src: str, after_src: str, logical_path: str) -> list[Candidate]:
    b_clean = _strip_c_style_comments_and_strings(before_src, rust_raw_strings=True)
    a_clean = _strip_c_style_comments_and_strings(after_src, rust_raw_strings=True)

    b_counts = {
        "Option<Regex>": len(RUST_OPTION_REGEX_RE.findall(b_clean)),
        ".ok()": len(RUST_DOT_OK_RE.findall(b_clean)),
        ".unwrap_or_default()": len(RUST_UNWRAP_OR_DEFAULT_RE.findall(b_clean)),
    }
    a_counts = {
        "Option<Regex>": len(RUST_OPTION_REGEX_RE.findall(a_clean)),
        ".ok()": len(RUST_DOT_OK_RE.findall(a_clean)),
        ".unwrap_or_default()": len(RUST_UNWRAP_OR_DEFAULT_RE.findall(a_clean)),
    }

    increased = [k for k in b_counts if a_counts[k] > b_counts[k]]
    if not increased:
        return []

    return [
        _make_candidate(
            logical_path,
            "<file>",
            "rust.fallible_error_swallow",
            _fmt_counter_map(b_counts),
            _fmt_counter_map(a_counts),
        )
    ]


def _extract_ts_waits(cleaned_src: str) -> tuple[list[str], list[str]]:
    timeouts = [" ".join(m.group(1).split()) for m in TS_WAIT_FOR_TIMEOUT_RE.finditer(cleaned_src)]
    load_states = [
        " ".join(m.group(1).split()) for m in TS_WAIT_FOR_LOAD_STATE_RE.finditer(cleaned_src)
    ]
    return timeouts, load_states


def screen_typescript(before_src: str, after_src: str, logical_path: str) -> list[Candidate]:
    b_clean = _strip_c_style_comments_and_strings(before_src, rust_raw_strings=False)
    a_clean = _strip_c_style_comments_and_strings(after_src, rust_raw_strings=False)

    b_timeouts, b_loads = _extract_ts_waits(b_clean)
    a_timeouts, a_loads = _extract_ts_waits(a_clean)

    timeout_dropped_or_weakened = len(a_timeouts) < len(b_timeouts) or (
        b_timeouts != a_timeouts and len(b_timeouts) > 0
    )
    replaced_with_load_state = len(a_timeouts) < len(b_timeouts) and len(a_loads) > len(b_loads)

    if not (timeout_dropped_or_weakened or replaced_with_load_state):
        return []

    return [
        _make_candidate(
            logical_path,
            "<file>",
            "ts.playwright_wait_change",
            f"waitForTimeout={len(b_timeouts)} ({b_timeouts}), waitForLoadState={len(b_loads)}",
            f"waitForTimeout={len(a_timeouts)} ({a_timeouts}), waitForLoadState={len(a_loads)}",
        )
    ]


# ---------------------------------------------------------------------------
# Cross-language existing test file expectation change detection
# ---------------------------------------------------------------------------


def _is_test_file(logical_path: str, language: str, before_src: str) -> bool:
    norm = logical_path.replace("\\", "/")
    if TEST_PATH_RE.search(norm) is not None:
        return True
    return language == "rust" and ("#[test]" in before_src or "#[cfg(test)]" in before_src)


def _strip_line_comment(line: str, language: str) -> str:
    marker = "#" if language == "python" else "//"
    in_quote: str | None = None
    escaped = False
    for idx, ch in enumerate(line):
        if escaped:
            escaped = False
            continue
        if in_quote is not None:
            if ch == "\\":
                escaped = True
            elif ch == in_quote:
                in_quote = None
            continue
        if ch in ('"', "'", "`"):
            in_quote = ch
        elif line[idx : idx + len(marker)] == marker:
            return line[:idx]
    return line


def _extract_expectation_lines(src: str, language: str) -> Counter[str]:
    pattern = EXPECTATION_PATTERNS.get(language)
    if pattern is None:
        return Counter()
    counts: Counter[str] = Counter()
    for raw_line in src.splitlines():
        code_part = _strip_line_comment(raw_line, language).strip()
        if not code_part:
            continue
        if pattern.search(code_part) is not None:
            normalized = " ".join(code_part.split())
            counts[normalized] += 1
    return counts


def screen_test_expectations(
    before_src: str,
    after_src: str,
    logical_path: str,
    language: str,
) -> list[Candidate]:
    if not _is_test_file(logical_path, language, before_src):
        return []
    b_exp = _extract_expectation_lines(before_src, language)
    a_exp = _extract_expectation_lines(after_src, language)
    removed = b_exp - a_exp
    if not removed:
        return []
    added = a_exp - b_exp
    before_summary = "; ".join(sorted(removed.keys()))[:240]
    after_summary = ("; ".join(sorted(added.keys())) or "<removed>")[:240]
    return [
        _make_candidate(
            logical_path,
            "<test>",
            "test.expectation_change",
            before_summary,
            after_summary,
        )
    ]


# ---------------------------------------------------------------------------
# High-level screening entry points
# ---------------------------------------------------------------------------


def screen_pair(
    before_path: pathlib.Path,
    after_path: pathlib.Path,
    *,
    logical_path: str | None = None,
    language: str | None = None,
) -> list[Candidate]:
    effective_path = logical_path or str(after_path)
    lang = infer_language(effective_path, language) or infer_language(str(before_path))
    if lang is None or lang not in SUPPORTED_LANGUAGES:
        return []

    before_src = before_path.read_text(encoding="utf-8", errors="replace")
    after_src = after_path.read_text(encoding="utf-8", errors="replace")
    if before_src == after_src:
        return []

    candidates: list[Candidate] = []
    if lang == "go":
        candidates.extend(screen_go(before_path, after_path, effective_path))
    elif lang == "python":
        candidates.extend(screen_python(before_src, after_src, effective_path))
    elif lang == "rust":
        candidates.extend(screen_rust(before_src, after_src, effective_path))
    elif lang == "typescript":
        candidates.extend(screen_typescript(before_src, after_src, effective_path))

    candidates.extend(screen_test_expectations(before_src, after_src, effective_path, lang))
    return candidates


def _run_git(repo_root: pathlib.Path, *git_args: str) -> subprocess.CompletedProcess[str]:
    git_bin = shutil.which("git")
    if git_bin is None:
        msg = "git executable not found in PATH"
        raise ScreenerError(msg)
    return subprocess.run(  # noqa: S603 -- runs git with fixed repo_root and explicit args
        [git_bin, "-C", str(repo_root), *git_args],
        capture_output=True,
        text=True,
        check=False,
    )


def _git_show_file(repo_root: pathlib.Path, rev: str, rel_path: str) -> str | None:
    res = _run_git(repo_root, "show", f"{rev}:{rel_path}")
    if res.returncode != 0:
        return None
    return res.stdout


def _parse_git_diff_entries(stdout: str) -> list[tuple[str, str]]:
    tokens = stdout.split("\0")
    entries: list[tuple[str, str]] = []
    idx = 0
    while idx < len(tokens):
        status = tokens[idx]
        if not status:
            idx += 1
            continue
        if status.startswith(("R", "C")):
            if idx + 2 < len(tokens):
                entries.append((tokens[idx + 1], tokens[idx + 2]))
            idx += 3
        elif status == "M":
            if idx + 1 < len(tokens):
                rel = tokens[idx + 1]
                entries.append((rel, rel))
            idx += 2
        else:
            idx += 2
    return entries


def _load_git_pair_texts(
    repo_root: pathlib.Path,
    base_rev: str,
    head_rev: str | None,
    old_rel: str,
    new_rel: str,
) -> tuple[str, str] | None:
    before_text = _git_show_file(repo_root, base_rev, old_rel)
    if before_text is None:
        return None
    if head_rev is not None:
        after_text = _git_show_file(repo_root, head_rev, new_rel)
        if after_text is None:
            return None
        return before_text, after_text
    after_disk = repo_root / new_rel
    if not after_disk.is_file():
        return None
    return before_text, after_disk.read_text(encoding="utf-8", errors="replace")


def screen_git_diff(
    repo_root: pathlib.Path,
    base_rev: str,
    head_rev: str | None = None,
) -> list[Candidate]:
    diff_args = ["diff", "--name-status", "-z", base_rev]
    if head_rev is not None:
        diff_args.append(head_rev)
    res = _run_git(repo_root, *diff_args)
    if res.returncode != 0:
        msg = f"git diff failed: {res.stderr.strip()}"
        raise ScreenerError(msg)

    entries = _parse_git_diff_entries(res.stdout)
    candidates: list[Candidate] = []
    with tempfile.TemporaryDirectory(prefix="alt-screener-git-") as tmp_str:
        tmp_dir = pathlib.Path(tmp_str)
        for pair_idx, (old_rel, new_rel) in enumerate(entries):
            lang = infer_language(new_rel)
            if lang is None:
                continue
            texts = _load_git_pair_texts(repo_root, base_rev, head_rev, old_rel, new_rel)
            if texts is None:
                continue
            before_text, after_text = texts
            suffix = pathlib.Path(new_rel).suffix or f".{lang}"
            b_tmp = tmp_dir / f"before_{pair_idx}{suffix}"
            a_tmp = tmp_dir / f"after_{pair_idx}{suffix}"
            b_tmp.write_text(before_text, encoding="utf-8")
            a_tmp.write_text(after_text, encoding="utf-8")
            candidates.extend(
                screen_pair(
                    b_tmp,
                    a_tmp,
                    logical_path=new_rel,
                    language=lang,
                )
            )
    return candidates


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="CHECK-01 AST Behavior-Change Screener for Go, Python, Rust, and TypeScript.",
    )
    parser.add_argument("--before", type=pathlib.Path, default=None, help="Before file (pair mode)")
    parser.add_argument("--after", type=pathlib.Path, default=None, help="After file (pair mode)")
    parser.add_argument(
        "--logical-path",
        default=None,
        help="Logical repository-relative path for pair mode output",
    )
    parser.add_argument(
        "--language",
        choices=sorted(SUPPORTED_LANGUAGES),
        default=None,
        help="Explicit language override (default: inferred from file extension)",
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=None,
        help="Git repository root (git diff mode)",
    )
    parser.add_argument(
        "--base",
        default=None,
        help="Base git revision (git diff mode)",
    )
    parser.add_argument(
        "--head",
        default=None,
        help="Optional head git revision (default: working tree)",
    )
    parser.add_argument(
        "--output",
        type=pathlib.Path,
        default=None,
        help="Write candidate JSON array to file instead of stdout",
    )
    parser.add_argument(
        "--fail-on-candidates",
        action="store_true",
        default=False,
        help="Exit 1 when one or more behavior-change candidates are detected",
    )
    return parser


def _execute_cli_mode(args: argparse.Namespace) -> list[Candidate]:
    pair_mode = args.before is not None or args.after is not None
    git_mode = args.repo_root is not None or args.base is not None

    if pair_mode == git_mode:
        msg = "specify either pair mode (--before/--after) or git diff mode (--repo-root/--base)"
        raise ScreenerError(msg)

    if pair_mode:
        if args.before is None or args.after is None:
            msg = "pair mode requires both --before and --after"
            raise ScreenerError(msg)
        if not args.before.is_file():
            msg = f"--before file not found: {args.before}"
            raise ScreenerError(msg)
        if not args.after.is_file():
            msg = f"--after file not found: {args.after}"
            raise ScreenerError(msg)
        return screen_pair(
            args.before,
            args.after,
            logical_path=args.logical_path,
            language=args.language,
        )

    if args.repo_root is None or args.base is None:
        msg = "git diff mode requires both --repo-root and --base"
        raise ScreenerError(msg)
    if not args.repo_root.is_dir():
        msg = f"--repo-root directory not found: {args.repo_root}"
        raise ScreenerError(msg)
    return screen_git_diff(args.repo_root, args.base, head_rev=args.head)


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    try:
        candidates = _execute_cli_mode(args)
    except (OSError, ScreenerError) as exc:
        sys.stderr.write(f"error: {exc}\n")
        return EXIT_USAGE

    payload = json.dumps([asdict(c) for c in candidates], ensure_ascii=False, indent=2) + "\n"
    if args.output is not None:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(payload, encoding="utf-8")
        if args.fail_on_candidates and candidates:
            kinds = ", ".join(sorted({c.kind for c in candidates}))
            sys.stderr.write(
                f"CHECK-01 detected {len(candidates)} behavior-change candidate(s): {kinds}\n"
            )
    else:
        sys.stdout.write(payload)

    return EXIT_CANDIDATES if (args.fail_on_candidates and candidates) else EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
