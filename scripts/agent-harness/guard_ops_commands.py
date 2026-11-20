#!/usr/bin/env python3
"""PreToolUse guard prompting human confirmation for risky Docker and Compose ops commands."""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Any

_SELF_DIR = str(Path(__file__).resolve().parent)
if _SELF_DIR not in sys.path:
    sys.path.insert(0, _SELF_DIR)

from hook_payload import HookPayloadError  # noqa: E402

WATCHED_SHELL_TOOLS = frozenset({"Bash", "shell"})
SHELL_NAMES = frozenset({"sh", "bash", "zsh", "dash"})

ENV_ASSIGN_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")

DOCKER_VALUE_FLAGS = frozenset(
    {
        "-H",
        "--host",
        "-c",
        "--context",
        "-l",
        "--log-level",
        "--config",
        "--tlscacert",
        "--tlscert",
        "--tlskey",
    }
)
COMPOSE_VALUE_FLAGS = frozenset(
    {
        "-f",
        "--file",
        "-p",
        "--project-name",
        "--project-directory",
        "--env-file",
        "--profile",
        "--ansi",
        "--progress",
        "--parallel",
        "-H",
        "--host",
        "-c",
        "--context",
        "-l",
        "--log-level",
        "--config",
        "--tlscacert",
        "--tlscert",
        "--tlskey",
        "--wait-timeout",
        "-t",
        "--timeout",
        "--scale",
        "--exit-code-from",
    }
)

PRUNE_GROUPS = frozenset({"system", "image", "container", "volume", "network", "builder", "buildx"})

RULE_REASONS: dict[str, str] = {
    "O1": (
        "Rule O1 (PM-2026-054, PM-2026-046): Confirm destructive Docker prune or "
        "volume/image/network removal because unconditional cleanup has swept live "
        "sidecars, image tags, and sibling resources."
    ),
    "O2": (
        "Rule O2 (PM-2026-028, PM-2026-054): Confirm container rm/stop/kill because "
        "removing or stopping running containers has left certificate renewers and "
        "sidecars absent."
    ),
    "O3": (
        "Rule O3 (PM-2026-001, PM-2026-005): Confirm docker compose up/down/rm on the "
        "production project because compose lifecycle changes can recreate dependent "
        "stateful or GPU containers as a side effect."
    ),
    "O4": (
        "Rule O4 (PM-2026-031): Confirm docker compose up with --build or "
        "--force-recreate because the release path builds and gates images and a "
        "direct rebuild skips the contract gate."
    ),
}

UNREADABLE_REASON = (
    "Hook payload or command was unreadable and could not be analysed, so confirm "
    "execution manually before proceeding."
)

DENY_GUIDANCE_SENTENCE = (
    "Do not retry or rephrase the command to get around the guard; instead tell "
    "the user which command is needed and why so the user can run it, or quote "
    "the flagged words if they are literal text such as a search pattern rather "
    "than a command."
)

RULE_PRIORITY: dict[str, int] = {"O4": 4, "O1": 3, "O2": 2, "O3": 1}
MAX_RECURSION_DEPTH = 32


@dataclass(frozen=True)
class Token:
    text: str
    quoted: bool


def _decision(permission_decision: str, reason: str) -> dict[str, Any]:
    return {
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": permission_decision,
            "permissionDecisionReason": reason,
        }
    }


def _pick_higher_rule(current: str | None, candidate: str | None) -> str | None:
    if candidate is None:
        return current
    if current is None or RULE_PRIORITY[candidate] > RULE_PRIORITY[current]:
        return candidate
    return current


@dataclass
class _SplitState:
    command: str
    length: int
    segments: list[list[Token]]
    current_seg: list[Token]
    buf: list[str]
    has_token: bool = False
    has_quote: bool = False
    quote: str = "NORMAL"

    def flush_token(self) -> None:
        if self.has_token:
            self.current_seg.append(Token("".join(self.buf), self.has_quote))
            self.buf.clear()
            self.has_token = False
            self.has_quote = False

    def flush_segment(self) -> None:
        self.flush_token()
        if self.current_seg:
            self.segments.append(list(self.current_seg))
            self.current_seg.clear()


def _scan_single(state: _SplitState, i: int) -> int:
    ch = state.command[i]
    if ch == "'":
        state.quote = "NORMAL"
    else:
        state.buf.append(ch)
    return i + 1


