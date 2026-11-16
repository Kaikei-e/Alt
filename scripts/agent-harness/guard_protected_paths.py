#!/usr/bin/env python3
"""PreToolUse guard blocking edits to protected env, key, secret, and tracked migration files."""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

_SELF_DIR = str(Path(__file__).resolve().parent)
if _SELF_DIR not in sys.path:
    sys.path.insert(0, _SELF_DIR)

from hook_payload import read_hook_stdin_or_exit  # noqa: E402

ENV_ALLOWED_SUFFIXES: tuple[str, ...] = (".example", ".template", ".sample")
KEY_SUFFIXES: tuple[str, ...] = (".key", ".pem")
PROTECTED_DIRS = frozenset({"secrets", ".kisage", ".git"})


def _is_protected_env_name(name: str) -> bool:
    is_env = name == ".env" or name.startswith(".env.") or name.endswith(".env")
    return is_env and not name.endswith(ENV_ALLOWED_SUFFIXES)


def _is_tracked_in_git(raw_path: str) -> tuple[bool, str | None]:
    """Return (should_block, reason_if_unanswerable_or_tracked) for a migrations file."""
    abs_target = Path(os.path.abspath(raw_path))
    probe = abs_target.parent
    while not probe.is_dir():
        if probe.parent == probe:
            return True, "no existing parent directory was found to verify git tracking"
        probe = probe.parent

    tail = abs_target.relative_to(probe)
    probe_real = probe.resolve()
    try:
        res = subprocess.run(
            [
                "git",
                "-C",
                str(probe_real),
                "ls-files",
                "--error-unmatch",
                "--",
                str(tail),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError as exc:
        return True, f"git could not be executed ({exc})"

    if res.returncode == 0:
        return True, None
    if res.returncode == 1:
        return False, None
    detail = res.stderr.strip() or res.stdout.strip() or f"git exited {res.returncode}"
    return True, detail


def check_target_path(raw_path: str) -> str | None:
    """Return a 1-2 sentence block message if raw_path is protected, else None."""
    norm = os.path.normpath(raw_path)
    p = Path(norm)
    name = p.name
    dir_parts = p.parts[:-1]

    if _is_protected_env_name(name):
        return (
            f"Blocked edit to {raw_path}: environment files matching .env are protected "
            "unless they end in .example, .template, or .sample. "
            "Change host env through deploy/desired-env.lkal and the kisage tools, "
            "or edit a tracked template file instead."
        )

    if name.endswith(KEY_SUFFIXES):
        return (
            f"Blocked edit to {raw_path}: private key and certificate files (*.key, *.pem) "
            "must not be modified directly. "
            "Keys and certificates are issued by tooling, not written by agents; "
            "ask the user if one has to change."
        )

    for seg in dir_parts:
        if seg in PROTECTED_DIRS:
            if seg == ".kisage":
                next_step = (
                    "Change host env through deploy/desired-env.lkal and the kisage tools."
                )
            elif seg == "secrets":
                next_step = (
                    "Secret files are written by the host operator; ask the user to change them."
                )
            else:
                next_step = (
                    "Use standard git CLI commands instead of editing .git internals directly."
                )
            return (
                f"Blocked edit to {raw_path}: files inside a '{seg}' directory are protected. "
                f"{next_step}"
            )

    if "migrations" in dir_parts:
        blocked, git_err = _is_tracked_in_git(raw_path)
        if blocked:
            if git_err is not None:
                return (
                    f"Blocked edit to {raw_path}: migration tracking status could not be "
                    f"verified by git ({git_err}). "
                    "Ensure you are inside the git repository, add a new migration file, "
                    "and regenerate atlas.sum with atlas migrate hash."
                )
            return (
                f"Blocked edit to {raw_path}: tracked files under a 'migrations' directory "
                "are immutable once applied. "
                "Add a new migration file and regenerate atlas.sum with atlas migrate hash."
            )

    return None


def main() -> int:
    payload = read_hook_stdin_or_exit()
    if payload is None:
        return 0

    violations: list[str] = []
    for path in payload.paths:
        msg = check_target_path(path)
        if msg is not None:
            violations.append(msg)

    if violations:
        print("\n".join(violations), file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
