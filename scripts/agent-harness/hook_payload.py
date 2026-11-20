#!/usr/bin/env python3
"""Shared PreToolUse stdin JSON parser for Claude Code and Codex hooks."""

from __future__ import annotations

import json
import sys
from dataclasses import dataclass
from typing import Any

WATCHED_TOOLS = frozenset({"Edit", "Write", "NotebookEdit", "apply_patch"})

PATCH_OPEN_PREFIXES: tuple[str, ...] = (
    "*** Add File: ",
    "*** Update File: ",
    "*** Delete File: ",
)
PATCH_MOVE_PREFIX = "*** Move to: "


class HookPayloadError(ValueError):
    """Raised when stdin is not a valid PreToolUse payload for a watched tool."""


@dataclass(frozen=True)
class HookPayload:
    tool_name: str
    paths: tuple[str, ...]
    added_text: dict[str, str]


def _append_unique(ordered: list[str], seen: set[str], path: str) -> None:
    if path not in seen:
        seen.add(path)
        ordered.append(path)


def extract_patch_details(command_text: str) -> tuple[list[str], dict[str, str]]:
    """Extract target file paths and added lines per path from an apply_patch body."""
    ordered: list[str] = []
    seen: set[str] = set()
    lines_by_path: dict[str, list[str]] = {}
    current_paths: list[str] = []

    for line in command_text.splitlines():
        matched_open = False
        for prefix in PATCH_OPEN_PREFIXES:
            if line.startswith(prefix):
                candidate = line[len(prefix) :].strip()
                if candidate:
                    _append_unique(ordered, seen, candidate)
                    lines_by_path.setdefault(candidate, [])
                    current_paths = [candidate]
                else:
                    current_paths = []
                matched_open = True
                break
        if matched_open:
            continue

        if line.startswith(PATCH_MOVE_PREFIX):
            candidate = line[len(PATCH_MOVE_PREFIX) :].strip()
            if candidate:
                _append_unique(ordered, seen, candidate)
                lines_by_path.setdefault(candidate, [])
                if candidate not in current_paths:
                    current_paths.append(candidate)
            continue

        if line.startswith("*** "):
            current_paths = []
            continue

        if line.startswith("+") and current_paths:
            added = line[1:]
            for target in current_paths:
                lines_by_path[target].append(added)

    return ordered, {p: "\n".join(lines) for p, lines in lines_by_path.items()}


def _extract_added_text_for_claude(tool_name: str, tool_input: dict[str, Any]) -> str:
    if tool_name == "Write":
        val = tool_input.get("content")
        return val if isinstance(val, str) else ""
    if tool_name == "Edit":
        val = tool_input.get("new_string")
        return val if isinstance(val, str) else ""
    if tool_name == "NotebookEdit":
        for key in ("new_source", "content"):
            val = tool_input.get(key)
            if isinstance(val, str):
                return val
    return ""


def _collect_apply_patch_paths(
    tool_input: dict[str, Any],
    ordered: list[str],
    seen: set[str],
    added_text: dict[str, str],
) -> None:
    cmd = tool_input.get("command")
    if isinstance(cmd, str):
        patch_text = cmd
    elif isinstance(cmd, list):
        patch_text = "\n".join(part for part in cmd if isinstance(part, str))
    else:
        patch_text = ""

    patch_paths, patch_added = extract_patch_details(patch_text)
    for path in patch_paths:
        _append_unique(ordered, seen, path)
        if path in added_text:
            combined = "\n".join(
                part for part in (added_text[path], patch_added.get(path, "")) if part
            )
            added_text[path] = combined
        else:
            added_text[path] = patch_added.get(path, "")


def parse_hook_stdin(raw: str) -> HookPayload | None:
    """Parse PreToolUse JSON from stdin.

    Returns None when tool_name is outside WATCHED_TOOLS (caller should exit 0).
    Raises HookPayloadError when stdin is malformed or a watched tool yields no path.
    """
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        msg = f"guard could not read the hook payload: invalid JSON ({exc.msg})"
        raise HookPayloadError(msg) from exc

    if not isinstance(payload, dict):
        msg = "guard could not read the hook payload: expected a JSON object"
        raise HookPayloadError(msg)

    tool_name = payload.get("tool_name")
    if not isinstance(tool_name, str) or not tool_name.strip():
        msg = "guard could not read the hook payload: missing tool_name"
        raise HookPayloadError(msg)

    if tool_name not in WATCHED_TOOLS:
        return None

    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        msg = f"guard could not read the hook payload: missing tool_input for {tool_name}"
        raise HookPayloadError(msg)

    ordered: list[str] = []
    seen: set[str] = set()
    added_text: dict[str, str] = {}

    claude_added = _extract_added_text_for_claude(tool_name, tool_input)
    for field in ("file_path", "notebook_path"):
        value = tool_input.get(field)
        if isinstance(value, str) and value.strip():
            cleaned = value.strip()
            _append_unique(ordered, seen, cleaned)
            added_text[cleaned] = claude_added

    if tool_name == "apply_patch":
        _collect_apply_patch_paths(tool_input, ordered, seen, added_text)

    if not ordered:
        msg = f"guard could not read the hook payload: {tool_name} yielded no target path"
        raise HookPayloadError(msg)

    return HookPayload(tool_name=tool_name, paths=tuple(ordered), added_text=added_text)


def read_hook_stdin_or_exit() -> HookPayload | None:
    """Read stdin and parse the hook payload, exiting 2 on fail-closed error."""
    raw = sys.stdin.read()
    try:
        return parse_hook_stdin(raw)
    except HookPayloadError as exc:
        sys.stderr.write(f"{exc}\n")
        sys.exit(2)
