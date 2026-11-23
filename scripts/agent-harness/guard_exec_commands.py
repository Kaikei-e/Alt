#!/usr/bin/env python3
"""PreToolUse guard blocking unsafe bulk autofixes (E1) and worker repo-wide/background commands (E2/E3)."""

from __future__ import annotations

import importlib
import json
import os
import sys
from pathlib import Path, PurePosixPath
from typing import Any

_SELF_DIR = str(Path(__file__).resolve().parent)
if _SELF_DIR not in sys.path:
    sys.path.insert(0, _SELF_DIR)

_ops = importlib.import_module("guard_ops_commands")
_harness_run = importlib.import_module("harness_run")

WATCHED_SHELL_TOOLS = frozenset({"Bash", "shell"})
BLOCKED_RC = 2
MAX_RECURSION_DEPTH = 32

CONTROL_KEYWORDS = frozenset({"if", "then", "elif", "else", "do", "while", "until", "!", "{"})
SIMPLE_WRAPPERS = frozenset({"time", "command", "exec", "builtin"})
RUN_SH_PARTS: tuple[str, str, str] = ("scripts", "lint", "run.sh")
RUN_SH_MIN_PARTS = 3

WRAPPER_VALUE_FLAGS: dict[str, frozenset[str]] = {
    "env": frozenset({"-u", "--unset", "-C", "--chdir", "-S", "--split-string"}),
    "sudo": frozenset({"-u", "-g", "-C", "-h", "-p", "-r", "-t", "-T", "-U", "--user", "--group"}),
    "timeout": frozenset({"-k", "--kill-after", "-s", "--signal"}),
    "nice": frozenset({"-n", "--adjustment"}),
    "xargs": frozenset(
        {
            "-a",
            "-d",
            "-E",
            "-I",
            "-L",
            "-n",
            "-P",
            "-s",
            "--arg-file",
            "--delimiter",
            "--eof",
            "--replace",
            "--max-lines",
            "--max-args",
            "--max-procs",
            "--max-chars",
        }
    ),
    "npx": frozenset({"-p", "--package"}),
    "bunx": frozenset({"-p", "--package"}),
    "uv": frozenset(
        {
            "-p",
            "-w",
            "--project",
            "--directory",
            "--python",
            "--with",
            "--from",
            "--group",
            "--extra",
            "--package",
        }
    ),
}

CARGO_VALUE_FLAGS = frozenset(
    {"-C", "--manifest-path", "--target-dir", "--config", "--color", "-Z"}
)
MAKE_VALUE_FLAGS = frozenset(
    {"-C", "-f", "-I", "-j", "-l", "-o", "-W", "--directory", "--file", "--makefile", "--jobs"}
)
FALSE_BOOL_STRINGS = frozenset({"false", "0", "no", "off"})

RULE_PRIORITY: dict[str, int] = {"E1": 3, "E2": 2, "E3": 1}


def _pick_higher(
    current: tuple[str, str] | None,
    candidate: tuple[str, str] | None,
) -> tuple[str, str] | None:
    if candidate is None:
        return current
    if current is None or RULE_PRIORITY[candidate[0]] > RULE_PRIORITY[current[0]]:
        return candidate
    return current


def _is_enabled_flag(arg: str, flag_name: str) -> bool:
    if arg == flag_name:
        return True
    prefix = f"{flag_name}="
    if arg.startswith(prefix):
        val = arg[len(prefix) :].strip().lower()
        return val not in FALSE_BOOL_STRINGS
    return False


def _step_quoted(command: str, i: int, quote: str) -> tuple[int, str]:
    ch = command[i]
    if quote == "SINGLE":
        return i + 1, ("NORMAL" if ch == "'" else "SINGLE")
    if ch == "\\" and i + 1 < len(command):
        return i + 2, "DOUBLE"
    return i + 1, ("NORMAL" if ch == '"' else "DOUBLE")


def _has_background_ampersand(command: str) -> bool:
    """Return True if command contains an unquoted shell background '&'."""
    n = len(command)
    i = 0
    quote = "NORMAL"

    while i < n:
        if quote != "NORMAL":
            i, quote = _step_quoted(command, i, quote)
            continue
        ch = command[i]
        if ch == "\\" and i + 1 < n:
            i += 2
        elif ch in ("'", '"'):
            quote = "SINGLE" if ch == "'" else "DOUBLE"
            i += 1
        elif ch == "#" and (i == 0 or command[i - 1].isspace() or command[i - 1] in ";|&()"):
            nl = command.find("\n", i)
            i = n if nl == -1 else nl + 1
        elif ch == "&":
            prev_ch = command[i - 1] if i > 0 else ""
            next_ch = command[i + 1] if i + 1 < n else ""
            if next_ch in ("&", ">"):
                i += 2
            elif prev_ch in (">", "<", "|"):
                i += 1
            else:
                return True
        else:
            i += 1
    return False


