#!/usr/bin/env python3
"""PreToolUse guard blocking edits to DO NOT EDIT generated files, stopped-using-k8s/, and out-of-scope packet paths (EXEC-02)."""

from __future__ import annotations

import importlib
import os
import re
import sys
from pathlib import Path
from typing import Any

_SELF_DIR = str(Path(__file__).resolve().parent)
if _SELF_DIR not in sys.path:
    sys.path.insert(0, _SELF_DIR)

_hook_payload = importlib.import_module("hook_payload")
_task_packet = importlib.import_module("task_packet")
_harness_run = importlib.import_module("harness_run")

BLOCKED_RC = 2
EXCLUDED_GENERATED_PREFIXES: tuple[str, ...] = ("stopped-using-k8s/", "scripts/harness/corpus/")
HISTORICAL_K8S_PREFIX = "stopped-using-k8s"


def _expand_braces(pattern: str) -> list[str]:
    start = -1
    depth = 0
    for idx, ch in enumerate(pattern):
        if ch == "{":
            if depth == 0:
                start = idx
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0 and start != -1:
                prefix = pattern[:start]
                inner = pattern[start + 1 : idx]
                suffix = pattern[idx + 1 :]
                options: list[str] = []
                buf: list[str] = []
                inner_depth = 0
                for c in inner:
                    if c == "{":
                        inner_depth += 1
                        buf.append(c)
                    elif c == "}":
                        inner_depth -= 1
                        buf.append(c)
                    elif c == "," and inner_depth == 0:
                        options.append("".join(buf))
                        buf = []
                    else:
                        buf.append(c)
                options.append("".join(buf))
                expanded: list[str] = []
                for opt in options:
                    expanded.extend(_expand_braces(prefix + opt + suffix))
                return expanded
    return [pattern]


def _glob_to_regex(pattern: str) -> re.Pattern[str]:
    pat = pattern.strip().removeprefix("./")
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


def matches_glob(pattern: str, rel_path: str) -> bool:
    """Return True if repo-relative POSIX path matches glob pattern (supporting ** and {})."""
    norm_rel = rel_path.strip().removeprefix("./")
    return any(_glob_to_regex(alt).match(norm_rel) is not None for alt in _expand_braces(pattern))


def _resolve_relative_base(repo_resolved: Path, cwd: str | Path | None) -> Path:
    if cwd is not None:
        return Path(cwd)
    proc_cwd = Path.cwd().resolve()
    if proc_cwd.is_relative_to(repo_resolved):
        return proc_cwd
    return repo_resolved


def _resolve_repo_rel_or_outside(
    raw_path: str,
    repo_root: Path,
    *,
    cwd: str | Path | None = None,
) -> tuple[str | None, str | None]:
    """Resolve raw_path against payload cwd (or repo_root fallback).

    Returns (repo_rel_posix_or_None_if_outside, block_reason_or_None).
    Symlinks escaping repo_root always return a block_reason.
    """
    if "\x00" in raw_path or not raw_path.strip():
        return None, f"BLOCKED [EXEC-02]: invalid target path: {raw_path!r}"

    repo_resolved = repo_root.resolve()
    p = Path(raw_path)
    base_dir = _resolve_relative_base(repo_resolved, cwd)
    candidate = p if p.is_absolute() else (base_dir / p)
    lexical_abs = Path(os.path.normpath(str(candidate)))

    try:
        resolved = candidate.resolve()
    except OSError as exc:
        return None, f"BLOCKED [EXEC-02]: could not resolve target path '{raw_path}' ({exc})"

    lexical_inside = lexical_abs.is_relative_to(repo_resolved) or lexical_abs.is_relative_to(
        repo_root
    )
    resolved_inside = resolved.is_relative_to(repo_resolved)

    if lexical_inside and not resolved_inside:
        return (
            None,
            f"BLOCKED [EXEC-02]: symlink escapes repository root: '{raw_path}' -> '{resolved}'",
        )

    if not resolved_inside:
        return None, None

    return resolved.relative_to(repo_resolved).as_posix(), None


