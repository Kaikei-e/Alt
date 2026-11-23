#!/usr/bin/env python3
"""CI gate requiring a documented recovery plan on destructive SQL migrations.

PM-2026-057 / PM-2026-060: destructive migrations that run automatically on
deploy without a recorded backfill/recovery plan silently wipe or hide data.
This gate inspects only git-tracked `*.sql` files under any `migrations`
directory that were ADDED relative to the merge base with `--base REV`.
Existing migrations at the merge base are never examined.

A destructive statement passes only if a real `-- destructive-ok: <20+ chars>`
line comment lies between the end of the previous statement and the end of
that statement (for statements inside a `DO $$ ... $$` block, either inside the
block for that statement or directly above the `DO` statement).

Usage:
    migration_destructive_gate.py --base REV [--root PATH]

Exit codes:
    0  No findings
    1  At least one destructive statement without a valid destructive-ok comment
    2  Invalid CLI usage, non-git root, unknown REV, or git error
"""

from __future__ import annotations

import argparse
import os
import pathlib
import re
import shutil
import subprocess
import sys

MIN_REASON_LEN = 20
DESTRUCTIVE_OK_RE = re.compile(r"^--\s*destructive-ok:(.*)$")
DOLLAR_TAG_RE = re.compile(r"\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$")
WHERE_PAREN_TOKEN_RE = re.compile(r"\(|\)|\bWHERE\b", re.IGNORECASE)

DROP_TABLE_RE = re.compile(r"\bDROP\s+TABLE\b", re.IGNORECASE)
DROP_SCHEMA_RE = re.compile(r"\bDROP\s+SCHEMA\b", re.IGNORECASE)
DROP_DATABASE_RE = re.compile(r"\bDROP\s+DATABASE\b", re.IGNORECASE)
DROP_PARTITION_RE = re.compile(r"\bDROP\s+PARTITION\b", re.IGNORECASE)
DROP_CASCADE_RE = re.compile(
    r"\bDROP\b[^,;()]*?(?:\([^)]*\)[^,;()]*?)?(?<!\bON\sDELETE\s)\bCASCADE\b",
    re.IGNORECASE,
)
ALTER_TABLE_RE = re.compile(r"\bALTER\s+TABLE\b", re.IGNORECASE)
ALTER_DROP_COLUMN_RE = re.compile(
    r"\bDROP\s+"
    r"(?!CONSTRAINT\b|DEFAULT\b|NOT\s+NULL\b|EXPRESSION\b|IDENTITY\b"
    r"|INDEX\b|PARTITION\b|PRIMARY\s+KEY\b|PROJECTION\b)"
    r"(?:COLUMN\s+)?(?:IF\s+EXISTS\s+)?(?:[A-Za-z_][A-Za-z0-9_]*|\"[^\"]+\")",
    re.IGNORECASE,
)
ALTER_USING_NULL_RE = re.compile(
    r"\bALTER\s+(?:COLUMN\s+)?"
    r"(?!(?:TABLE|INDEX|VIEW|SCHEMA|DATABASE|TYPE|SEQUENCE)\b)"
    r"(?:[A-Za-z_][A-Za-z0-9_]*|\"[^\"]+\")\s+"
    r"(?:SET\s+DATA\s+)?TYPE\b.*?\bUSING\s*\(?\s*NULL\b",
    re.IGNORECASE | re.DOTALL,
)
ALTER_MODIFY_TTL_RE = re.compile(
    r"\bALTER\s+TABLE\b.*?\bMODIFY\s+TTL\b",
    re.IGNORECASE | re.DOTALL,
)
ALTER_CH_DELETE_RE = re.compile(
    r"\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?"
    r"(?:[A-Za-z_][A-Za-z0-9_]*|\"[^\"]+\")"
    r"(?:\.(?:[A-Za-z_][A-Za-z0-9_]*|\"[^\"]+\"))?"
    r"(?:\s+ON\s+CLUSTER\s+(?:[A-Za-z_][A-Za-z0-9_]*|\"[^\"]+\"))?"
    r"\s+DELETE\b",
    re.IGNORECASE,
)
TRUNCATE_RE = re.compile(r"\bTRUNCATE\b", re.IGNORECASE)
DELETE_FROM_RE = re.compile(r"\bDELETE\s+FROM\b", re.IGNORECASE)
UPDATE_SET_NULL_RE = re.compile(
    r"\bUPDATE\b.*?\bSET\b.*?=\s*\(?\s*NULL\b",
    re.IGNORECASE | re.DOTALL,
)


