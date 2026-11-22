#!/usr/bin/env python3
"""Enforce inline suppression count ratchet and block-new rule fingerprint baselines (CHECK-02).

Capabilities:
1. Inline suppression count ratchet (`scripts/lint/ratchet/suppressions.toml`):
   - Counts inline suppressions per `(target, kind)` across tracked non-excluded sources
     (python_noqa, rust_allow, go_nosec, go_nolint, ts_biome_ignore, ts_deno_ignore).
   - Fails if ANY `(target, kind)` increases above its recorded ceiling.
   - Fails if ANY `(target, kind)` decreases below its recorded ceiling, instructing the
     author to run `--update` in the same commit so the ceiling drops monotonically.
   - Preserves optional `[governance] max_judgment_rules = <int>` on `--update` unless
     `--max-judgment-rules` is explicitly supplied.
2. `block-new` rule fingerprint baseline (`scripts/lint/ratchet/rule_baselines.toml`):
   - Computes `(rule, rel_file, sha256(normalized_source_line))` fingerprints from a
     linter JSON report (`--findings-json`, supporting Ruff JSON or generic `{rule, file, line}`).
   - Existing fingerprinted violations pass (`exit 0`); any NEW fingerprint fails (`exit 1`).
   - Removing an existing violation lowers the baseline via `--update-fingerprints`.

Run:
    python3 scripts/lint/check_ratchet.py
    python3 scripts/lint/check_ratchet.py --update
    python3 scripts/lint/check_ratchet.py check-fingerprints --findings-json report.json
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import pathlib
import re
import shutil
import subprocess
import sys
import tokenize
import tomllib
from collections import Counter
from dataclasses import dataclass
from urllib.parse import unquote, urlparse

EXIT_CLEAN = 0
EXIT_VIOLATIONS = 1
EXIT_USAGE_ERROR = 2

SOURCE_EXTENSIONS = (".py", ".go", ".rs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte")
TS_EXTENSIONS = (".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte")
MARKER_FILENAMES = frozenset(
    {"go.mod", "Cargo.toml", "pyproject.toml", "deno.json", "bun.lock", "buf.yaml"}
)

GENERATED_HEADER_RE = re.compile(r"Code generated .* DO NOT EDIT\.?", re.IGNORECASE)
GENERATED_HEADER_LINES = 40

PY_FILE_NOQA_RE = re.compile(r"^#\s*(?:ruff|flake8)\s*:\s*noqa\b", re.IGNORECASE)
PY_INLINE_NOQA_RE = re.compile(r"#\s*noqa\b", re.IGNORECASE)
PY_NOSEC_RE = re.compile(r"#\s*nosec\b", re.IGNORECASE)
GO_NOLINT_RE = re.compile(r"^(?://|/\*)\s*nolint(?::|(?=$|[\s*/]))", re.DOTALL)
GO_NOSEC_RE = re.compile(r"^(?://|/\*)\s*#nosec\b", re.DOTALL)
GOSEC_MSG_RULE_RE = re.compile(r"^(G\d{3}):")
RUST_ALLOW_OR_EXPECT_RE = re.compile(r"^#\s*!?\s*\[\s*(?:allow|expect)\s*\(", re.DOTALL)
TS_BIOME_LINE_RE = re.compile(r"\bbiome-ignore\b(?!-all)")
TS_DENO_LINE_RE = re.compile(r"\bdeno-lint-ignore\b(?!-file)")

SUPPRESSION_KINDS = (
    "go_nolint",
    "go_nosec",
    "python_noqa",
    "python_nosec",
    "rust_allow",
    "ts_biome_ignore",
    "ts_deno_ignore",
)

FingerprintKey = tuple[str, str, str]


@dataclass(frozen=True)
class FindingItem:
    rule: str
    rel_file: str
    line: int
    line_sha256: str


def is_excluded_source_path(rel_path: str) -> bool:
    """Return True if a repository-relative path is excluded from suppression counting."""
    norm = rel_path.lstrip("./")
    if norm.startswith(("stopped-using-k8s/", "scripts/harness/corpus/")):
        return True
    if norm.endswith((".pb.go", ".connect.go", "server.gen.go", ".md")):
        return True
    if norm.startswith(("gen/", "alt-frontend-sv/src/lib/gen/")) or "/gen/" in norm:
        return True
    return not norm.endswith(SOURCE_EXTENSIONS)


def is_generated_source_text(text: str) -> bool:
    """Return True if the first 40 lines contain a standard DO NOT EDIT header."""
    return any(
        GENERATED_HEADER_RE.search(line) for line in text.splitlines()[:GENERATED_HEADER_LINES]
    )


def normalize_source_line(line: str) -> str:
    """Collapse whitespace for line-number-independent violation fingerprinting."""
    return " ".join(line.split())


def line_fingerprint_sha256(line: str) -> str:
    """Return SHA-256 hex digest of whitespace-normalized source line content."""
    return hashlib.sha256(normalize_source_line(line).encode("utf-8")).hexdigest()


def _skip_quoted(text: str, pos: int, quote: str) -> int:
    n = len(text)
    pos += 1
    while pos < n:
        ch = text[pos]
        if ch == "\\" and pos + 1 < n:
            pos += 2
            continue
        if ch == "\n":
            return pos
        if ch == quote:
            return pos + 1
        pos += 1
    return pos


def _scan_go_comments(text: str) -> list[str]:
    comments: list[str] = []
    n = len(text)
    i = 0
    while i < n:
        ch = text[i]
        if ch in ('"', "'"):
            i = _skip_quoted(text, i, ch)
        elif ch == "`":
            end = text.find("`", i + 1)
            if end == -1:
                break
            i = end + 1
        elif ch == "/" and i + 1 < n and text[i + 1] == "/":
            end = text.find("\n", i)
            end = n if end == -1 else end
            comments.append(text[i:end])
            i = end
        elif ch == "/" and i + 1 < n and text[i + 1] == "*":
            end = text.find("*/", i + 2)
            end = n if end == -1 else end + 2
            comments.append(text[i:end])
            i = end
        else:
            i += 1
    return comments


def _skip_rust_raw_string(text: str, pos: int) -> int | None:
    n = len(text)
    j = pos
    if j < n and text[j] in ("b", "c"):
        j += 1
    if j >= n or text[j] != "r":
        return None
    j += 1
    hashes = 0
    while j < n and text[j] == "#":
        hashes += 1
        j += 1
    if j >= n or text[j] != '"':
        return None
    closing = '"' + ("#" * hashes)
    end = text.find(closing, j + 1)
    return n if end == -1 else end + len(closing)


def _skip_rust_block_comment(text: str, pos: int) -> int:
    n = len(text)
    depth = 1
    i = pos + 2
    while i < n and depth > 0:
        if i + 1 < n and text[i : i + 2] == "/*":
            depth += 1
            i += 2
        elif i + 1 < n and text[i : i + 2] == "*/":
            depth -= 1
            i += 2
        else:
            i += 1
    return i


def _skip_rust_char_literal(text: str, pos: int) -> int | None:
    n = len(text)
    if pos + 2 >= n or text[pos] != "'":
        return None
    if text[pos + 1] == "\\":
        j = pos + 2
        while j < n and text[j] not in ("'", "\n"):
            j += 1
        return j + 1 if j < n and text[j] == "'" else None
    if text[pos + 1] != "\n" and text[pos + 2] == "'":
        return pos + 3
    return None


def _match_rust_attr_bracket(text: str, pos: int) -> int | None:
    """Return index of '[' if pos ('#') starts a Rust outer/inner attribute (with optional spaces)."""
    n = len(text)
    if pos >= n or text[pos] != "#":
        return None
    j = pos + 1
    while j < n and text[j].isspace():
        j += 1
    if j < n and text[j] == "!":
        j += 1
        while j < n and text[j].isspace():
            j += 1
    if j < n and text[j] == "[":
        return j
    return None


def _read_rust_attr(text: str, pos: int, bracket_pos: int) -> tuple[str, int]:
    n = len(text)
    i = bracket_pos
    depth = 0
    while i < n:
        ch = text[i]
        if ch == '"':
            i = _skip_quoted(text, i, '"')
        elif ch == "[":
            depth += 1
            i += 1
        elif ch == "]":
            depth -= 1
            i += 1
            if depth == 0:
                break
        else:
            i += 1
    return text[pos:i], i


def _skip_rust_non_attr_token(text: str, pos: int) -> int | None:
    n = len(text)
    ch = text[pos]
    result: int | None = None
    if ch == "/" and pos + 1 < n and text[pos + 1] == "/":
        end = text.find("\n", pos)
        result = n if end == -1 else end
    elif ch == "/" and pos + 1 < n and text[pos + 1] == "*":
        result = _skip_rust_block_comment(text, pos)
    elif ch == '"':
        result = _skip_quoted(text, pos, '"')
    elif ch in ("r", "b", "c"):
        raw_end = _skip_rust_raw_string(text, pos)
        if raw_end is not None:
            result = raw_end
        elif ch in ("b", "c") and pos + 1 < n and text[pos + 1] == '"':
            result = _skip_quoted(text, pos + 1, '"')
    elif ch == "'":
        char_end = _skip_rust_char_literal(text, pos)
        result = char_end if char_end is not None else pos + 1
    return result


def _count_rust_allows(text: str) -> int:
    count = 0
    n = len(text)
    i = 0
    while i < n:
        skipped = _skip_rust_non_attr_token(text, i)
        if skipped is not None:
            i = skipped
            continue
        if text[i] == "#":
            bracket_pos = _match_rust_attr_bracket(text, i)
            if bracket_pos is not None:
                attr_text, i = _read_rust_attr(text, i, bracket_pos)
                if RUST_ALLOW_OR_EXPECT_RE.match(attr_text):
                    count += 1
                continue
        i += 1
    return count


def _skip_ts_template(text: str, pos: int) -> int:
    n = len(text)
    i = pos + 1
    while i < n:
        ch = text[i]
        if ch == "\\" and i + 1 < n:
            i += 2
            continue
        if ch == "`":
            return i + 1
        i += 1
    return i


def _scan_ts_comments(text: str, *, is_svelte: bool) -> list[str]:
    comments: list[str] = []
    n = len(text)
    i = 0
    while i < n:
        ch = text[i]
        if ch in ('"', "'"):
            i = _skip_quoted(text, i, ch)
        elif ch == "`":
            i = _skip_ts_template(text, i)
        elif ch == "/" and i + 1 < n and text[i + 1] == "/":
            end = text.find("\n", i)
            end = n if end == -1 else end
            comments.append(text[i:end])
            i = end
        elif ch == "/" and i + 1 < n and text[i + 1] == "*":
            end = text.find("*/", i + 2)
            end = n if end == -1 else end + 2
            comments.append(text[i:end])
            i = end
        elif is_svelte and text[i : i + 4] == "<!--":
            end = text.find("-->", i + 4)
            end = n if end == -1 else end + 3
            comments.append(text[i:end])
            i = end
        else:
            i += 1
    return comments


def _count_python_suppressions(text: str) -> tuple[int, int]:
    try:
        comments = [
            tok.string
            for tok in tokenize.generate_tokens(io.StringIO(text).readline)
            if tok.type == tokenize.COMMENT
        ]
    except (tokenize.TokenError, IndentationError):
        comments = [line.lstrip() for line in text.splitlines() if line.lstrip().startswith("#")]
    noqa_n = sum(
        1 for raw in comments if not PY_FILE_NOQA_RE.match(raw) and PY_INLINE_NOQA_RE.search(raw)
    )
    nosec_n = sum(1 for raw in comments if PY_NOSEC_RE.search(raw))
    return noqa_n, nosec_n


def count_file_suppressions(rel_path: str, text: str) -> dict[str, int]:
    """Count inline suppressions by kind in a single source file."""
    if is_excluded_source_path(rel_path) or is_generated_source_text(text):
        return {}
    counts: dict[str, int] = {}
    if rel_path.endswith(".py"):
        noqa_n, nosec_n = _count_python_suppressions(text)
        if noqa_n:
            counts["python_noqa"] = noqa_n
        if nosec_n:
            counts["python_nosec"] = nosec_n
    elif rel_path.endswith(".go"):
        go_comments = _scan_go_comments(text)
        nolint_n = sum(1 for raw in go_comments if GO_NOLINT_RE.match(raw))
        nosec_n = sum(1 for raw in go_comments if GO_NOSEC_RE.match(raw))
        if nolint_n:
            counts["go_nolint"] = nolint_n
        if nosec_n:
            counts["go_nosec"] = nosec_n
    elif rel_path.endswith(".rs"):
        n = _count_rust_allows(text)
        if n:
            counts["rust_allow"] = n
    elif rel_path.endswith(TS_EXTENSIONS):
        ts_comments = _scan_ts_comments(text, is_svelte=rel_path.endswith(".svelte"))
        biome_n = sum(1 for raw in ts_comments if TS_BIOME_LINE_RE.search(raw))
        deno_n = sum(1 for raw in ts_comments if TS_DENO_LINE_RE.search(raw))
        if biome_n:
            counts["ts_biome_ignore"] = biome_n
        if deno_n:
            counts["ts_deno_ignore"] = deno_n
    return counts


def _git_ls_files(repo_root: pathlib.Path) -> list[str]:
    git_bin = shutil.which("git")
    if git_bin is None:
        raise FileNotFoundError("git")
    proc = subprocess.run(  # noqa: S603 - fixed git ls-files invocation
        [git_bin, "-C", str(repo_root), "ls-files", "--cached", "--others", "--exclude-standard"],
        capture_output=True,
        text=True,
        check=True,
    )
    return sorted({line for line in proc.stdout.splitlines() if line})


def discover_directory_targets(repo_root: pathlib.Path, tracked_files: list[str]) -> list[str]:
    """Return directory lint targets from scripts/lint/run.sh --list or marker fallback."""
    run_sh = repo_root / "scripts" / "lint" / "run.sh"
    bash_bin = shutil.which("bash")
    if run_sh.is_file() and bash_bin is not None:
        proc = subprocess.run(  # noqa: S603 - fixed scripts/lint/run.sh --list invocation
            [bash_bin, str(run_sh), "--list"],
            capture_output=True,
            text=True,
            check=True,
        )
        return sorted(
            {
                t.strip().rstrip("/")
                for t in proc.stdout.splitlines()
                if t.strip() and (repo_root / t.strip()).is_dir()
            }
        )

    targets: set[str] = set()
    for rel_path in tracked_files:
        if rel_path.startswith(("stopped-using-k8s/", "scripts/harness/corpus/")):
            continue
        p = pathlib.PurePosixPath(rel_path)
        if p.name in MARKER_FILENAMES and str(p.parent) != ".":
            targets.add(str(p.parent))
    return sorted(targets)


def resolve_target(rel_path: str, directory_targets: list[str]) -> str:
    """Map a repo-relative path to its longest matching directory target, or 'repo-root'."""
    norm = rel_path.lstrip("./")
    best = ""
    for target in directory_targets:
        prefix = f"{target}/"
        if norm.startswith(prefix) and len(target) > len(best):
            best = target
    return best or "repo-root"


def _is_safe_repo_file(full_path: pathlib.Path, repo_root: pathlib.Path) -> bool:
    if full_path.is_symlink():
        return False
    try:
        resolved = full_path.resolve()
        if not resolved.is_relative_to(repo_root.resolve()):
            return False
    except OSError:
        return False
    return full_path.is_file()


def collect_repo_counts(repo_root: pathlib.Path) -> dict[str, dict[str, int]]:
    """Scan tracked source files and aggregate inline suppression counts per (target, kind)."""
    tracked_files = _git_ls_files(repo_root)
    dir_targets = discover_directory_targets(repo_root, tracked_files)
    totals: dict[str, Counter[str]] = {}

    for rel_path in tracked_files:
        if is_excluded_source_path(rel_path):
            continue
        full_path = repo_root / rel_path
        if not _is_safe_repo_file(full_path, repo_root):
            continue
        text = full_path.read_text(encoding="utf-8", errors="replace")
        file_counts = count_file_suppressions(rel_path, text)
        if not file_counts:
            continue
        target = resolve_target(rel_path, dir_targets)
        bucket = totals.setdefault(target, Counter())
        for kind, count in file_counts.items():
            bucket[kind] += count

    return {
        target: {kind: bucket[kind] for kind in sorted(bucket) if bucket[kind] > 0}
        for target, bucket in sorted(totals.items())
        if sum(bucket.values()) > 0
    }


def load_suppressions_toml(
    path: pathlib.Path,
) -> tuple[dict[str, dict[str, int]], dict[str, object]]:
    """Load recorded per-(target, kind) counts and optional [governance] table."""
    data = tomllib.loads(path.read_text(encoding="utf-8"))
    raw_targets = data.get("targets", {})
    recorded: dict[str, dict[str, int]] = {}
    if isinstance(raw_targets, dict):
        for target, kinds_tbl in raw_targets.items():
            if not isinstance(kinds_tbl, dict):
                continue
            parsed = {
                str(kind): int(val)
                for kind, val in kinds_tbl.items()
                if isinstance(val, int) and val > 0
            }
            if parsed:
                recorded[str(target)] = parsed
    raw_gov = data.get("governance", {})
    governance = dict(raw_gov) if isinstance(raw_gov, dict) else {}
    return recorded, governance


def _toml_string(value: str) -> str:
    """Escape a string as a valid TOML basic string using RFC 8259 JSON escaping."""
    return json.dumps(value, ensure_ascii=False)


def _toml_key_quote(key: str) -> str:
    if re.match(r"^[A-Za-z0-9_-]+$", key):
        return key
    return _toml_string(key)


def render_suppressions_toml(
    counts: dict[str, dict[str, int]],
    governance: dict[str, object] | None = None,
) -> str:
    """Render deterministic TOML for scripts/lint/ratchet/suppressions.toml."""
    lines = [
        "# Inline suppression count ratchet baseline (CHECK-02).",
        "# Auto-generated by `python3 scripts/lint/check_ratchet.py --update`.",
        "# Counts may only decrease; increasing any (target, kind) count fails CI.",
        "",
        "version = 1",
        "",
    ]
    if governance:
        lines.append("[governance]")
        for key in sorted(governance):
            val = governance[key]
            if isinstance(val, bool):
                lines.append(f"{_toml_key_quote(key)} = {'true' if val else 'false'}")
            elif isinstance(val, int):
                lines.append(f"{_toml_key_quote(key)} = {val}")
            elif isinstance(val, str):
                lines.append(f"{_toml_key_quote(key)} = {_toml_string(val)}")
        lines.append("")

    for target in sorted(counts):
        kinds = counts[target]
        if not kinds:
            continue
        lines.append(f"[targets.{_toml_key_quote(target)}]")
        lines.extend(f"{kind} = {kinds[kind]}" for kind in sorted(kinds) if kinds[kind] > 0)
        lines.append("")
    return "\n".join(lines)


def compare_ratchet_counts(
    actual: dict[str, dict[str, int]],
    recorded: dict[str, dict[str, int]],
) -> tuple[list[tuple[str, str, int, int]], list[tuple[str, str, int, int]]]:
    """Return (increases, decreases) as lists of (target, kind, recorded_n, actual_n)."""
    all_targets = sorted(set(actual) | set(recorded))
    increases: list[tuple[str, str, int, int]] = []
    decreases: list[tuple[str, str, int, int]] = []
    for target in all_targets:
        act_kinds = actual.get(target, {})
        rec_kinds = recorded.get(target, {})
        for kind in sorted(set(act_kinds) | set(rec_kinds)):
            act_n = act_kinds.get(kind, 0)
            rec_n = rec_kinds.get(kind, 0)
            if act_n > rec_n:
                increases.append((target, kind, rec_n, act_n))
            elif act_n < rec_n:
                decreases.append((target, kind, rec_n, act_n))
    return increases, decreases


def _validate_contained_path(full: pathlib.Path, repo_root: pathlib.Path, raw_label: str) -> str:
    """Fail closed if path is a symlink or escapes repo_root; return repo-relative POSIX path."""
    repo_resolved = repo_root.resolve()
    if full.is_symlink():
        msg = f"finding path must not be a symlink: {raw_label!r}"
        raise ValueError(msg)
    resolved = full.resolve()
    if not resolved.is_relative_to(repo_resolved):
        msg = f"finding path escapes repository root: {raw_label!r}"
        raise ValueError(msg)
    # Also reject symlinks in any intermediate component under repo_root.
    rel = resolved.relative_to(repo_resolved)
    cursor = repo_resolved
    for part in rel.parts:
        cursor = cursor / part
        if cursor.is_symlink():
            msg = f"finding path traverses a symlink: {raw_label!r}"
            raise ValueError(msg)
    return rel.as_posix()


def _resolve_finding_rel_path(
    raw_file: str,
    repo_root: pathlib.Path,
    project_dir: pathlib.Path | None = None,
) -> str:
    if not raw_file or "\x00" in raw_file:
        msg = f"invalid empty or NUL finding path: {raw_file!r}"
        raise ValueError(msg)
    candidate = raw_file
    if candidate.startswith("file://"):
        parsed = urlparse(candidate)
        if parsed.netloc and parsed.netloc != "localhost":
            msg = f"unsupported non-local file URI in finding path: {raw_file!r}"
            raise ValueError(msg)
        candidate = unquote(parsed.path)
    normalized = candidate.replace("\\", "/")
    p = pathlib.Path(normalized)
    if ".." in p.parts:
        msg = f"finding path must not contain '..': {raw_file!r}"
        raise ValueError(msg)
    if p.is_absolute():
        return _validate_contained_path(p, repo_root, raw_file)
    if normalized.startswith("/"):
        msg = f"finding path must not start with '/': {raw_file!r}"
        raise ValueError(msg)
    base_dir = project_dir if project_dir is not None else repo_root
    full = base_dir / p
    return _validate_contained_path(full, repo_root, raw_file)


def _extract_source_line(
    repo_root: pathlib.Path,
    rel_file: str,
    line_no: int,
    override_line: str | None,
    cache: dict[str, list[str]],
) -> str:
    if override_line is not None:
        return override_line
    if not rel_file or "\x00" in rel_file or rel_file.startswith("/"):
        msg = f"invalid relative finding path: {rel_file!r}"
        raise ValueError(msg)
    p = pathlib.PurePosixPath(rel_file)
    if p.is_absolute() or ".." in p.parts:
        msg = f"invalid relative finding path: {rel_file!r}"
        raise ValueError(msg)
    full = repo_root / rel_file
    _validate_contained_path(full, repo_root, rel_file)
    if rel_file not in cache:
        cache[rel_file] = (
            full.read_text(encoding="utf-8", errors="replace").splitlines()
            if full.is_file()
            else []
        )
    lines = cache[rel_file]
    if 1 <= line_no <= len(lines):
        return lines[line_no - 1]
    return ""


@dataclass
class _FindingContext:
    repo_root: pathlib.Path
    project_dir: pathlib.Path | None
    line_cache: dict[str, list[str]]


@dataclass(frozen=True)
class _FingerprintRunOpts:
    repo_root: pathlib.Path
    input_kind: str
    input_path: pathlib.Path
    baselines_path: pathlib.Path
    project_dir: pathlib.Path | None = None
    update_fingerprints: bool = False
    require_findings: bool = False


def _build_finding_item(
    ctx: _FindingContext,
    rule: str,
    file_raw: str,
    line_val: int,
    override_line: str | None = None,
) -> FindingItem:
    rel_file = _resolve_finding_rel_path(file_raw, ctx.repo_root, project_dir=ctx.project_dir)
    src_line = _extract_source_line(
        ctx.repo_root, rel_file, line_val, override_line, ctx.line_cache
    )
    return FindingItem(
        rule=rule.strip(),
        rel_file=rel_file,
        line=line_val,
        line_sha256=line_fingerprint_sha256(src_line),
    )


def load_findings_json(
    findings_path: pathlib.Path,
    repo_root: pathlib.Path,
    *,
    project_dir: pathlib.Path | None = None,
) -> list[FindingItem]:
    """Load linter findings from Ruff JSON or generic `{rule, file, line}` JSON."""
    raw = json.loads(findings_path.read_text(encoding="utf-8"))
    if isinstance(raw, dict) and isinstance(raw.get("findings"), list):
        items_raw = raw["findings"]
    elif isinstance(raw, list):
        items_raw = raw
    else:
        msg = "findings JSON must be a list or an object containing a 'findings' list"
        raise TypeError(msg)

    ctx = _FindingContext(repo_root=repo_root, project_dir=project_dir, line_cache={})
    findings: list[FindingItem] = []
    for entry in items_raw:
        if not isinstance(entry, dict):
            continue
        rule = entry.get("rule") or entry.get("code")
        if rule is None and "filename" in entry and "location" in entry:
            rule = "syntax-error"
        file_raw = entry.get("file") or entry.get("filename") or entry.get("path")
        line_val = entry.get("line")
        if line_val is None and isinstance(entry.get("location"), dict):
            line_val = entry["location"].get("row")
        if (
            not isinstance(rule, str)
            or not isinstance(file_raw, str)
            or not isinstance(line_val, int)
        ):
            msg = f"invalid finding entry (expected rule/code, file/filename, line/location.row): {entry!r}"
            raise TypeError(msg)
        override_line = entry.get("source_line")
        findings.append(
            _build_finding_item(
                ctx,
                rule,
                file_raw,
                line_val,
                override_line if isinstance(override_line, str) else None,
            )
        )
    return findings


def load_golangci_sarif(
    sarif_path: pathlib.Path,
    repo_root: pathlib.Path,
    *,
    project_dir: pathlib.Path | None = None,
) -> list[FindingItem]:
    """Load findings from golangci-lint / gosec SARIF 2.1.0 output."""
    raw = json.loads(sarif_path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict) or not isinstance(raw.get("runs"), list):
        msg = "SARIF payload must be an object with a 'runs' list"
        raise TypeError(msg)

    ctx = _FindingContext(repo_root=repo_root, project_dir=project_dir, line_cache={})
    findings: list[FindingItem] = []
    for run in raw["runs"]:
        if not isinstance(run, dict):
            continue
        for result in run.get("results") or []:
            if not isinstance(result, dict):
                continue
            rule_id = result.get("ruleId")
            if not isinstance(rule_id, str) or not rule_id.strip():
                msg = f"SARIF result missing ruleId: {result!r}"
                raise TypeError(msg)
            rule = rule_id.strip()
            if rule == "gosec":
                msg_text = (
                    result.get("message", {}).get("text", "")
                    if isinstance(result.get("message"), dict)
                    else ""
                )
                gosec_match = GOSEC_MSG_RULE_RE.match(str(msg_text).strip())
                if gosec_match:
                    rule = gosec_match.group(1)
            locations = result.get("locations") or []
            if not isinstance(locations, list) or not locations:
                msg = f"SARIF result missing locations: {result!r}"
                raise TypeError(msg)
            phys = locations[0].get("physicalLocation") if isinstance(locations[0], dict) else None
            art = phys.get("artifactLocation") if isinstance(phys, dict) else None
            reg = phys.get("region") if isinstance(phys, dict) else None
            uri = art.get("uri") if isinstance(art, dict) else None
            start_line = reg.get("startLine") if isinstance(reg, dict) else None
            if not isinstance(uri, str) or not isinstance(start_line, int):
                msg = f"SARIF location missing artifactLocation.uri or region.startLine: {result!r}"
                raise TypeError(msg)
            findings.append(_build_finding_item(ctx, rule, uri, start_line))
    return findings


def load_clippy_json(
    clippy_path: pathlib.Path,
    repo_root: pathlib.Path,
    *,
    project_dir: pathlib.Path | None = None,
) -> list[FindingItem]:
    """Load findings from `cargo clippy --message-format=json` NDJSON output."""
    ctx = _FindingContext(repo_root=repo_root, project_dir=project_dir, line_cache={})
    findings: list[FindingItem] = []
    for raw_line in clippy_path.read_text(encoding="utf-8").splitlines():
        stripped = raw_line.strip()
        if not stripped or not stripped.startswith("{"):
            continue
        obj = json.loads(stripped)
        if not isinstance(obj, dict) or obj.get("reason") != "compiler-message":
            continue
        msg_obj = obj.get("message")
        if not isinstance(msg_obj, dict):
            continue
        level = msg_obj.get("level")
        if level not in ("error", "warning"):
            continue
        spans = msg_obj.get("spans") or []
        code_obj = msg_obj.get("code")
        code_val = code_obj.get("code") if isinstance(code_obj, dict) else None
        if not code_val and not spans:
            continue
        rule = str(code_val) if isinstance(code_val, str) and code_val.strip() else "rustc-error"
        primary = next(
            (s for s in spans if isinstance(s, dict) and s.get("is_primary")),
            spans[0] if spans and isinstance(spans[0], dict) else None,
        )
        if not isinstance(primary, dict):
            msg = f"clippy compiler-message missing span: {msg_obj!r}"
            raise TypeError(msg)
        file_name = primary.get("file_name")
        line_start = primary.get("line_start")
        if not isinstance(file_name, str) or not isinstance(line_start, int):
            msg = f"clippy span missing file_name or line_start: {primary!r}"
            raise TypeError(msg)
        findings.append(_build_finding_item(ctx, rule, file_name, line_start))
    return findings


def load_deno_json(
    deno_path: pathlib.Path,
    repo_root: pathlib.Path,
    *,
    project_dir: pathlib.Path | None = None,
) -> list[FindingItem]:
    """Load findings from `deno lint --json` output."""
    raw = json.loads(deno_path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict):
        msg = "deno lint JSON must be an object"
        raise TypeError(msg)
    parse_errors = raw.get("errors") or []
    if isinstance(parse_errors, list) and parse_errors:
        msg = f"deno lint reported {len(parse_errors)} parse/config error(s): {parse_errors[0]!r}"
        raise ValueError(msg)
    diagnostics = raw.get("diagnostics") or []
    if not isinstance(diagnostics, list):
        msg = "deno lint JSON 'diagnostics' must be a list"
        raise TypeError(msg)

    ctx = _FindingContext(repo_root=repo_root, project_dir=project_dir, line_cache={})
    findings: list[FindingItem] = []
    for diag in diagnostics:
        if not isinstance(diag, dict):
            continue
        code = diag.get("code")
        filename = diag.get("filename")
        rng = diag.get("range")
        start = rng.get("start") if isinstance(rng, dict) else None
        line_no = start.get("line") if isinstance(start, dict) else None
        if (
            not isinstance(code, str)
            or not isinstance(filename, str)
            or not isinstance(line_no, int)
        ):
            msg = f"invalid deno lint diagnostic: {diag!r}"
            raise TypeError(msg)
        findings.append(_build_finding_item(ctx, code, filename, line_no))
    return findings


def _biome_line_from_span(
    repo_root: pathlib.Path,
    rel_file: str,
    location: dict[str, object],
) -> int:
    start = location.get("start")
    if isinstance(start, dict) and isinstance(start.get("line"), int):
        return int(start["line"])
    span = location.get("span")
    if isinstance(span, list) and span and isinstance(span[0], int):
        offset = max(0, int(span[0]))
        src_code = location.get("sourceCode")
        if isinstance(src_code, str):
            return src_code.encode("utf-8")[:offset].count(b"\n") + 1
        full = repo_root / rel_file
        _validate_contained_path(full, repo_root, rel_file)
        if full.is_file():
            return full.read_bytes()[:offset].count(b"\n") + 1
    return 1


def load_biome_json(
    biome_path: pathlib.Path,
    repo_root: pathlib.Path,
    *,
    project_dir: pathlib.Path | None = None,
) -> list[FindingItem]:
    """Load findings from `biome lint --reporter=json` output."""
    raw = json.loads(biome_path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict) or not isinstance(raw.get("diagnostics"), list):
        msg = "biome JSON must be an object with a 'diagnostics' list"
        raise TypeError(msg)

    line_cache: dict[str, list[str]] = {}
    findings: list[FindingItem] = []
    for diag in raw["diagnostics"]:
        if not isinstance(diag, dict):
            continue
        category = diag.get("category")
        loc = diag.get("location")
        if not isinstance(category, str) or not isinstance(loc, dict):
            continue
        path_field = loc.get("path")
        if isinstance(path_field, dict):
            file_raw = path_field.get("file")
        elif isinstance(path_field, str):
            file_raw = path_field
        else:
            file_raw = None
        if not isinstance(file_raw, str) or not file_raw.strip():
            msg = f"biome diagnostic missing location.path.file: {diag!r}"
            raise TypeError(msg)
        rel_file = _resolve_finding_rel_path(file_raw, repo_root, project_dir=project_dir)
        line_no = _biome_line_from_span(repo_root, rel_file, loc)
        src_line = _extract_source_line(repo_root, rel_file, line_no, None, line_cache)
        findings.append(
            FindingItem(
                rule=category.strip(),
                rel_file=rel_file,
                line=line_no,
                line_sha256=line_fingerprint_sha256(src_line),
            )
        )
    return findings


def load_rule_baselines(path: pathlib.Path) -> Counter[FingerprintKey]:
    """Load block-new rule fingerprint baseline counts from rule_baselines.toml."""
    if not path.is_file():
        return Counter()
    data = tomllib.loads(path.read_text(encoding="utf-8"))
    counts: Counter[FingerprintKey] = Counter()
    for entry in data.get("fingerprints", []):
        if not isinstance(entry, dict):
            continue
        rule = entry.get("rule")
        rel_file = entry.get("file")
        line_sha = entry.get("line_sha256")
        count = entry.get("count", 1)
        if (
            isinstance(rule, str)
            and isinstance(rel_file, str)
            and isinstance(line_sha, str)
            and isinstance(count, int)
            and count > 0
        ):
            counts[(rule, rel_file, line_sha)] += count
    return counts


def render_rule_baselines_toml(counts: Counter[FingerprintKey]) -> str:
    """Serialize block-new rule fingerprint baseline ledger to deterministic TOML."""
    lines = [
        "# block-new rule violation fingerprint baseline ledger (CHECK-02).",
        "# Managed by `python3 scripts/lint/check_ratchet.py --findings-json <file> --update-fingerprints`.",
        "# Existing fingerprinted violations pass; new fingerprints fail. Never use `ruff --add-noqa`.",
        "",
        "version = 1",
        "",
    ]
    if not counts:
        lines.append("fingerprints = []")
        lines.append("")
        return "\n".join(lines)
    for (rule, rel_file, line_sha), count in sorted(counts.items()):
        if count <= 0:
            continue
        lines.extend(
            [
                "[[fingerprints]]",
                f"rule = {_toml_string(rule)}",
                f"file = {_toml_string(rel_file)}",
                f"line_sha256 = {_toml_string(line_sha)}",
                f"count = {count}",
                "",
            ]
        )
    return "\n".join(lines)


def _load_findings_for_mode(opts: _FingerprintRunOpts) -> list[FindingItem] | None:
    if not opts.input_path.is_file():
        sys.stderr.write(f"ERROR: findings report not found: {opts.input_path}\n")
        return None
    loaders = {
        "generic": load_findings_json,
        "ruff": load_findings_json,
        "golangci-sarif": load_golangci_sarif,
        "clippy-json": load_clippy_json,
        "deno-json": load_deno_json,
        "biome-json": load_biome_json,
    }
    try:
        findings = loaders[opts.input_kind](
            opts.input_path, opts.repo_root, project_dir=opts.project_dir
        )
    except (ValueError, TypeError, json.JSONDecodeError, OSError) as exc:
        sys.stderr.write(f"ERROR: failed to load {opts.input_kind} findings: {exc}\n")
        return None
    if opts.require_findings and not findings:
        sys.stderr.write(
            f"ERROR: {opts.input_kind} report contained 0 parsed findings while linter exited non-zero "
            f"(fail-closed: refusing to treat unparsed linter failure as clean)\n"
        )
        return None
    return findings


def _run_fingerprint_mode(opts: _FingerprintRunOpts) -> int:
    findings = _load_findings_for_mode(opts)
    if findings is None:
        return EXIT_USAGE_ERROR

    actual_counts: Counter[FingerprintKey] = Counter(
        (f.rule, f.rel_file, f.line_sha256) for f in findings
    )
    if opts.update_fingerprints:
        opts.baselines_path.parent.mkdir(parents=True, exist_ok=True)
        opts.baselines_path.write_text(render_rule_baselines_toml(actual_counts), encoding="utf-8")
        sys.stdout.write(
            f"Updated {opts.baselines_path} ({len(actual_counts)} unique fingerprints, "
            f"{sum(actual_counts.values())} total findings).\n"
        )
        return EXIT_CLEAN

    try:
        recorded_counts = load_rule_baselines(opts.baselines_path)
    except (tomllib.TOMLDecodeError, OSError) as exc:
        sys.stderr.write(f"ERROR: failed to load baselines TOML: {exc}\n")
        return EXIT_USAGE_ERROR

    seen: Counter[FingerprintKey] = Counter()
    new_findings: list[FindingItem] = []
    for item in findings:
        key: FingerprintKey = (item.rule, item.rel_file, item.line_sha256)
        seen[key] += 1
        if seen[key] > recorded_counts.get(key, 0):
            new_findings.append(item)

    if new_findings:
        for item in new_findings:
            sys.stderr.write(
                f"{item.rel_file}:{item.line}: [ratchet.new-fingerprint] "
                f"new {item.rule} violation (sha256={item.line_sha256[:12]})\n"
            )
        sys.stderr.write(
            f"\nFAIL: {len(new_findings)} new block-new violation(s) not in {opts.baselines_path}.\n"
        )
        return EXIT_VIOLATIONS

    removed_total = sum(
        max(0, rec_n - actual_counts.get(key, 0)) for key, rec_n in recorded_counts.items()
    )
    if removed_total > 0:
        sys.stdout.write(
            f"OK: 0 new block-new violations ({removed_total} baselined violation(s) resolved; "
            f"run --update-fingerprints to lower the baseline).\n"
        )
    else:
        sys.stdout.write(
            f"OK: 0 new block-new violations ({len(findings)} baselined finding(s) matched).\n"
        )
    return EXIT_CLEAN


def _write_updated_suppressions(
    actual: dict[str, dict[str, int]],
    suppressions_path: pathlib.Path,
    max_judgment_rules: int | None,
) -> int:
    governance: dict[str, object] = {}
    if suppressions_path.is_file():
        try:
            _, governance = load_suppressions_toml(suppressions_path)
        except (tomllib.TOMLDecodeError, OSError) as exc:
            sys.stderr.write(f"ERROR: failed to parse existing {suppressions_path}: {exc}\n")
            return EXIT_USAGE_ERROR
    if max_judgment_rules is not None:
        governance["max_judgment_rules"] = max_judgment_rules
    suppressions_path.parent.mkdir(parents=True, exist_ok=True)
    suppressions_path.write_text(
        render_suppressions_toml(actual, governance),
        encoding="utf-8",
    )
    grand_total = sum(sum(k.values()) for k in actual.values())
    sys.stdout.write(
        f"Updated {suppressions_path} ({len(actual)} targets, {grand_total} total inline suppressions).\n"
    )
    return EXIT_CLEAN


def _run_suppression_ratchet_mode(
    repo_root: pathlib.Path,
    suppressions_path: pathlib.Path,
    *,
    update: bool,
    max_judgment_rules: int | None,
) -> int:
    try:
        actual = collect_repo_counts(repo_root)
    except (subprocess.CalledProcessError, FileNotFoundError, OSError) as exc:
        sys.stderr.write(f"ERROR: failed to collect suppression counts: {exc}\n")
        return EXIT_USAGE_ERROR

    if update:
        return _write_updated_suppressions(actual, suppressions_path, max_judgment_rules)

    if not suppressions_path.is_file():
        sys.stderr.write(
            f"ERROR: ratchet baseline not found: {suppressions_path} "
            f"(run `python3 scripts/lint/check_ratchet.py --update` to create it)\n"
        )
        return EXIT_USAGE_ERROR

    try:
        recorded, _ = load_suppressions_toml(suppressions_path)
    except (tomllib.TOMLDecodeError, OSError) as exc:
        sys.stderr.write(f"ERROR: failed to parse {suppressions_path}: {exc}\n")
        return EXIT_USAGE_ERROR

    increases, decreases = compare_ratchet_counts(actual, recorded)
    for target, kind, rec_n, act_n in increases:
        sys.stderr.write(
            f"{target}: [ratchet.increase] {kind} increased from {rec_n} to {act_n} (+{act_n - rec_n})\n"
        )
    for target, kind, rec_n, act_n in decreases:
        sys.stderr.write(
            f"{target}: [ratchet.decrease-update-required] {kind} decreased from {rec_n} to {act_n} "
            f"(-{rec_n - act_n}); run `python3 scripts/lint/check_ratchet.py --update` in the same commit\n"
        )
    if increases or decreases:
        sys.stderr.write(
            f"\nFAIL: suppression ratchet mismatch ({len(increases)} increase(s), "
            f"{len(decreases)} decrease(s)).\n"
        )
        return EXIT_VIOLATIONS

    grand_total = sum(sum(k.values()) for k in actual.values())
    sys.stdout.write(
        f"OK: suppression ratchet clean ({len(actual)} targets, {grand_total} total inline suppressions).\n"
    )
    return EXIT_CLEAN


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    raw_argv = list(sys.argv[1:] if argv is None else argv)
    submode = "ratchet"
    if raw_argv and raw_argv[0] in ("check-fingerprints", "ratchet"):
        submode = raw_argv.pop(0)

    default_root = pathlib.Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(
        description="Enforce inline suppression ratchet and block-new rule fingerprint baselines.",
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=default_root,
        help="Repository root directory (default: detected repo root).",
    )
    parser.add_argument(
        "--suppressions-file",
        type=pathlib.Path,
        default=None,
        help="Path to suppressions.toml (default: <repo-root>/scripts/lint/ratchet/suppressions.toml).",
    )
    parser.add_argument(
        "--update",
        action="store_true",
        default=False,
        help="Regenerate suppressions.toml from current repository counts.",
    )
    parser.add_argument(
        "--max-judgment-rules",
        type=int,
        default=None,
        help="Set [governance].max_judgment_rules when running --update.",
    )
    parser.add_argument(
        "--findings-json",
        type=pathlib.Path,
        default=None,
        help="Path to generic/Ruff findings JSON for block-new fingerprint checking.",
    )
    parser.add_argument(
        "--ruff-json",
        type=pathlib.Path,
        default=None,
        help="Path to `ruff check --output-format json` report.",
    )
    parser.add_argument(
        "--golangci-sarif",
        type=pathlib.Path,
        default=None,
        help="Path to `golangci-lint` / `gosec` SARIF 2.1.0 report.",
    )
    parser.add_argument(
        "--clippy-json",
        type=pathlib.Path,
        default=None,
        help="Path to `cargo clippy --message-format=json` NDJSON report.",
    )
    parser.add_argument(
        "--deno-json",
        type=pathlib.Path,
        default=None,
        help="Path to `deno lint --json` report.",
    )
    parser.add_argument(
        "--biome-json",
        type=pathlib.Path,
        default=None,
        help="Path to `biome lint --reporter=json` report.",
    )
    parser.add_argument(
        "--project-dir",
        type=pathlib.Path,
        default=None,
        help="Optional project directory for resolving project-relative finding paths.",
    )
    parser.add_argument(
        "--require-findings",
        action="store_true",
        default=False,
        help="Fail closed (exit 2) if 0 findings were parsed from the report.",
    )
    parser.add_argument(
        "--baselines-file",
        type=pathlib.Path,
        default=None,
        help="Path to rule_baselines.toml (default: <repo-root>/scripts/lint/ratchet/rule_baselines.toml).",
    )
    parser.add_argument(
        "--update-fingerprints",
        action="store_true",
        default=False,
        help="Update rule_baselines.toml from findings report (lowers baseline when violations are removed).",
    )
    args = parser.parse_args(raw_argv)
    args.submode = submode
    return args


def _select_fingerprint_input(args: argparse.Namespace) -> tuple[str, pathlib.Path] | None:
    candidates: list[tuple[str, pathlib.Path]] = []
    for kind, path in (
        ("generic", args.findings_json),
        ("ruff", args.ruff_json),
        ("golangci-sarif", args.golangci_sarif),
        ("clippy-json", args.clippy_json),
        ("deno-json", args.deno_json),
        ("biome-json", args.biome_json),
    ):
        if path is not None:
            candidates.append((kind, path))
    if len(candidates) > 1:
        msg = "specify at most one findings input flag"
        raise ValueError(msg)
    return candidates[0] if candidates else None


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    repo_root: pathlib.Path = args.repo_root.resolve()

    try:
        fp_input = _select_fingerprint_input(args)
    except ValueError as exc:
        sys.stderr.write(f"ERROR: {exc}\n")
        return EXIT_USAGE_ERROR

    if args.submode == "check-fingerprints" or fp_input is not None or args.update_fingerprints:
        if fp_input is None:
            sys.stderr.write(
                "ERROR: one of --findings-json, --ruff-json, --golangci-sarif, "
                "--clippy-json, --deno-json, or --biome-json is required for fingerprint mode\n"
            )
            return EXIT_USAGE_ERROR
        baselines_path = (
            args.baselines_file.resolve()
            if args.baselines_file is not None
            else repo_root / "scripts" / "lint" / "ratchet" / "rule_baselines.toml"
        )
        project_dir = args.project_dir.resolve() if args.project_dir is not None else None
        return _run_fingerprint_mode(
            _FingerprintRunOpts(
                repo_root=repo_root,
                input_kind=fp_input[0],
                input_path=fp_input[1].resolve(),
                baselines_path=baselines_path,
                project_dir=project_dir,
                update_fingerprints=args.update_fingerprints,
                require_findings=args.require_findings,
            )
        )

    suppressions_path = (
        args.suppressions_file.resolve()
        if args.suppressions_file is not None
        else repo_root / "scripts" / "lint" / "ratchet" / "suppressions.toml"
    )
    return _run_suppression_ratchet_mode(
        repo_root,
        suppressions_path,
        update=args.update,
        max_judgment_rules=args.max_judgment_rules,
    )


if __name__ == "__main__":
    sys.exit(main())
