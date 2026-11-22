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
GO_NOLINT_RE = re.compile(r"^(?://|/\*)\s*nolint(?::|(?=$|[\s*/]))", re.DOTALL)
GO_NOSEC_RE = re.compile(r"^(?://|/\*)\s*#nosec\b", re.DOTALL)
RUST_OUTER_ALLOW_RE = re.compile(r"^#\[\s*allow\s*\(", re.DOTALL)
TS_BIOME_LINE_RE = re.compile(r"\bbiome-ignore\b(?!-all)")
TS_DENO_LINE_RE = re.compile(r"\bdeno-lint-ignore\b(?!-file)")

SUPPRESSION_KINDS = (
    "go_nolint",
    "go_nosec",
    "python_noqa",
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


def _read_rust_outer_attr(text: str, pos: int) -> tuple[str, int]:
    n = len(text)
    i = pos + 1
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


def _count_rust_allows(text: str) -> int:
    count = 0
    n = len(text)
    i = 0
    while i < n:
        ch = text[i]
        if ch == "/" and i + 1 < n and text[i + 1] == "/":
            end = text.find("\n", i)
            i = n if end == -1 else end
        elif ch == "/" and i + 1 < n and text[i + 1] == "*":
            i = _skip_rust_block_comment(text, i)
        elif ch == '"':
            i = _skip_quoted(text, i, '"')
        elif ch in ("r", "b", "c"):
            raw_end = _skip_rust_raw_string(text, i)
            if raw_end is not None:
                i = raw_end
            elif ch in ("b", "c") and i + 1 < n and text[i + 1] == '"':
                i = _skip_quoted(text, i + 1, '"')
            else:
                i += 1
        elif ch == "'":
            char_end = _skip_rust_char_literal(text, i)
            i = char_end if char_end is not None else i + 1
        elif ch == "#" and i + 1 < n and text[i + 1] == "[":
            attr_text, i = _read_rust_outer_attr(text, i)
            if RUST_OUTER_ALLOW_RE.match(attr_text):
                count += 1
        else:
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


def _count_python_noqa(text: str) -> int:
    try:
        comments = [
            tok.string
            for tok in tokenize.generate_tokens(io.StringIO(text).readline)
            if tok.type == tokenize.COMMENT
        ]
    except (tokenize.TokenError, IndentationError):
        comments = [line.lstrip() for line in text.splitlines() if line.lstrip().startswith("#")]
    return sum(
        1 for raw in comments if not PY_FILE_NOQA_RE.match(raw) and PY_INLINE_NOQA_RE.search(raw)
    )


def count_file_suppressions(rel_path: str, text: str) -> dict[str, int]:
    """Count inline suppressions by kind in a single source file."""
    if is_excluded_source_path(rel_path) or is_generated_source_text(text):
        return {}
    counts: dict[str, int] = {}
    if rel_path.endswith(".py"):
        n = _count_python_noqa(text)
        if n:
            counts["python_noqa"] = n
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


def collect_repo_counts(repo_root: pathlib.Path) -> dict[str, dict[str, int]]:
    """Scan tracked source files and aggregate inline suppression counts per (target, kind)."""
    tracked_files = _git_ls_files(repo_root)
    dir_targets = discover_directory_targets(repo_root, tracked_files)
    totals: dict[str, Counter[str]] = {}

    for rel_path in tracked_files:
        if is_excluded_source_path(rel_path):
            continue
        full_path = repo_root / rel_path
        if not full_path.is_file():
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


def _toml_key_quote(key: str) -> str:
    if re.match(r"^[A-Za-z0-9_-]+$", key):
        return key
    escaped = key.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


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
                escaped = val.replace("\\", "\\\\").replace('"', '\\"')
                lines.append(f'{_toml_key_quote(key)} = "{escaped}"')
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


def _resolve_finding_rel_path(raw_file: str, repo_root: pathlib.Path) -> str:
    p = pathlib.Path(raw_file)
    if p.is_absolute():
        try:
            return p.resolve().relative_to(repo_root.resolve()).as_posix()
        except ValueError:
            return p.as_posix()
    return raw_file.lstrip("./").replace("\\", "/")


def _extract_source_line(
    repo_root: pathlib.Path,
    rel_file: str,
    line_no: int,
    override_line: str | None,
    cache: dict[str, list[str]],
) -> str:
    if override_line is not None:
        return override_line
    if rel_file not in cache:
        full = repo_root / rel_file
        cache[rel_file] = (
            full.read_text(encoding="utf-8", errors="replace").splitlines()
            if full.is_file()
            else []
        )
    lines = cache[rel_file]
    if 1 <= line_no <= len(lines):
        return lines[line_no - 1]
    return ""


def load_findings_json(findings_path: pathlib.Path, repo_root: pathlib.Path) -> list[FindingItem]:
    """Load linter findings from Ruff JSON or generic `{rule, file, line}` JSON."""
    raw = json.loads(findings_path.read_text(encoding="utf-8"))
    if isinstance(raw, dict) and isinstance(raw.get("findings"), list):
        items_raw = raw["findings"]
    elif isinstance(raw, list):
        items_raw = raw
    else:
        msg = "findings JSON must be a list or an object containing a 'findings' list"
        raise TypeError(msg)

    line_cache: dict[str, list[str]] = {}
    findings: list[FindingItem] = []
    for entry in items_raw:
        if not isinstance(entry, dict):
            continue
        rule = entry.get("rule") or entry.get("code")
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
        rel_file = _resolve_finding_rel_path(file_raw, repo_root)
        override_line = entry.get("source_line")
        src_line = _extract_source_line(
            repo_root,
            rel_file,
            line_val,
            override_line if isinstance(override_line, str) else None,
            line_cache,
        )
        findings.append(
            FindingItem(
                rule=rule.strip(),
                rel_file=rel_file,
                line=line_val,
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
                f'rule = "{rule}"',
                f'file = "{rel_file}"',
                f'line_sha256 = "{line_sha}"',
                f"count = {count}",
                "",
            ]
        )
    return "\n".join(lines)


def _run_fingerprint_mode(
    repo_root: pathlib.Path,
    findings_json: pathlib.Path,
    baselines_path: pathlib.Path,
    *,
    update_fingerprints: bool,
) -> int:
    if not findings_json.is_file():
        sys.stderr.write(f"ERROR: findings JSON not found: {findings_json}\n")
        return EXIT_USAGE_ERROR
    try:
        findings = load_findings_json(findings_json, repo_root)
    except (ValueError, TypeError, json.JSONDecodeError, OSError) as exc:
        sys.stderr.write(f"ERROR: failed to load findings JSON: {exc}\n")
        return EXIT_USAGE_ERROR

    actual_counts: Counter[FingerprintKey] = Counter(
        (f.rule, f.rel_file, f.line_sha256) for f in findings
    )
    if update_fingerprints:
        baselines_path.parent.mkdir(parents=True, exist_ok=True)
        baselines_path.write_text(render_rule_baselines_toml(actual_counts), encoding="utf-8")
        sys.stdout.write(
            f"Updated {baselines_path} ({len(actual_counts)} unique fingerprints, "
            f"{sum(actual_counts.values())} total findings).\n"
        )
        return EXIT_CLEAN

    try:
        recorded_counts = load_rule_baselines(baselines_path)
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
            f"\nFAIL: {len(new_findings)} new block-new violation(s) not in {baselines_path}.\n"
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
        help="Path to linter findings JSON for block-new fingerprint checking.",
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
        help="Update rule_baselines.toml from --findings-json (lowers baseline when violations are removed).",
    )
    args = parser.parse_args(raw_argv)
    args.submode = submode
    return args


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    repo_root: pathlib.Path = args.repo_root.resolve()

    if (
        args.submode == "check-fingerprints"
        or args.findings_json is not None
        or args.update_fingerprints
    ):
        if args.findings_json is None:
            sys.stderr.write("ERROR: --findings-json is required for fingerprint mode\n")
            return EXIT_USAGE_ERROR
        baselines_path = (
            args.baselines_file.resolve()
            if args.baselines_file is not None
            else repo_root / "scripts" / "lint" / "ratchet" / "rule_baselines.toml"
        )
        return _run_fingerprint_mode(
            repo_root,
            args.findings_json.resolve(),
            baselines_path,
            update_fingerprints=args.update_fingerprints,
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