def _run_git(cwd: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    git_bin = shutil.which("git")
    if git_bin is None:
        msg = "git executable not found on PATH"
        raise FileNotFoundError(msg)
    return subprocess.run(  # noqa: S603 -- git resolved via shutil.which with explicit args
        [git_bin, "-C", str(cwd), *args],
        capture_output=True,
        text=True,
        check=False,
    )


def is_migration_sql(rel_path: str) -> bool:
    """Return True for *.sql under any directory named `migrations`."""
    if not rel_path.lower().endswith(".sql"):
        return False
    parts = pathlib.PurePosixPath(rel_path).parts
    return "migrations" in parts[:-1]


def _scan_line_comment(text: str, out: list[str], start: int) -> tuple[int, int | None]:
    n = len(text)
    out[start] = " "
    out[start + 1] = " "
    i = start + 2
    while i < n and text[i] != "\n":
        out[i] = " "
        i += 1
    raw_comment = text[start:i].strip()
    ok_match = DESTRUCTIVE_OK_RE.match(raw_comment)
    ok_offset = (
        start if ok_match is not None and len(ok_match.group(1).strip()) >= MIN_REASON_LEN else None
    )
    return i, ok_offset


def _scan_block_comment(text: str, out: list[str], start: int) -> int:
    n = len(text)
    out[start] = " "
    out[start + 1] = " "
    i = start + 2
    depth = 1
    while i < n and depth > 0:
        if text[i] == "/" and i + 1 < n and text[i + 1] == "*":
            out[i] = " "
            out[i + 1] = " "
            depth += 1
            i += 2
        elif text[i] == "*" and i + 1 < n and text[i + 1] == "/":
            out[i] = " "
            out[i + 1] = " "
            depth -= 1
            i += 2
        else:
            if text[i] != "\n":
                out[i] = " "
            i += 1
    return i


def _scan_dollar_block(
    text: str, out: list[str], start: int
) -> tuple[int, tuple[int, int, int, str] | None]:
    tag_match = DOLLAR_TAG_RE.match(text, start)
    if tag_match is None:
        return start + 1, None
    tag = tag_match.group(0)
    body_start = tag_match.end()
    close_idx = text.find(tag, body_start)
    if close_idx == -1:
        return body_start, None
    outer_end = close_idx + len(tag)
    for j in range(start, outer_end):
        if text[j] != "\n":
            out[j] = " "
    return outer_end, (start, outer_end, body_start, text[body_start:close_idx])


def _scan_escaped_string(text: str, out: list[str], start: int) -> int:
    n = len(text)
    i = start + 2
    while i < n:
        if text[i] == "\\" and i + 1 < n:
            if text[i] != "\n":
                out[i] = " "
            if text[i + 1] != "\n":
                out[i + 1] = " "
            i += 2
        elif text[i] == "'":
            if i + 1 < n and text[i + 1] == "'":
                out[i] = " "
                out[i + 1] = " "
                i += 2
                continue
            return i + 1
        else:
            if text[i] != "\n":
                out[i] = " "
            i += 1
    return i


def _scan_single_quoted(text: str, out: list[str], start: int) -> int:
    n = len(text)
    i = start + 1
    while i < n:
        if text[i] == "'":
            if i + 1 < n and text[i + 1] == "'":
                out[i] = " "
                out[i + 1] = " "
                i += 2
                continue
            return i + 1
        if text[i] != "\n":
            out[i] = " "
        i += 1
    return i


def _scan_double_quoted(text: str, start: int) -> int:
    n = len(text)
    i = start + 1
    while i < n:
        if text[i] == '"':
            if i + 1 < n and text[i + 1] == '"':
                i += 2
                continue
            return i + 1
        i += 1
    return i


def _scan_sql(
    text: str,
) -> tuple[str, list[int], list[int], list[tuple[int, int, int, str]]]:
    """Mask comments and literals while tracking semicolons, ok comments, and $$ blocks.

    Returns:
        masked: same length and newline positions as `text`, with comments,
            string contents, and dollar-quoted blocks masked to spaces.
        semicolons: top-level `;` offsets in `text`.
        ok_comment_offsets: offsets of real `-- destructive-ok: <20+ chars>`
            line comments (not inside block comments, strings, or $$ blocks).
        dollar_blocks: list of (outer_start, outer_end, body_start, body_text).
    """
    n = len(text)
    out = list(text)
    semicolons: list[int] = []
    ok_comment_offsets: list[int] = []
    dollar_blocks: list[tuple[int, int, int, str]] = []
    i = 0

    while i < n:
        ch = text[i]
        if ch == "-" and i + 1 < n and text[i + 1] == "-":
            i, ok_offset = _scan_line_comment(text, out, i)
            if ok_offset is not None:
                ok_comment_offsets.append(ok_offset)
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "*":
            i = _scan_block_comment(text, out, i)
            continue
        if ch == "$" and DOLLAR_TAG_RE.match(text, i) is not None:
            i, block = _scan_dollar_block(text, out, i)
            if block is not None:
                dollar_blocks.append(block)
            continue
        if (
            ch in ("e", "E")
            and i + 1 < n
            and text[i + 1] == "'"
            and (i == 0 or not (text[i - 1].isalnum() or text[i - 1] in ("_", "$")))
        ):
            i = _scan_escaped_string(text, out, i)
            continue
        if ch == "'":
            i = _scan_single_quoted(text, out, i)
            continue
        if ch == '"':
            i = _scan_double_quoted(text, i)
            continue
        if ch == ";":
            semicolons.append(i)
        i += 1

    return "".join(out), semicolons, ok_comment_offsets, dollar_blocks


def _paren_depth_at(text: str, pos: int) -> int:
    depth = 0
    for ch in text[:pos]:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
    return depth


def _has_where_after(stmt: str, kw_start: int, kw_len: int) -> bool:
    """True when a WHERE keyword follows kw_start at the same paren depth."""
    kw_depth = _paren_depth_at(stmt, kw_start)
    depth = kw_depth
    for tok_match in WHERE_PAREN_TOKEN_RE.finditer(stmt, kw_start + kw_len):
        tok = tok_match.group(0)
        if tok == "(":
            depth += 1
        elif tok == ")":
            depth -= 1
        elif depth == kw_depth:
            return True
    return False


def _statement_findings(
    masked: str,
    stmt_start: int,
    stmt_end: int,
    line_offset: int,
) -> list[tuple[int, str]]:
    stmt = masked[stmt_start:stmt_end]
    if not stmt.strip():
        return []

    findings: list[tuple[int, str]] = []
    matched_drop_positions: set[int] = set()

    def line_of(rel_pos: int) -> int:
        return line_offset + 1 + masked[: stmt_start + rel_pos].count("\n")

    for pattern, kind in (
        (DROP_TABLE_RE, "DROP TABLE"),
        (DROP_SCHEMA_RE, "DROP SCHEMA"),
        (DROP_DATABASE_RE, "DROP DATABASE"),
        (DROP_PARTITION_RE, "DROP PARTITION"),
    ):
        for m in pattern.finditer(stmt):
            matched_drop_positions.add(m.start())
            findings.append((line_of(m.start()), kind))

    if ALTER_TABLE_RE.search(stmt) is not None:
        for m in ALTER_DROP_COLUMN_RE.finditer(stmt):
            matched_drop_positions.add(m.start())
            findings.append((line_of(m.start()), "DROP COLUMN"))
        findings.extend(
            (line_of(m.start()), "ALTER COLUMN ... TYPE ... USING NULL")
            for m in ALTER_USING_NULL_RE.finditer(stmt)
        )
        findings.extend(
            (line_of(m.start()), "MODIFY TTL") for m in ALTER_MODIFY_TTL_RE.finditer(stmt)
        )
        findings.extend(
            (line_of(m.start()), "ALTER TABLE ... DELETE")
            for m in ALTER_CH_DELETE_RE.finditer(stmt)
        )

    findings.extend(
        (line_of(m.start()), "DROP ... CASCADE")
        for m in DROP_CASCADE_RE.finditer(stmt)
        if m.start() not in matched_drop_positions
    )

    findings.extend((line_of(m.start()), "TRUNCATE") for m in TRUNCATE_RE.finditer(stmt))

    findings.extend(
        (line_of(m.start()), "DELETE FROM without WHERE")
        for m in DELETE_FROM_RE.finditer(stmt)
        if not _has_where_after(stmt, m.start(), len("DELETE"))
    )

    findings.extend(
        (line_of(m.start()), "UPDATE ... SET ... = NULL without WHERE")
        for m in UPDATE_SET_NULL_RE.finditer(stmt)
        if not _has_where_after(stmt, m.start(), len("UPDATE"))
    )

    return findings


def _scan_findings(
    text: str,
    *,
    line_offset: int = 0,
    inherited_ok: bool = False,
    respect_ok: bool = True,
) -> list[tuple[int, str]]:
    masked, semicolons, ok_comment_offsets, dollar_blocks = _scan_sql(text)
    boundaries = [0, *[idx + 1 for idx in semicolons], len(text)]
    findings: list[tuple[int, str]] = []

    for pos in range(len(boundaries) - 1):
        stmt_start = boundaries[pos]
        stmt_end = boundaries[pos + 1]
        stmt_ok = inherited_ok or any(
            stmt_start <= offset < stmt_end for offset in ok_comment_offsets
        )
        if not (respect_ok and stmt_ok):
            findings.extend(_statement_findings(masked, stmt_start, stmt_end, line_offset))

        for d_start, _d_end, body_start, body_text in dollar_blocks:
            if stmt_start <= d_start < stmt_end:
                nested_offset = line_offset + text[:body_start].count("\n")
                findings.extend(
                    _scan_findings(
                        body_text,
                        line_offset=nested_offset,
                        inherited_ok=stmt_ok,
                        respect_ok=respect_ok,
                    )
                )

    return findings


def find_destructive_statements(text: str) -> list[tuple[int, str]]:
    """Return [(1_based_line, statement_kind), ...] for all destructive statements."""
    return _scan_findings(text, respect_ok=False)


def check_migration_text(rel_path: str, text: str) -> list[str]:
    """Return formatted M1 finding strings for a single added migration file."""
    return [
        f"M1 {rel_path}:{line}: {kind} without a destructive-ok comment"
        for line, kind in _scan_findings(text, respect_ok=True)
    ]


def _resolve_git_context(base_rev: str, root_dir: pathlib.Path) -> tuple[pathlib.Path, str] | int:
    if not root_dir.is_dir():
        sys.stderr.write(f"error: repository root does not exist: {root_dir}\n")
        return 2
    if not base_rev or base_rev.startswith("-"):
        sys.stderr.write(f"error: invalid base revision: {base_rev!r}\n")
        return 2

    top_res = _run_git(root_dir, "rev-parse", "--show-toplevel")
    if top_res.returncode != 0:
        sys.stderr.write(
            f"error: {root_dir} is not inside a git repository: {top_res.stderr.strip()}\n"
        )
        return 2
    repo_root = pathlib.Path(top_res.stdout.strip()).resolve()

    rev_res = _run_git(repo_root, "rev-parse", "--verify", "--quiet", f"{base_rev}^{{commit}}")
    if rev_res.returncode != 0:
        sys.stderr.write(f"error: unknown base revision: {base_rev}\n")
        return 2

    mb_res = _run_git(repo_root, "merge-base", base_rev, "HEAD")
    if mb_res.returncode != 0 or not mb_res.stdout.strip():
        sys.stderr.write(
            f"error: cannot compute merge-base between {base_rev} and HEAD: "
            f"{mb_res.stderr.strip()}\n"
        )
        return 2
    return repo_root, mb_res.stdout.strip()


def _added_migration_paths(repo_root: pathlib.Path, merge_base: str) -> list[str] | int:
    ls_cur = _run_git(repo_root, "ls-files", "-z")
    if ls_cur.returncode != 0:
        sys.stderr.write(f"error: git ls-files failed: {ls_cur.stderr.strip()}\n")
        return 2
    ls_base = _run_git(repo_root, "ls-tree", "-r", "-z", "--name-only", merge_base)
    if ls_base.returncode != 0:
        sys.stderr.write(f"error: git ls-tree failed: {ls_base.stderr.strip()}\n")
        return 2

    current = {p for p in ls_cur.stdout.split("\0") if p}
    base = {p for p in ls_base.stdout.split("\0") if p}
    return [p for p in sorted(current - base) if is_migration_sql(p)]


def run_gate(base_rev: str, root_dir: pathlib.Path) -> int:
    """Execute the destructive migration gate against merge-base(base_rev, HEAD)."""
    ctx = _resolve_git_context(base_rev, root_dir)
    if isinstance(ctx, int):
        return ctx
    repo_root, merge_base = ctx

    added_paths = _added_migration_paths(repo_root, merge_base)
    if isinstance(added_paths, int):
        return added_paths

    findings: list[str] = []
    checked = 0
    for rel_path in added_paths:
        full_path = repo_root / rel_path
        if not full_path.is_file():
            continue
        checked += 1
        text = full_path.read_text(encoding="utf-8", errors="replace")
        findings.extend(check_migration_text(rel_path, text))

    if findings:
        for finding in findings:
            sys.stdout.write(f"{finding}\n")
        sys.stdout.write(
            "\nAdd a '-- destructive-ok: <at least 20 chars explaining the backfill "
            "command, completion query, or why no data is lost>' comment line directly "
            "above or inside each flagged statement.\n"
        )
        return 1

    if checked == 0:
        sys.stdout.write(f"OK: no migration files added relative to merge base {merge_base}\n")
        return 0

    sys.stdout.write(
        f"OK: checked {checked} added migration file(s) relative to merge base "
        f"{merge_base}, 0 findings\n"
    )
    return 0


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Fail when an added migration SQL file performs a destructive "
            "operation without a `-- destructive-ok: <reason>` comment."
        )
    )
    parser.add_argument(
        "--base",
        required=True,
        help="Git base revision used to compute the merge base against HEAD.",
    )
    parser.add_argument(
        "--root",
        default=".",
        help="Repository root override (default: current directory).",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    return run_gate(args.base, pathlib.Path(args.root))


if __name__ == "__main__":
    _launcher = (
        pathlib.Path(__file__).resolve().parents[2] / "scripts" / "altharness" / "alt-harness"
    )
    if _launcher.is_file():
        os.execv(_launcher, [str(_launcher), "migration-gate", *sys.argv[1:]])  # noqa: S606
    sys.exit(main())