def _scan_double(state: _SplitState, i: int) -> int:
    ch = state.command[i]
    if ch == "\\":
        if i + 1 < state.length and state.command[i + 1] == "\n":
            return i + 2
        if i + 1 < state.length:
            state.buf.append(state.command[i + 1])
            return i + 2
    elif ch == '"':
        state.quote = "NORMAL"
        return i + 1
    state.buf.append(ch)
    return i + 1


def _scan_normal_backslash(state: _SplitState, i: int) -> int:
    if i + 1 < state.length and state.command[i + 1] == "\n":
        return i + 2
    if i + 1 < state.length:
        state.buf.append(state.command[i + 1])
        state.has_token = True
        return i + 2
    return i + 1


def _scan_normal(state: _SplitState, i: int) -> int:
    ch = state.command[i]
    if ch in ("'", '"'):
        state.quote = "SINGLE" if ch == "'" else "DOUBLE"
        state.has_token = True
        state.has_quote = True
        return i + 1
    if ch == "\\":
        return _scan_normal_backslash(state, i)
    if ch == "#" and not state.has_token:
        while i < state.length and state.command[i] != "\n":
            i += 1
        return i
    if ch in ("\n", ";", "(", ")", "&", "|"):
        state.flush_segment()
        step = 2 if ch in ("&", "|") and i + 1 < state.length and state.command[i + 1] == ch else 1
        return i + step
    if ch.isspace():
        state.flush_token()
        return i + 1
    state.buf.append(ch)
    state.has_token = True
    return i + 1


_SPLIT_HANDLERS = {
    "SINGLE": _scan_single,
    "DOUBLE": _scan_double,
    "NORMAL": _scan_normal,
}


def _split_shell_segments(command: str) -> list[list[Token]]:
    """Tokenize a shell command and split it on ; && || | & ( ) and newlines."""
    state = _SplitState(
        command=command,
        length=len(command),
        segments=[],
        current_seg=[],
        buf=[],
    )
    i = 0
    while i < state.length:
        i = _SPLIT_HANDLERS[state.quote](state, i)
    state.flush_segment()
    return state.segments


@dataclass
class _SubScanState:
    command: str
    length: int
    depth: int = 1
    quote: str = "NORMAL"


def _scan_sub_single(state: _SubScanState, j: int) -> int:
    if state.command[j] == "'":
        state.quote = "NORMAL"
    return j + 1


def _scan_sub_double(state: _SubScanState, j: int) -> int:
    c = state.command[j]
    if c == "\\" and j + 1 < state.length:
        return j + 2
    if c == '"':
        state.quote = "NORMAL"
    return j + 1


def _scan_sub_normal(state: _SubScanState, j: int) -> int:
    c = state.command[j]
    if c == "\\" and j + 1 < state.length:
        return j + 2
    if c == "'":
        state.quote = "SINGLE"
        return j + 1
    if c == '"':
        state.quote = "DOUBLE"
        return j + 1
    if c == "(":
        state.depth += 1
        return j + 1
    if c == ")":
        state.depth -= 1
        return j if state.depth == 0 else j + 1
    return j + 1


_SUB_HANDLERS = {
    "SINGLE": _scan_sub_single,
    "DOUBLE": _scan_sub_double,
    "NORMAL": _scan_sub_normal,
}


def _scan_dollar_paren(command: str, start: int, length: int) -> tuple[str, int]:
    state = _SubScanState(command=command, length=length)
    j = start
    while j < length and state.depth > 0:
        j = _SUB_HANDLERS[state.quote](state, j)
    return command[start:j], j + 1


def _scan_backtick(command: str, start: int, length: int) -> tuple[str, int]:
    j = start
    while j < length:
        if command[j] == "\\" and j + 1 < length:
            j += 2
            continue
        if command[j] == "`":
            break
        j += 1
    return command[start:j], j + 1