def _check_historical_k8s(rel_path: str, active_packet: dict[str, Any] | None) -> str | None:
    if rel_path != HISTORICAL_K8S_PREFIX and not rel_path.startswith(f"{HISTORICAL_K8S_PREFIX}/"):
        return None
    if active_packet is not None:
        allowed_paths = active_packet.get("allowed_paths") or []
        if any(
            isinstance(pat, str) and HISTORICAL_K8S_PREFIX in pat and matches_glob(pat, rel_path)
            for pat in allowed_paths
        ):
            return None
    return (
        f"BLOCKED [EXEC-02]: historical asset '{rel_path}' under stopped-using-k8s/ is read-only "
        "unless explicitly authorized in active packet allowed_paths."
    )


def _check_active_packet_scope(rel_path: str, active_packet: dict[str, Any]) -> str | None:
    packet_id = str(active_packet.get("packet_id", "unknown"))
    forbidden_paths = active_packet.get("forbidden_paths") or []
    if any(isinstance(pat, str) and matches_glob(pat, rel_path) for pat in forbidden_paths):
        return (
            f"BLOCKED [EXEC-02]: '{rel_path}' matches forbidden_paths "
            f"in active packet '{packet_id}'."
        )

    allowed_paths = active_packet.get("allowed_paths") or []
    if not allowed_paths or not any(
        isinstance(pat, str) and matches_glob(pat, rel_path) for pat in allowed_paths
    ):
        return (
            f"BLOCKED [EXEC-02]: '{rel_path}' is outside allowed_paths "
            f"in active packet '{packet_id}'."
        )
    return None


def _check_inside_repo_path(
    rel_path: str,
    repo_root: Path,
    active_packet: dict[str, Any] | None,
) -> str | None:
    # (a) Generated DO NOT EDIT guard (C-16)
    allow_gen = bool(active_packet and active_packet.get("allow_generated_code_edits") is True)
    if (
        not allow_gen
        and not rel_path.startswith(EXCLUDED_GENERATED_PREFIXES)
        and _task_packet._repo_rel_is_generated_code(repo_root.resolve(), rel_path)
    ):
        return (
            f"BLOCKED [EXEC-02]: generated DO NOT EDIT file '{rel_path}' cannot be edited "
            "directly. Regenerate it from its source schema/spec instead "
            "(or opt in via allow_generated_code_edits: true in an active packet)."
        )

    # (b) Historical stopped-using-k8s/ guard (R-33)
    k8s_err = _check_historical_k8s(rel_path, active_packet)
    if k8s_err is not None:
        return k8s_err

    # (c) Active packet scope (allowed_paths / forbidden_paths)
    if active_packet is not None:
        return _check_active_packet_scope(rel_path, active_packet)

    return None


def check_target_path(
    raw_path: str,
    repo_root: Path,
    active_packet: dict[str, Any] | None,
    *,
    cwd: str | Path | None = None,
) -> str | None:
    """Return a block reason string if editing raw_path is forbidden, else None."""
    rel_path, resolve_err = _resolve_repo_rel_or_outside(raw_path, repo_root, cwd=cwd)
    if resolve_err is not None:
        return resolve_err

    if rel_path is None:
        if active_packet is not None:
            packet_id = str(active_packet.get("packet_id", "unknown"))
            return (
                f"BLOCKED [EXEC-02]: '{raw_path}' is outside repository root and outside "
                f"allowed_paths in active packet '{packet_id}'."
            )
        return None

    return _check_inside_repo_path(rel_path, repo_root, active_packet)


def main(argv: list[str] | None = None) -> int:
    _ = argv
    payload = _hook_payload.read_hook_stdin_or_exit()
    if payload is None:
        return 0

    repo_root = _harness_run.resolve_repo_root(start_dir=payload.cwd)
    active_packet, pkt_err = _harness_run.load_active_packet(repo_root)
    if pkt_err is not None:
        sys.stderr.write(f"BLOCKED [EXEC-02]: {pkt_err}.\n")
        return BLOCKED_RC

    violations: list[str] = []
    for raw_path in payload.paths:
        err = check_target_path(raw_path, repo_root, active_packet, cwd=payload.cwd)
        if err is not None:
            violations.append(err)

    if violations:
        sys.stderr.write("\n".join(violations) + "\n")
        return BLOCKED_RC
    return 0


if __name__ == "__main__":
    _launcher = Path(__file__).resolve().parents[2] / "scripts" / "altharness" / "alt-harness"
    if _launcher.is_file():
        os.execv(_launcher, [str(_launcher), "guard-scope", *sys.argv[1:]])  # noqa: S606 -- delegates CLI execution to the Go harness
    sys.exit(main())