def _skip_wrapper_flags(
    tokens: list[Any],
    start: int,
    value_flags: frozenset[str],
    *,
    skip_one_positional: bool = False,
) -> int:
    idx = start
    n = len(tokens)
    while idx < n:
        text = tokens[idx].text
        if text == "--":
            idx += 1
            break
        if text in value_flags:
            idx += 2
            continue
        if text.startswith("-"):
            idx += 1
            continue
        break
    if skip_one_positional and idx < n:
        idx += 1
    return idx


def _unwrap_python_module(tokens: list[Any], idx: int) -> int | None:
    n = len(tokens)
    j = idx + 1
    while j < n:
        tok = tokens[j].text
        if tok == "-m" and j + 1 < n:
            return j + 1
        if tok.startswith("-"):
            j += 1
            continue
        return None
    return None


def _unwrap_uv_or_bun(base: str, tokens: list[Any], idx: int) -> int | None:
    n = len(tokens)
    if base == "uv":
        j = _skip_wrapper_flags(tokens, idx + 1, WRAPPER_VALUE_FLAGS["uv"])
        if j < n and tokens[j].text == "run":
            return _skip_wrapper_flags(tokens, j + 1, WRAPPER_VALUE_FLAGS["uv"])
        return None
    if base == "bun" and idx + 1 < n and tokens[idx + 1].text in ("x", "exec"):
        return idx + 2
    return None


def _unwrap_command_prefix(tokens: list[Any]) -> tuple[list[Any], dict[str, str], bool]:
    """Strip env assignments, shell keywords, and command wrappers.

    Returns (remaining_tokens, env_vars, saw_nohup).
    """
    env_vars: dict[str, str] = {}
    saw_nohup = False
    idx = 0
    n = len(tokens)

    while idx < n:
        tok = tokens[idx]
        if _ops.ENV_ASSIGN_RE.match(tok.text):
            key, val = tok.text.split("=", 1)
            env_vars[key] = val
            idx += 1
            continue
        if tok.quoted:
            break
        base = PurePosixPath(tok.text).name
        if base in CONTROL_KEYWORDS:
            idx += 1
            continue
        if base == "nohup":
            saw_nohup = True
            idx += 1
            continue
        if base in SIMPLE_WRAPPERS:
            idx = _skip_wrapper_flags(tokens, idx + 1, frozenset())
            continue
        if base in ("env", "sudo", "nice", "xargs", "npx", "bunx", "timeout"):
            idx = _skip_wrapper_flags(
                tokens,
                idx + 1,
                WRAPPER_VALUE_FLAGS[base],
                skip_one_positional=(base == "timeout"),
            )
            continue
        if base in ("uv", "bun"):
            next_idx = _unwrap_uv_or_bun(base, tokens, idx)
            if next_idx is not None:
                idx = next_idx
                continue
            break
        if base in ("python", "python3"):
            mod_idx = _unwrap_python_module(tokens, idx)
            if mod_idx is not None:
                idx = mod_idx
                continue
            break
        break

    return tokens[idx:], env_vars, saw_nohup


def _check_cargo_e1(args: list[str]) -> str | None:
    i = 0
    n = len(args)
    subcmd: str | None = None
    while i < n:
        tok = args[i]
        if tok.startswith("+") and subcmd is None:
            i += 1
            continue
        if tok in CARGO_VALUE_FLAGS and subcmd is None:
            i += 2
            continue
        if tok.startswith("-") and subcmd is None:
            i += 1
            continue
        subcmd = tok
        i += 1
        break

    if subcmd == "fix":
        return "cargo fix"
    if subcmd == "clippy" and any(_is_enabled_flag(a, "--fix") for a in args[i:]):
        return "cargo clippy --fix"
    return None


E1_FLAG_RULES: dict[str, tuple[str, str]] = {
    "golangci-lint": ("--fix", "golangci-lint --fix"),
    "clippy-driver": ("--fix", "clippy-driver --fix"),
    "ruff": ("--unsafe-fixes", "ruff --unsafe-fixes"),
    "biome": ("--unsafe", "biome --unsafe"),
}