def _extract_substitutions(command: str) -> list[str]:
    """Extract inner command strings from $(...) and `...` outside single quotes."""
    subs: list[str] = []
    i = 0
    n = len(command)
    in_double = False

    while i < n:
        ch = command[i]
        if ch == "\\" and i + 1 < n:
            i += 2
        elif not in_double and ch == "'":
            i += 1
            while i < n and command[i] != "'":
                i += 1
            i += 1
        elif ch == '"':
            in_double = not in_double
            i += 1
        elif ch == "$" and i + 1 < n and command[i + 1] == "(":
            inner, i = _scan_dollar_paren(command, i + 2, n)
            subs.append(inner)
        elif ch == "`":
            inner, i = _scan_backtick(command, i + 1, n)
            subs.append(inner)
        else:
            i += 1

    return subs


def _parse_compose_args(args: list[str]) -> tuple[str | None, str | None, bool]:
    cli_project: str | None = None
    compose_subcmd: str | None = None
    has_build_or_recreate = False

    i = 0
    n = len(args)
    while i < n:
        tok = args[i]
        if tok == "--":
            break
        if tok in ("-p", "--project-name"):
            if i + 1 < n:
                cli_project = args[i + 1]
                i += 2
                continue
            i += 1
            continue
        if tok.startswith(("--project-name=", "-p=")):
            cli_project = tok.split("=", 1)[1]
            i += 1
            continue
        if tok in COMPOSE_VALUE_FLAGS:
            i += 2
            continue
        if tok.startswith("-"):
            if tok in ("--build", "--force-recreate"):
                has_build_or_recreate = True
            i += 1
            continue
        if compose_subcmd is None:
            compose_subcmd = tok
        i += 1

    return cli_project, compose_subcmd, has_build_or_recreate


def _classify_compose(args: list[str], env_vars: dict[str, str]) -> str | None:
    """Classify a docker compose / docker-compose argument list into O2, O3, O4, or None."""
    cli_project, compose_subcmd, has_build_or_recreate = _parse_compose_args(args)

    if compose_subcmd not in ("up", "down", "rm", "stop", "kill"):
        return None

    effective_project = (
        cli_project
        if (cli_project is not None and cli_project != "")
        else env_vars.get("COMPOSE_PROJECT_NAME", "")
    )
    if effective_project not in ("", "alt") and "$" not in effective_project:
        return None

    if compose_subcmd in ("stop", "kill"):
        return "O2"
    if compose_subcmd == "up" and has_build_or_recreate:
        return "O4"
    return "O3"


def _first_positional_arg(
    tokens: list[str], start_idx: int, value_flags: frozenset[str]
) -> tuple[str | None, int]:
    i = start_idx
    n = len(tokens)
    while i < n:
        tok = tokens[i]
        if tok == "--":
            break
        if tok in value_flags:
            i += 2
            continue
        if tok.startswith("-"):
            i += 1
            continue
        return tok, i + 1
    return None, n


def _classify_docker_segment(tokens: list[str], env_vars: dict[str, str]) -> str | None:  # noqa: PLR0911 — one return per distinct verdict
    """Classify a docker / docker-compose invocation starting at tokens[0]."""
    exe = PurePosixPath(tokens[0]).name
    if exe == "docker-compose":
        return _classify_compose(tokens[1:], env_vars)
    if exe != "docker":
        return None

    subcmd, next_idx = _first_positional_arg(tokens, 1, DOCKER_VALUE_FLAGS)
    if subcmd is None:
        return None
    if subcmd == "compose":
        return _classify_compose(tokens[next_idx:], env_vars)
    if subcmd == "rmi":
        return "O1"
    if subcmd in ("rm", "stop", "kill"):
        return "O2"

    action, _ = _first_positional_arg(tokens, next_idx, frozenset())

    if subcmd in PRUNE_GROUPS and action == "prune":
        return "O1"
    if subcmd in ("volume", "image", "network") and action in ("rm", "remove"):
        return "O1"
    if subcmd == "container" and action in ("rm", "remove", "stop", "kill"):
        return "O2"
    return None


def _extract_shell_c_script(tokens: list[Token], start_idx: int) -> str | None:
    """If tokens[start_idx] is a shell with a -c / -lc flag, return the script argument."""
    i = start_idx + 1
    n = len(tokens)
    while i < n:
        tok = tokens[i].text
        if tok == "--":
            return None
        if tok.startswith("-") and not tok.startswith("--"):
            if "c" in tok[1:]:
                return tokens[i + 1].text if i + 1 < n else None
            i += 1
            continue
        if tok.startswith("--"):
            i += 1
            continue
        return None
    return None


