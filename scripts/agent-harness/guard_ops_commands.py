#!/usr/bin/env python3
"""PreToolUse guard prompting human confirmation for risky Docker and Compose ops commands."""

from __future__ import annotations

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

PRUNE_GROUPS = frozenset(
    {"system", "image", "container", "volume", "network", "builder", "buildx"}
)

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

RULE_PRIORITY: dict[str, int] = {"O4": 4, "O1": 3, "O2": 2, "O3": 1}
MAX_RECURSION_DEPTH = 32


@dataclass(frozen=True)
class Token:
    text: str
    quoted: bool


def _ask_decision(reason: str) -> dict[str, Any]:
    return {
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "ask",
            "permissionDecisionReason": reason,
        }
    }


def _pick_higher_rule(current: str | None, candidate: str | None) -> str | None:
    if candidate is None:
        return current
    if current is None or RULE_PRIORITY[candidate] > RULE_PRIORITY[current]:
        return candidate
    return current


def _split_shell_segments(command: str) -> list[list[Token]]:
    """Tokenize a shell command and split it on ; && || | & ( ) and newlines."""
    segments: list[list[Token]] = []
    current_seg: list[Token] = []
    buf: list[str] = []
    has_token = False
    has_quote = False
    state = "NORMAL"
    i = 0
    n = len(command)

    def flush_token() -> None:
        nonlocal has_token, has_quote
        if has_token:
            current_seg.append(Token("".join(buf), has_quote))
            buf.clear()
            has_token = False
            has_quote = False

    def flush_segment() -> None:
        flush_token()
        if current_seg:
            segments.append(list(current_seg))
            current_seg.clear()

    while i < n:
        ch = command[i]
        if state == "SINGLE":
            if ch == "'":
                state = "NORMAL"
            else:
                buf.append(ch)
            i += 1
            continue

        if state == "DOUBLE":
            if ch == "\\":
                if i + 1 < n and command[i + 1] == "\n":
                    i += 2
                    continue
                if i + 1 < n:
                    buf.append(command[i + 1])
                    i += 2
                    continue
            elif ch == '"':
                state = "NORMAL"
                i += 1
                continue
            buf.append(ch)
            i += 1
            continue

        # NORMAL state
        if ch == "'":
            state = "SINGLE"
            has_token = True
            has_quote = True
            i += 1
        elif ch == '"':
            state = "DOUBLE"
            has_token = True
            has_quote = True
            i += 1
        elif ch == "\\":
            if i + 1 < n and command[i + 1] == "\n":
                i += 2
            elif i + 1 < n:
                buf.append(command[i + 1])
                has_token = True
                i += 2
            else:
                i += 1
        elif ch == "#" and not has_token:
            while i < n and command[i] != "\n":
                i += 1
        elif ch in ("\n", ";", "(", ")"):
            flush_segment()
            i += 1
        elif ch in ("&", "|"):
            flush_segment()
            if i + 1 < n and command[i + 1] == ch:
                i += 2
            else:
                i += 1
        elif ch.isspace():
            flush_token()
            i += 1
        else:
            buf.append(ch)
            has_token = True
            i += 1

    flush_segment()
    return segments


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
            continue
        if not in_double and ch == "'":
            i += 1
            while i < n and command[i] != "'":
                i += 1
            i += 1
            continue
        if ch == '"':
            in_double = not in_double
            i += 1
            continue
        if ch == "$" and i + 1 < n and command[i + 1] == "(":
            start = i + 2
            j = start
            depth = 1
            sub_state = "NORMAL"
            while j < n and depth > 0:
                c = command[j]
                if sub_state == "SINGLE":
                    if c == "'":
                        sub_state = "NORMAL"
                    j += 1
                    continue
                if sub_state == "DOUBLE":
                    if c == "\\" and j + 1 < n:
                        j += 2
                        continue
                    if c == '"':
                        sub_state = "NORMAL"
                    j += 1
                    continue
                if c == "\\" and j + 1 < n:
                    j += 2
                    continue
                if c == "'":
                    sub_state = "SINGLE"
                elif c == '"':
                    sub_state = "DOUBLE"
                elif c == "(":
                    depth += 1
                elif c == ")":
                    depth -= 1
                    if depth == 0:
                        break
                j += 1
            subs.append(command[start:j])
            i = j + 1
            continue
        if ch == "`":
            start = i + 1
            j = start
            while j < n:
                if command[j] == "\\" and j + 1 < n:
                    j += 2
                    continue
                if command[j] == "`":
                    break
                j += 1
            subs.append(command[start:j])
            i = j + 1
            continue
        i += 1

    return subs


def _classify_compose(args: list[str], env_vars: dict[str, str]) -> str | None:
    """Classify a docker compose / docker-compose argument list into O2, O3, O4, or None."""
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
        if tok.startswith("--project-name="):
            cli_project = tok.split("=", 1)[1]
            i += 1
            continue
        if tok.startswith("-p="):
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


def _classify_docker_segment(tokens: list[str], env_vars: dict[str, str]) -> str | None:
    """Classify a docker / docker-compose invocation starting at tokens[0]."""
    exe = PurePosixPath(tokens[0]).name
    if exe == "docker-compose":
        return _classify_compose(tokens[1:], env_vars)
    if exe != "docker":
        return None

    i = 1
    n = len(tokens)
    subcmd: str | None = None
    while i < n:
        tok = tokens[i]
        if tok == "--":
            break
        if tok in DOCKER_VALUE_FLAGS:
            i += 2
            continue
        if tok.startswith("-"):
            i += 1
            continue
        subcmd = tok
        i += 1
        break

    if subcmd is None:
        return None
    if subcmd == "compose":
        return _classify_compose(tokens[i:], env_vars)
    if subcmd == "rmi":
        return "O1"
    if subcmd in ("rm", "stop", "kill"):
        return "O2"

    action: str | None = None
    for tok in tokens[i:]:
        if tok == "--":
            break
        if tok.startswith("-"):
            continue
        action = tok
        break

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
        raise ValueError("command nesting exceeded maximum recursion depth")
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
        raise HookPayloadError(f"invalid JSON ({exc.msg})") from exc

    if not isinstance(payload, dict):
        raise HookPayloadError("expected a JSON object")

    tool_name = payload.get("tool_name")
    if not isinstance(tool_name, str) or not tool_name.strip():
        raise HookPayloadError("missing tool_name")

    if tool_name not in WATCHED_SHELL_TOOLS:
        return None

    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        raise HookPayloadError(f"missing tool_input for {tool_name}")

    cmd = tool_input.get("command")
    if isinstance(cmd, str):
        return cmd
    if isinstance(cmd, list) and all(isinstance(part, str) for part in cmd):
        return cmd
    raise HookPayloadError(f"missing or invalid command for {tool_name}")


def main() -> int:
    try:
        raw_bytes = sys.stdin.buffer.read()
        raw = raw_bytes.decode("utf-8")
        command = parse_shell_hook_stdin(raw)
        if command is None:
            return 0
        if isinstance(command, list):
            rule = classify_argv(command)
        else:
            rule = classify_command(command)
    except (HookPayloadError, UnicodeDecodeError, RecursionError, Exception):
        print(json.dumps(_ask_decision(UNREADABLE_REASON)))
        return 0

    if rule is not None:
        print(json.dumps(_ask_decision(RULE_REASONS[rule])))
    return 0


if __name__ == "__main__":
    sys.exit(main())
