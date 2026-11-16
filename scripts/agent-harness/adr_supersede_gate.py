#!/usr/bin/env python3
"""CI gate requiring new ADR `supersedes` edges to be referenced in the body.

Every `supersedes` edge removes the target ADR from `docdag query --binding`.
When an ADR adds a new `supersedes` edge relative to `--base REV`, this gate
requires the superseding ADR's body to contain a wikilink `[[DST]]` or
`[[DST|label]]` explaining why DST is replaced (see ADR-001011 / ADR-001012).

Usage:
    adr_supersede_gate.py --base REV [--dir docs/ADR]

Exit codes:
    0  No findings
    1  At least one new supersedes edge is missing a body wikilink
    2  Invalid CLI usage, missing directory, non-git repo, or unknown REV
"""

from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys

ADR_FILENAME_RE = re.compile(r"^(\d{6})\.md$")
WIKILINK_RE = re.compile(r"\[\[(\d{6})(?:\|[^\]\n]*)?\]\]")
SUPERSEDES_KEY_RE = re.compile(r"^supersedes\s*:\s*(.*)$")
SUPERSEDES_ENTRY_RE = re.compile(
    r"^(?:\[\[)?(?:ADR-)?(\d{6})(?:\|[^\]]*)?(?:\]\])?$", re.IGNORECASE
)
BLOCK_ITEM_RE = re.compile(r"^\s*-\s+(.*)$")


def split_frontmatter_and_body(text: str) -> tuple[list[str], str]:
    """Return (frontmatter_lines, body_text) split on the first two '---' lines."""
    lines = text.splitlines()
    if not lines or lines[0].lstrip("\ufeff").rstrip() != "---":
        return [], text
    for idx in range(1, len(lines)):
        if lines[idx].rstrip() == "---":
            return lines[1:idx], "\n".join(lines[idx + 1 :])
    return [], text


def _strip_yaml_comment(raw: str) -> str:
    """Strip an unquoted YAML `# ...` comment outside `[[...]]`."""
    quote: str | None = None
    bracket_depth = 0
    for idx, ch in enumerate(raw):
        if quote is not None:
            if ch == quote and (idx == 0 or raw[idx - 1] != "\\"):
                quote = None
            continue
        if ch in ('"', "'"):
            quote = ch
        elif ch == "[":
            bracket_depth += 1
        elif ch == "]":
            if bracket_depth > 0:
                bracket_depth -= 1
        elif ch == "#" and bracket_depth == 0 and (idx == 0 or raw[idx - 1].isspace()):
            return raw[:idx]
    return raw


def _find_matching_bracket(text: str, open_idx: int) -> int:
    """Return index of the ']' matching '[' at open_idx, or -1 if unclosed."""
    quote: str | None = None
    depth = 0
    for idx in range(open_idx, len(text)):
        ch = text[idx]
        if quote is not None:
            if ch == quote and (idx == 0 or text[idx - 1] != "\\"):
                quote = None
            continue
        if ch in ('"', "'"):
            quote = ch
        elif ch == "[":
            depth += 1
        elif ch == "]":
            depth -= 1
            if depth == 0:
                return idx
    return -1


def _split_inline_list(inner: str) -> list[str]:
    """Split a YAML inline list body on commas outside quotes and brackets."""
    items: list[str] = []
    start = 0
    quote: str | None = None
    bracket_depth = 0
    for idx, ch in enumerate(inner):
        if quote is not None:
            if ch == quote and (idx == 0 or inner[idx - 1] != "\\"):
                quote = None
            continue
        if ch in ('"', "'"):
            quote = ch
        elif ch == "[":
            bracket_depth += 1
        elif ch == "]":
            if bracket_depth > 0:
                bracket_depth -= 1
        elif ch == "," and bracket_depth == 0:
            token = inner[start:idx].strip()
            if token:
                items.append(token)
            start = idx + 1
    tail = inner[start:].strip()
    if tail:
        items.append(tail)
    return items


def _normalize_adr_id(token: str) -> str | None:
    """Extract a 6-digit ADR id from bare, quoted, or wikilink frontmatter items."""
    val = token.strip()
    if len(val) >= 2 and val[0] == val[-1] and val[0] in ('"', "'"):
        val = val[1:-1].strip()
    match = SUPERSEDES_ENTRY_RE.match(val)
    if match is None:
        return None
    return match.group(1)


def parse_supersedes(frontmatter_lines: list[str]) -> list[str]:
    """Parse deduplicated supersedes ADR ids in appearance order."""
    targets: list[str] = []
    seen: set[str] = set()

    def add_token(token: str) -> None:
        adr_id = _normalize_adr_id(token)
        if adr_id is not None and adr_id not in seen:
            seen.add(adr_id)
            targets.append(adr_id)

    idx = 0
    total = len(frontmatter_lines)
    while idx < total:
        line = frontmatter_lines[idx]
        key_match = SUPERSEDES_KEY_RE.match(line)
        if key_match is None:
            idx += 1
            continue

        rest = _strip_yaml_comment(key_match.group(1)).strip()
        if rest.startswith("["):
            combined = rest
            close_idx = _find_matching_bracket(combined, 0)
            while close_idx == -1 and idx + 1 < total:
                idx += 1
                next_part = _strip_yaml_comment(frontmatter_lines[idx]).strip()
                combined = f"{combined} {next_part}"
                close_idx = _find_matching_bracket(combined, 0)
            inner = combined[1:close_idx] if close_idx != -1 else combined[1:]
            for item in _split_inline_list(inner):
                add_token(item)
            idx += 1
        elif rest == "":
            idx += 1
            while idx < total:
                subline = frontmatter_lines[idx]
                stripped = subline.strip()
                if not stripped or stripped.startswith("#"):
                    idx += 1
                    continue
                item_match = BLOCK_ITEM_RE.match(subline)
                if item_match is None:
                    break
                item_raw = _strip_yaml_comment(item_match.group(1)).strip()
                if item_raw:
                    add_token(item_raw)
                idx += 1
        else:
            add_token(rest)
            idx += 1

    return targets