def _classify_tokens(tokens: list[Token], *, depth: int) -> str | None:
    """Classify a segment or argv token list, scanning all bare docker/shell invocations."""
    env_vars: dict[str, str] = {}
    best_rule: str | None = None
    for idx, tok in enumerate(tokens):
        if ENV_ASSIGN_RE.match(tok.text):
            key, val = tok.text.split("=", 1)
            env_vars[key] = val
            continue
        if tok.quoted:
            continue
        base = PurePosixPath(tok.text).name
        if base in ("docker", "docker-compose"):
            rule = _classify_docker_segment([t.text for t in tokens[idx:]], env_vars)
            best_rule = _pick_higher_rule(best_rule, rule)
        elif base in SHELL_NAMES:
            script = _extract_shell_c_script(tokens, idx)
            if script is not None:
                rule = classify_command(script, depth=depth + 1)
                best_rule = _pick_higher_rule(best_rule, rule)
    return best_rule


def classify_command(command: str, *, depth: int = 0) -> str | None:
    """Return matching rule id (O1..O4) for a shell command string, or None."""
    if depth > MAX_RECURSION_DEPTH:
        msg = "command nesting exceeded maximum recursion depth"
        raise ValueError(msg)
    if not command.strip():
        return None

    best_rule: str | None = None
    for seg in _split_shell_segments(command):
        rule = _classify_tokens(seg, depth=depth)
        best_rule = _pick_higher_rule(best_rule, rule)

    for sub in _extract_substitutions(command):
        rule = classify_command(sub, depth=depth + 1)
        best_rule = _pick_higher_rule(best_rule, rule)

    return best_rule


def classify_argv(argv: list[str]) -> str | None:
    """Classify an already-split argv command list without re-joining or re-splitting."""
    if not argv:
        return None
    if PurePosixPath(argv[0]).name in SHELL_NAMES:
        tokens = [Token(part, quoted=False) for part in argv]
        script = _extract_shell_c_script(tokens, 0)
        if script is not None:
            return classify_command(script, depth=1)
    tokens = [Token(part, quoted=False) for part in argv]
    return _classify_tokens(tokens, depth=0)


def parse_shell_hook_stdin(raw: str) -> str | list[str] | None:
    """Parse PreToolUse stdin for a shell tool.

    Returns None when tool_name is outside WATCHED_SHELL_TOOLS.
    Raises HookPayloadError when stdin is malformed or missing command.
    """
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        msg = f"invalid JSON ({exc.msg})"
        raise HookPayloadError(msg) from exc

    if not isinstance(payload, dict):
        msg = "expected a JSON object"
        raise HookPayloadError(msg)

    tool_name = payload.get("tool_name")
    if not isinstance(tool_name, str) or not tool_name.strip():
        msg = "missing tool_name"
        raise HookPayloadError(msg)

    if tool_name not in WATCHED_SHELL_TOOLS:
        return None

    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        msg = f"missing tool_input for {tool_name}"
        raise HookPayloadError(msg)

    cmd = tool_input.get("command")
    if isinstance(cmd, str):
        return cmd
    if isinstance(cmd, list) and all(isinstance(part, str) for part in cmd):
        return cmd
    msg = f"missing or invalid command for {tool_name}"
    raise HookPayloadError(msg)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="PreToolUse guard for risky Docker and Compose commands."
    )
    parser.add_argument(
        "--on-match",
        choices=("ask", "deny"),
        default="ask",
        help="Permission decision emitted when a command matches or payload is unreadable.",
    )
    args = parser.parse_args(argv)
    on_match: str = args.on_match

    try:
        raw_bytes = sys.stdin.buffer.read()
        raw = raw_bytes.decode("utf-8")
        command = parse_shell_hook_stdin(raw)
        if command is None:
            return 0
        rule = classify_argv(command) if isinstance(command, list) else classify_command(command)
    except Exception:  # noqa: BLE001 — fail-closed hook boundary must emit ask/deny on any parse/classification error
        sys.stdout.write(json.dumps(_decision(on_match, UNREADABLE_REASON)) + "\n")
        return 0

    if rule is not None:
        reason = RULE_REASONS[rule]
        if on_match == "deny":
            reason = f"{reason} {DENY_GUIDANCE_SENTENCE}"
        sys.stdout.write(json.dumps(_decision(on_match, reason)) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