def _check_e1_command(cmd_tokens: list[str]) -> str | None:
    """Return bulk-autofix label if cmd_tokens invokes a forbidden autofix (Rule E1)."""
    if not cmd_tokens:
        return None
    base = PurePosixPath(cmd_tokens[0]).name
    args = cmd_tokens[1:]
    if base == "cargo":
        return _check_cargo_e1(args)
    spec = E1_FLAG_RULES.get(base)
    if spec is not None and any(_is_enabled_flag(a, spec[0]) for a in args):
        return spec[1]
    return None


def _is_run_sh_path(token_text: str) -> bool:
    parts = PurePosixPath(token_text).parts
    return len(parts) >= RUN_SH_MIN_PARTS and parts[-RUN_SH_MIN_PARTS:] == RUN_SH_PARTS


def _check_run_sh_all(args: list[str]) -> str | None:
    i = 0
    n = len(args)
    while i < n:
        tok = args[i]
        if tok == "--policy":
            i += 2
            continue
        if tok.startswith("--policy="):
            i += 1
            continue
        if tok == "--":
            i += 1
            break
        if tok.startswith("-"):
            i += 1
            continue
        return "scripts/lint/run.sh all" if tok == "all" else None
    if i < n and args[i] == "all":
        return "scripts/lint/run.sh all"
    return None


def _check_make_lint(args: list[str], env_vars: dict[str, str]) -> str | None:
    targets: list[str] = []
    target_var: str | None = env_vars.get("TARGET")
    i = 0
    n = len(args)
    while i < n:
        tok = args[i]
        if tok in MAKE_VALUE_FLAGS:
            i += 2
            continue
        if tok.startswith("-"):
            i += 1
            continue
        if "=" in tok and not tok.startswith("="):
            key, val = tok.split("=", 1)
            if key == "TARGET":
                target_var = val
            i += 1
            continue
        targets.append(tok)
        i += 1

    if "lint" not in targets:
        return None
    if target_var is None or not target_var.strip() or target_var.strip() == "all":
        return "make lint"
    return None


def _check_e2_command(cmd_tokens: list[str], env_vars: dict[str, str]) -> str | None:
    """Return repo-wide lint label if cmd_tokens violates Rule E2."""
    if not cmd_tokens:
        return None
    if _is_run_sh_path(cmd_tokens[0]):
        return _check_run_sh_all(cmd_tokens[1:])
    base = PurePosixPath(cmd_tokens[0]).name
    if base == "make":
        return _check_make_lint(cmd_tokens[1:], env_vars)
    if base in _ops.SHELL_NAMES:
        i = 1
        while i < len(cmd_tokens) and cmd_tokens[i].startswith("-"):
            i += 1
        if i < len(cmd_tokens) and _is_run_sh_path(cmd_tokens[i]):
            return _check_run_sh_all(cmd_tokens[i + 1 :])
    return None


def _classify_segment(
    seg: list[Any],
    *,
    enforce_e2: bool,
    enforce_e3: bool,
    depth: int,
) -> tuple[str, str] | None:
    remaining, env_vars, saw_nohup = _unwrap_command_prefix(seg)
    best: tuple[str, str] | None = None
    if enforce_e3 and saw_nohup:
        best = _pick_higher(best, ("E3", "nohup"))

    if not remaining or remaining[0].quoted:
        return best

    cmd_texts = [t.text for t in remaining]
    e1 = _check_e1_command(cmd_texts)
    if e1 is not None:
        best = _pick_higher(best, ("E1", e1))

    if enforce_e2:
        e2 = _check_e2_command(cmd_texts, env_vars)
        if e2 is not None:
            best = _pick_higher(best, ("E2", e2))

    base = PurePosixPath(remaining[0].text).name
    if base in _ops.SHELL_NAMES:
        script = _ops._extract_shell_c_script(remaining, 0)
        if script is not None:
            nested = classify_command(
                script,
                enforce_e2=enforce_e2,
                enforce_e3=enforce_e3,
                depth=depth + 1,
            )
            best = _pick_higher(best, nested)

    return best


def classify_command(
    command: str,
    *,
    enforce_e2: bool,
    enforce_e3: bool,
    depth: int = 0,
) -> tuple[str, str] | None:
    """Return (rule_id, detail) if command violates E1, E2, or E3, else None."""
    if depth > MAX_RECURSION_DEPTH:
        msg = "command nesting exceeded maximum recursion depth"
        raise ValueError(msg)
    if not command.strip():
        return None

    best: tuple[str, str] | None = None
    if enforce_e3 and _has_background_ampersand(command):
        best = _pick_higher(best, ("E3", "background &"))

    for seg in _ops._split_shell_segments(command):
        best = _pick_higher(
            best,
            _classify_segment(seg, enforce_e2=enforce_e2, enforce_e3=enforce_e3, depth=depth),
        )

    for sub in _ops._extract_substitutions(command):
        best = _pick_higher(
            best,
            classify_command(
                sub,
                enforce_e2=enforce_e2,
                enforce_e3=enforce_e3,
                depth=depth + 1,
            ),
        )

    return best