def body_wikilink_ids(body: str) -> set[str]:
    """Return all 6-digit ADR ids linked as [[DST]] or [[DST|label]] in body."""
    return set(WIKILINK_RE.findall(body))


def _run_git(cwd: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(cwd), *args],
        capture_output=True,
        text=True,
        check=False,
    )


def _git_show_file(
    repo_root: pathlib.Path, rev: str, rel_path: str
) -> str | None:
    """Return file text at rev:rel_path via git show, or None if absent at rev."""
    res = _run_git(repo_root, "show", f"{rev}:{rel_path}")
    if res.returncode != 0:
        return None
    return res.stdout


def format_finding(src: str, dst: str) -> str:
    """Format a gate violation finding for SRC -> DST."""
    return (
        f"ADR {src} adds supersedes: {dst} but its body never references [[{dst}]]. "
        f"A supersedes edge removes {dst} from the binding set (docdag query --binding). "
        f"Name {dst} in the body with the reason it is replaced, or use depends-on, "
        f"or drop the edge. See ADR-001011."
    )


def run_gate(base_rev: str, adr_dir: pathlib.Path) -> int:
    """Execute the supersede gate against base_rev for all NNNNNN.md in adr_dir."""
    if not adr_dir.is_dir():
        print(f"error: ADR directory does not exist: {adr_dir}", file=sys.stderr)
        return 2

    if not base_rev or base_rev.startswith("-"):
        print(f"error: invalid base revision: {base_rev!r}", file=sys.stderr)
        return 2

    top_res = _run_git(adr_dir, "rev-parse", "--show-toplevel")
    if top_res.returncode != 0:
        print(
            f"error: {adr_dir} is not inside a git repository: {top_res.stderr.strip()}",
            file=sys.stderr,
        )
        return 2

    repo_root = pathlib.Path(top_res.stdout.strip()).resolve()
    rev_res = _run_git(repo_root, "rev-parse", "--verify", "--quiet", f"{base_rev}^{{commit}}")
    if rev_res.returncode != 0:
        print(f"error: unknown base revision: {base_rev}", file=sys.stderr)
        return 2

    try:
        dir_rel = adr_dir.resolve().relative_to(repo_root)
    except ValueError:
        print(
            f"error: {adr_dir} is outside git repository {repo_root}",
            file=sys.stderr,
        )
        return 2

    adr_entries: list[tuple[str, pathlib.Path]] = []
    for entry in sorted(adr_dir.iterdir(), key=lambda p: p.name):
        if not entry.is_file():
            continue
        match = ADR_FILENAME_RE.match(entry.name)
        if match is not None:
            adr_entries.append((match.group(1), entry))

    findings: list[str] = []
    newly_superseded: set[str] = set()

    for src_id, path in adr_entries:
        current_text = path.read_text(encoding="utf-8")
        current_fm, current_body = split_frontmatter_and_body(current_text)
        current_supersedes = parse_supersedes(current_fm)
        if not current_supersedes:
            continue

        rel_file = (dir_rel / path.name).as_posix()
        base_text = _git_show_file(repo_root, base_rev, rel_file)
        if base_text is None:
            base_supersedes: set[str] = set()
        else:
            base_fm, _ = split_frontmatter_and_body(base_text)
            base_supersedes = set(parse_supersedes(base_fm))

        new_targets = [dst for dst in current_supersedes if dst not in base_supersedes]
        if not new_targets:
            continue

        refs = body_wikilink_ids(current_body)
        for dst in new_targets:
            newly_superseded.add(dst)
            if dst not in refs:
                findings.append(format_finding(src_id, dst))

    for finding in findings:
        print(finding)

    superseded_list = sorted(newly_superseded)
    noun = "ADR" if len(superseded_list) == 1 else "ADRs"
    ids_text = ", ".join(superseded_list) if superseded_list else "none"
    print(f"Summary: {len(superseded_list)} {noun} superseded by new edges: {ids_text}")

    return 1 if findings else 0


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Fail when an ADR adds a supersedes edge not referenced in its body."
    )
    parser.add_argument(
        "--base",
        required=True,
        help="Git base revision to compare working-tree ADRs against.",
    )
    parser.add_argument(
        "--dir",
        default="docs/ADR",
        help="Directory containing NNNNNN.md ADR files (default: docs/ADR).",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    return run_gate(args.base, pathlib.Path(args.dir))


if __name__ == "__main__":
    sys.exit(main())
