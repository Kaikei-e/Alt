#!/usr/bin/env python3
"""PreToolUse guard preventing wall-clock reads and GetLatest queries in projectors."""

from __future__ import annotations

import posixpath
import re
import sys
from pathlib import Path, PurePosixPath

_SELF_DIR = str(Path(__file__).resolve().parent)
if _SELF_DIR not in sys.path:
    sys.path.insert(0, _SELF_DIR)

from hook_payload import read_hook_stdin_or_exit  # noqa: E402

GUARDED_GLOBS: list[str] = [
    "knowledge-sovereign/app/usecase/knowledge_home_projector/*.go",
    "knowledge-sovereign/app/usecase/knowledge_trail_projector/*.go",
]

WALL_CLOCK_RE = re.compile(r"\btime\.(?:Now|Since|Until)\s*\(")
GET_LATEST_RE = re.compile(r"\bGetLatest[A-Z][A-Za-z0-9_]*")


def _glob_to_regex(pattern: str) -> re.Pattern[str]:
    pat = pattern.strip().lstrip("./")
    out: list[str] = ["^"]
    i = 0
    n = len(pat)
    while i < n:
        if pat[i : i + 3] == "**/":
            out.append("(?:.+/)?")
            i += 3
        elif pat[i : i + 3] == "/**" and i + 3 == n:
            out.append("/.+")
            i += 3
        elif pat[i : i + 2] == "**":
            out.append(".*")
            i += 2
        elif pat[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pat[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pat[i]))
            i += 1
    out.append("$")
    return re.compile("".join(out))


_GUARDED_REGEXES: tuple[re.Pattern[str], ...] = tuple(
    _glob_to_regex(pat) for pat in GUARDED_GLOBS
)


def is_guarded_path(raw_path: str) -> bool:
    """Return True if raw_path matches GUARDED_GLOBS on a repo-relative suffix and is not _test.go."""
    norm = posixpath.normpath(raw_path.replace("\\", "/")).lstrip("./")
    if not norm or norm.endswith("_test.go"):
        return False

    parts = [part for part in PurePosixPath(norm).parts if part not in ("/", "")]
    for idx in range(len(parts)):
        suffix = "/".join(parts[idx:])
        if any(rx.match(suffix) for rx in _GUARDED_REGEXES):
            return True
    return False


def strip_go_line_comments(text: str) -> str:
    """Strip Go // line comments while ignoring // inside double-quoted string literals."""
    stripped_lines: list[str] = []
    for line in text.splitlines():
        in_double = False
        escaped = False
        cut_at: int | None = None
        i = 0
        n = len(line)
        while i < n:
            ch = line[i]
            if in_double:
                if escaped:
                    escaped = False
                elif ch == "\\":
                    escaped = True
                elif ch == '"':
                    in_double = False
            else:
                if ch == '"':
                    in_double = True
                elif ch == "/" and i + 1 < n and line[i + 1] == "/":
                    cut_at = i
                    break
            i += 1
        stripped_lines.append(line if cut_at is None else line[:cut_at])
    return "\n".join(stripped_lines)


def find_event_time_violation(added_text: str) -> str | None:
    """Return the offending call token if added_text introduces a prohibited call."""
    code = strip_go_line_comments(added_text)
    m_clock = WALL_CLOCK_RE.search(code)
    if m_clock is not None:
        return m_clock.group(0).rstrip()
    m_latest = GET_LATEST_RE.search(code)
    if m_latest is not None:
        return m_latest.group(0)
    return None


def main() -> int:
    payload = read_hook_stdin_or_exit()
    if payload is None:
        return 0

    violations: list[str] = []
    for path in payload.paths:
        if not is_guarded_path(path):
            continue
        added = payload.added_text.get(path, "")
        call = find_event_time_violation(added)
        if call is not None:
            violations.append(
                f"Blocked edit to {path}: found '{call}' in projector code. "
                "Projectors must be reproject-safe: derive time from the event's OccurredAt "
                "and read the version id carried in the event payload "
                "(see ADR-000468 and ADR-000447)."
            )

    if violations:
        print("\n".join(violations), file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