def classify_argv(
    argv: list[str],
    *,
    enforce_e2: bool,
    enforce_e3: bool,
) -> tuple[str, str] | None:
    if not argv:
        return None
    tokens = [_ops.Token(part, quoted=False) for part in argv]
    return _classify_segment(tokens, enforce_e2=enforce_e2, enforce_e3=enforce_e3, depth=0)


def _non_empty_str(val: object) -> bool:
    return isinstance(val, str) and bool(val.strip())


def parse_exec_hook_stdin(raw_bytes: bytes) -> tuple[str | list[str] | None, bool]:
    """Parse PreToolUse stdin bytes into (command_or_None, is_worker_identity)."""
    try:
        raw = raw_bytes.decode("utf-8")
    except UnicodeDecodeError as exc:
        msg = f"invalid UTF-8 stdin ({exc})"
        raise ValueError(msg) from exc

    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        msg = f"invalid JSON ({exc.msg})"
        raise ValueError(msg) from exc

    if not isinstance(payload, dict):
        msg = "expected a JSON object"
        raise TypeError(msg)

    tool_name = payload.get("tool_name")
    if not _non_empty_str(tool_name):
        msg = "missing tool_name"
        raise ValueError(msg)

    if tool_name not in WATCHED_SHELL_TOOLS:
        return None, False

    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        msg = f"missing tool_input for {tool_name}"
        raise TypeError(msg)

    cmd = tool_input.get("command")
    if not isinstance(cmd, str) and not (
        isinstance(cmd, list) and all(isinstance(part, str) for part in cmd)
    ):
        msg = f"missing or invalid command for {tool_name}"
        raise ValueError(msg)

    is_worker = (
        _non_empty_str(payload.get("agent_id"))
        or _non_empty_str(payload.get("subagent_type"))
        or os.environ.get("ALT_HARNESS_ROLE", "").strip().lower() == "worker"
    )
    return cmd, is_worker


def _format_block_message(rule_id: str, detail: str) -> str:
    if rule_id == "E1":
        return (
            f"BLOCKED [EXEC-01]: 一括自動修正 ({detail}) は禁止されています。"
            "対象箇所を手動で修正し ./scripts/lint/run.sh <target> で検証してください。\n"
        )
    if rule_id == "E2":
        return (
            "BLOCKED [EXEC-01]: 全体検査はホストが行います。"
            "./scripts/lint/run.sh <target> を実行してください。\n"
        )
    return "BLOCKED [EXEC-01]: 前景同期で実行してください（背景実行 & / nohup は禁止）。\n"


def main(argv: list[str] | None = None) -> int:
    _ = argv
    raw_bytes = sys.stdin.buffer.read()
    try:
        command, is_worker_identity = parse_exec_hook_stdin(raw_bytes)
    except (TypeError, ValueError) as exc:
        sys.stderr.write(f"BLOCKED [EXEC-01]: hook payload could not be parsed ({exc}).\n")
        return BLOCKED_RC

    if command is None:
        return 0

    repo_root = _harness_run.resolve_repo_root()
    active_packet, pkt_err = _harness_run.load_active_packet(repo_root)
    if pkt_err is not None:
        sys.stderr.write(f"BLOCKED [EXEC-01]: {pkt_err}.\n")
        return BLOCKED_RC

    enforce_e2 = is_worker_identity or bool(
        active_packet and active_packet.get("forbid_repo_wide_test_build", True)
    )
    enforce_e3 = is_worker_identity or bool(
        active_packet and active_packet.get("foreground_only", True)
    )

    try:
        verdict = (
            classify_argv(command, enforce_e2=enforce_e2, enforce_e3=enforce_e3)
            if isinstance(command, list)
            else classify_command(command, enforce_e2=enforce_e2, enforce_e3=enforce_e3)
        )
    except ValueError as exc:
        sys.stderr.write(f"BLOCKED [EXEC-01]: command analysis failed ({exc}).\n")
        return BLOCKED_RC

    if verdict is not None:
        rule_id, detail = verdict
        sys.stderr.write(_format_block_message(rule_id, detail))
        return BLOCKED_RC

    return 0


if __name__ == "__main__":
    sys.exit(main())
