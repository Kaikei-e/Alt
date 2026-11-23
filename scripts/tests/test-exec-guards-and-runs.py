#!/usr/bin/env python3
"""Tests for EXEC-01, EXEC-02, EXEC-03, LOOP-03, and LOOP-06 harness components.

Run:
    python3 scripts/tests/test-exec-guards-and-runs.py
"""

from __future__ import annotations

import asyncio
import contextlib
import importlib
import io
import json
import os
import pathlib
import shutil
import sys
import tempfile
import unittest.mock
from dataclasses import dataclass

ROOT = pathlib.Path(__file__).resolve().parents[2]
HARNESS_DIR = ROOT / "scripts" / "agent-harness"
CLAUDE_HOOK_SH = ROOT / ".claude" / "hooks" / "deny-unsafe-commands.sh"

if str(HARNESS_DIR) not in sys.path:
    sys.path.insert(0, str(HARNESS_DIR))

GIT_BIN = shutil.which("git")
BASH_BIN = shutil.which("bash")
BLOCKED_RC = 2
INIT_TSV_LINES = 2
PAR_BASE = 2
PAR_UP = 4
PAR_CAP = 8


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        suffix = f" ({detail})" if detail else ""
        sys.stdout.write(f"  FAIL  {name}{suffix}\n")
        RESULTS.failed += 1


@dataclass(frozen=True)
class RunResult:
    returncode: int
    stdout: str
    stderr: str


class _BytesTextStdin:
    """Minimal stdin stub providing both .read() and .buffer.read()."""

    def __init__(self, raw_bytes: bytes) -> None:
        self.buffer = io.BytesIO(raw_bytes)
        self._text = raw_bytes.decode("utf-8", errors="replace")

    def read(self) -> str:
        return self._text


def invoke_main(
    mod: object,
    *,
    argv: list[str] | None = None,
    stdin_text: str | None = None,
    env: dict[str, str] | None = None,
    cwd: pathlib.Path | None = None,
) -> RunResult:
    raw_bytes = stdin_text.encode("utf-8") if stdin_text is not None else b""
    out_buf = io.StringIO()
    err_buf = io.StringIO()
    fake_stdin = _BytesTextStdin(raw_bytes)
    main_fn = mod.main  # type: ignore[attr-defined]

    clean_env = dict(os.environ)
    for key in (
        "ALT_HARNESS_ROLE",
        "ALT_HARNESS_REPO_ROOT",
        "ALT_HARNESS_BRANCH",
        "ALT_ACTIVE_PACKET",
    ):
        clean_env.pop(key, None)
    if env:
        clean_env.update(env)

    orig_cwd = pathlib.Path.cwd()
    rc = 0
    try:
        if cwd is not None:
            os.chdir(cwd)
        with (
            unittest.mock.patch.dict(os.environ, clean_env, clear=True),
            unittest.mock.patch.object(sys, "stdin", fake_stdin),
            contextlib.redirect_stdout(out_buf),
            contextlib.redirect_stderr(err_buf),
        ):
            try:
                rc = main_fn(argv) if argv is not None else main_fn()
            except SystemExit as exc:
                code = exc.code
                rc = code if isinstance(code, int) else (1 if code else 0)
    finally:
        if cwd is not None:
            os.chdir(orig_cwd)
    return RunResult(returncode=rc, stdout=out_buf.getvalue(), stderr=err_buf.getvalue())


async def _exec_bash_script_async(
    script: pathlib.Path,
    stdin_bytes: bytes,
    env: dict[str, str],
    cwd: pathlib.Path,
) -> RunResult:
    assert BASH_BIN is not None
    proc = await asyncio.create_subprocess_exec(
        BASH_BIN,
        str(script),
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        env=env,
        cwd=str(cwd),
    )
    stdout_b, stderr_b = await proc.communicate(stdin_bytes)
    return RunResult(
        returncode=proc.returncode if proc.returncode is not None else 1,
        stdout=stdout_b.decode("utf-8", errors="replace"),
        stderr=stderr_b.decode("utf-8", errors="replace"),
    )


def run_shell_hook(
    script: pathlib.Path,
    stdin_text: str,
    *,
    env: dict[str, str] | None = None,
    cwd: pathlib.Path | None = None,
) -> RunResult:
    merged_env = dict(os.environ)
    for key in (
        "ALT_HARNESS_ROLE",
        "ALT_HARNESS_REPO_ROOT",
        "ALT_HARNESS_BRANCH",
        "ALT_ACTIVE_PACKET",
    ):
        merged_env.pop(key, None)
    if env:
        merged_env.update(env)
    return asyncio.run(
        _exec_bash_script_async(
            script,
            stdin_text.encode("utf-8"),
            merged_env,
            cwd or ROOT,
        )
    )


def bash_payload(
    command: str | list[str],
    *,
    tool_name: str = "Bash",
    agent_id: str | None = None,
    subagent_type: str | None = None,
) -> str:
    payload: dict[str, object] = {
        "hook_event_name": "PreToolUse",
        "tool_name": tool_name,
        "tool_input": {"command": command},
    }
    if agent_id is not None:
        payload["agent_id"] = agent_id
    if subagent_type is not None:
        payload["subagent_type"] = subagent_type
    return json.dumps(payload)


def edit_payload(path: str, tool_name: str = "Edit") -> str:
    if tool_name == "Write":
        tool_input: dict[str, object] = {"file_path": path, "content": "x = 1\n"}
    elif tool_name == "apply_patch":
        tool_input = {
            "command": f"*** Begin Patch\n*** Update File: {path}\n@@\n+x = 1\n*** End Patch\n"
        }
    else:
        tool_input = {"file_path": path, "old_string": "a", "new_string": "b"}
    return json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": tool_name,
            "tool_input": tool_input,
        }
    )


def init_git_repo(task_packet_mod: object, repo: pathlib.Path) -> None:
    assert GIT_BIN is not None
    run_git = task_packet_mod._run_git_bytes  # type: ignore[attr-defined]
    assert run_git(repo, GIT_BIN, "init", "-b", "feat/test-branch").returncode == 0
    assert run_git(repo, GIT_BIN, "config", "user.email", "test@example.com").returncode == 0
    assert run_git(repo, GIT_BIN, "config", "user.name", "Harness Test").returncode == 0


def git_commit_file(
    task_packet_mod: object,
    repo: pathlib.Path,
    rel_path: str,
    content: str,
    message: str,
) -> None:
    assert GIT_BIN is not None
    run_git = task_packet_mod._run_git_bytes  # type: ignore[attr-defined]
    target = repo / rel_path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")
    assert run_git(repo, GIT_BIN, "add", "--", rel_path).returncode == 0
    assert run_git(repo, GIT_BIN, "commit", "-m", message).returncode == 0


def sample_packet_md(
    packet_id: str = "PKT-001",
    *,
    allowed_paths: list[str] | None = None,
    forbidden_paths: list[str] | None = None,
    allow_generated: bool = False,
    forbid_repo_wide: bool = True,
) -> str:
    allowed = allowed_paths or ["scripts/agent-harness/**"]
    forbidden = forbidden_paths or ["scripts/agent-harness/forbidden.py"]
    allowed_yaml = "\n".join(f'    - "{p}"' for p in allowed)
    forbidden_yaml = "\n".join(f'    - "{p}"' for p in forbidden)
    return (
        "---\n"
        'packet_version: "1.0"\n'
        f'packet_id: "{packet_id}"\n'
        'title: "Test packet"\n'
        "scope:\n"
        '  target: "scripts"\n'
        "  allowed_paths:\n"
        f"{allowed_yaml}\n"
        "  forbidden_paths:\n"
        f"{forbidden_yaml}\n"
        f"  allow_generated_code_edits: {'true' if allow_generated else 'false'}\n"
        "  max_files: 5\n"
        "  max_functions: 3\n"
        "  allowed_rules:\n"
        '    - "S101"\n'
        '  stop_condition: "done"\n'
        "execution_policy:\n"
        '  policy: "fix-existing"\n'
        "  foreground_only: true\n"
        f"  forbid_repo_wide_test_build: {'true' if forbid_repo_wide else 'false'}\n"
        "  forbid_bulk_autofix: true\n"
        '  verification_command: "./scripts/lint/run.sh scripts"\n'
        "handoff_requirements:\n"
        "  require_modified_files_and_lines: true\n"
        "  require_runtime_behavior_impact: true\n"
        "  require_change_rationale: true\n"
        "---\n\n"
        f"# Task Packet {packet_id}\n"
    )


def sample_handoff_md(packet_id: str = "PKT-001") -> str:
    return (
        "---\n"
        'packet_version: "1.0"\n'
        f'packet_id: "{packet_id}"\n'
        'status: "completed"\n'
        "modified_files:\n"
        '  - path: "scripts/agent-harness/foo.py"\n'
        "    added_lines: 10\n"
        "    deleted_lines: 2\n"
        "modified_functions_count: 1\n"
        "runtime_behavior_impact:\n"
        "  has_change: false\n"
        "  checked_dimensions:\n"
        '    - "control_flow"\n'
        '  summary: "変更なし"\n'
        'rationale: "test handoff"\n'
        "verification:\n"
        '  command: "./scripts/lint/run.sh scripts"\n'
        "  exit_code: 0\n"
        "---\n\n"
        f"# Hand-off {packet_id}\n"
    )


def test_exec01_commands(
    exec_mod: object, harness_run_mod: object, task_packet_mod: object
) -> None:
    sys.stdout.write("EXEC-01: guard_exec_commands.py E1/E2/E3 and deny-unsafe-commands.sh\n")

    # E1: Bulk autofix blocked for EVERYONE (host and worker)
    e1_blocked_cmds = [
        ("C-01/C-02/C-03 golangci-lint run --fix", "golangci-lint run --fix ./..."),
        ("golangci-lint --fix=true", "golangci-lint --fix=true"),
        ("cargo clippy --fix", "cargo clippy --fix --allow-dirty"),
        ("cargo +nightly clippy --fix", "cargo +nightly clippy --all-targets --fix"),
        ("clippy-driver --fix", "clippy-driver --fix foo.rs"),
        ("cargo fix", "cargo fix --allow-dirty"),
        ("ruff check --unsafe-fixes", "ruff check --unsafe-fixes ."),
        ("uv run ruff check --unsafe-fixes", "uv run --no-sync ruff check --fix --unsafe-fixes ."),
        ("biome check --unsafe", "biome check --write --unsafe src/"),
        ("npx biome lint --unsafe", "npx --no-install biome lint --unsafe src/"),
        ("nested bash -c golangci-lint --fix", "bash -c 'golangci-lint run --fix ./...'"),
        ("subshell $(ruff --unsafe-fixes)", 'echo "$(ruff check --unsafe-fixes .)"'),
        ("chained && cargo fix", "echo ok && cargo fix"),
        ("env + sudo golangci-lint --fix", "GOFLAGS=-v sudo -E golangci-lint run --fix"),
    ]
    for label, cmd in e1_blocked_cmds:
        res = invoke_main(exec_mod, stdin_text=bash_payload(cmd))
        check(
            f"E1 blocks {label}",
            res.returncode == BLOCKED_RC and "BLOCKED [EXEC-01]" in res.stderr,
            f"rc={res.returncode} err={res.stderr!r}",
        )

    # E1 argv form
    res_argv = invoke_main(
        exec_mod,
        stdin_text=bash_payload(["golangci-lint", "run", "--fix", "./..."]),
    )
    check(
        "E1 blocks argv list form",
        res_argv.returncode == BLOCKED_RC and "BLOCKED [EXEC-01]" in res_argv.stderr,
        f"rc={res_argv.returncode} err={res_argv.stderr!r}",
    )

    # E1 allowed benign / safe / quoted commands
    e1_allowed_cmds = [
        ("golangci-lint run (no --fix)", "golangci-lint run ./..."),
        ("golangci-lint --fix=false", "golangci-lint run --fix=false ./..."),
        ("cargo clippy (no --fix)", "cargo clippy --all-targets -- -D warnings"),
        ("ruff check --fix (safe fixes only)", "uv run ruff check --fix ."),
        ("biome check --write (safe only)", "biome check --write src/"),
        ("git commit mentioning golangci-lint --fix", 'git commit -m "ban golangci-lint --fix"'),
        ("echo quoted --unsafe-fixes", 'echo "--unsafe-fixes"'),
        ("rg searching golangci-lint --fix", "rg golangci-lint --fix docs/"),
    ]
    for label, cmd in e1_allowed_cmds:
        res = invoke_main(exec_mod, stdin_text=bash_payload(cmd))
        check(
            f"E1 allows {label}",
            res.returncode == 0 and res.stderr == "",
            f"rc={res.returncode} err={res.stderr!r}",
        )

    # E2 & E3: Host (no agent_id, no subagent_type, no worker env, no active packet) is allowed
    with tempfile.TemporaryDirectory() as tmp:
        repo = pathlib.Path(tmp)
        init_git_repo(task_packet_mod, repo)
        host_env = {"ALT_HARNESS_REPO_ROOT": str(repo)}
        for label, cmd in [
            ("host run.sh all", "./scripts/lint/run.sh all"),
            ("host make lint", "make lint"),
            ("host background &", "sleep 1 &"),
            ("host nohup", "nohup echo hi"),
        ]:
            res = invoke_main(exec_mod, stdin_text=bash_payload(cmd), env=host_env)
            check(
                f"Host allows {label}",
                res.returncode == 0,
                f"rc={res.returncode} err={res.stderr!r}",
            )

        # E2: Worker blocked on repo-wide lint
        worker_e2_blocked = [
            (
                "agent_id ./scripts/lint/run.sh all",
                bash_payload("./scripts/lint/run.sh all", agent_id="w-1"),
                host_env,
            ),
            (
                "agent_id scripts/lint/run.sh all",
                bash_payload("scripts/lint/run.sh all", agent_id="w-1"),
                host_env,
            ),
            (
                "agent_id bash run.sh --policy block-new all",
                bash_payload("bash ./scripts/lint/run.sh --policy block-new all", agent_id="w-1"),
                host_env,
            ),
            ("subagent_type make lint", bash_payload("make lint", subagent_type="impl"), host_env),
            (
                "ALT_HARNESS_ROLE=worker make lint",
                bash_payload("make lint"),
                {**host_env, "ALT_HARNESS_ROLE": "worker"},
            ),
            (
                "worker make lint TARGET=all",
                bash_payload("make lint TARGET=all", agent_id="w-1"),
                host_env,
            ),
        ]
        for label, payload, env in worker_e2_blocked:
            res = invoke_main(exec_mod, stdin_text=payload, env=env)
            check(
                f"E2 blocks {label}",
                res.returncode == BLOCKED_RC and "BLOCKED [EXEC-01]" in res.stderr,
                f"rc={res.returncode} err={res.stderr!r}",
            )

        # E2: Worker allowed on targeted lint
        for label, cmd in [
            ("worker ./scripts/lint/run.sh scripts", "./scripts/lint/run.sh scripts"),
            ("worker make lint TARGET=scripts", "make lint TARGET=scripts"),
            ("worker make test", "make test"),
        ]:
            res = invoke_main(exec_mod, stdin_text=bash_payload(cmd, agent_id="w-1"), env=host_env)
            check(
                f"E2 allows {label}",
                res.returncode == 0,
                f"rc={res.returncode} err={res.stderr!r}",
            )

        # E3: Worker blocked on background & and nohup
        for label, cmd in [
            ("trailing &", "go test ./... &"),
            ("mid-line &", "cmd1 & cmd2"),
            ("2>&1 then trailing &", "go test ./... 2>&1 &"),
            ("nohup prefix", "nohup go test ./..."),
            ("nested bash -c background", "bash -c 'sleep 1 &'"),
        ]:
            res = invoke_main(exec_mod, stdin_text=bash_payload(cmd, agent_id="w-1"), env=host_env)
            check(
                f"E3 blocks worker {label}",
                res.returncode == BLOCKED_RC and "BLOCKED [EXEC-01]" in res.stderr,
                f"rc={res.returncode} err={res.stderr!r}",
            )

        # E3: Worker allowed on logical && / || and redirections / quoted &
        for label, cmd in [
            ("logical &&", "echo a && echo b"),
            ("logical ||", "echo a || echo b"),
            ("fd redirect 2>&1", "go test ./... 2>&1"),
            ("fd redirect 1>&2", "echo err 1>&2"),
            ("fd redirect >&2", "echo err >&2"),
            ("combined redirect &>", "go test ./... &> out.log"),
            ("pipe stderr |&", "go test ./... |& cat"),
            ("double-quoted URL &", 'curl "https://example.com?a=1&b=2"'),
            ("single-quoted &", "echo 'a & b'"),
            ("escaped ampersand", r"echo a \& b"),
        ]:
            res = invoke_main(exec_mod, stdin_text=bash_payload(cmd, agent_id="w-1"), env=host_env)
            check(
                f"E3 allows worker {label}",
                res.returncode == 0,
                f"rc={res.returncode} err={res.stderr!r}",
            )

        # Active packet triggers E2 & E3 even without agent_id
        pkt_file = repo / "packet.md"
        pkt_file.write_text(sample_packet_md("PKT-EXEC"), encoding="utf-8")
        act_res = invoke_main(
            harness_run_mod,
            argv=["activate-packet", str(pkt_file)],
            env=host_env,
        )
        check("activate-packet succeeds for EXEC-01 test", act_res.returncode == 0, act_res.stderr)

        res_pkt_e2 = invoke_main(
            exec_mod,
            stdin_text=bash_payload("./scripts/lint/run.sh all"),
            env=host_env,
        )
        check(
            "E2 enforced by active-packet.json without agent_id",
            res_pkt_e2.returncode == BLOCKED_RC,
            res_pkt_e2.stderr,
        )
        res_pkt_e3 = invoke_main(
            exec_mod,
            stdin_text=bash_payload("pytest &"),
            env=host_env,
        )
        check(
            "E3 enforced by active-packet.json without agent_id",
            res_pkt_e3.returncode == BLOCKED_RC,
            res_pkt_e3.stderr,
        )

    # Fail-closed & non-shell tools
    for label, raw in [
        ("empty stdin", ""),
        ("invalid JSON", "{bad"),
        ("missing tool_name", json.dumps({"tool_input": {"command": "echo hi"}})),
        ("missing tool_input", json.dumps({"tool_name": "Bash"})),
        ("missing command", json.dumps({"tool_name": "Bash", "tool_input": {}})),
    ]:
        res = invoke_main(exec_mod, stdin_text=raw)
        check(
            f"EXEC-01 fail-closed on {label}",
            res.returncode == BLOCKED_RC,
            f"rc={res.returncode} err={res.stderr!r}",
        )

    res_non_shell = invoke_main(
        exec_mod,
        stdin_text=json.dumps({"tool_name": "Read", "tool_input": {"file_path": "foo.go"}}),
    )
    check("EXEC-01 allows non-shell tool Read", res_non_shell.returncode == 0, res_non_shell.stderr)

    # Thin shell wrapper .claude/hooks/deny-unsafe-commands.sh
    check(
        ".claude/hooks/deny-unsafe-commands.sh exists and is executable",
        CLAUDE_HOOK_SH.is_file() and os.access(CLAUDE_HOOK_SH, os.X_OK),
        str(CLAUDE_HOOK_SH),
    )
    if CLAUDE_HOOK_SH.is_file() and BASH_BIN is not None:
        sh_block = run_shell_hook(CLAUDE_HOOK_SH, bash_payload("golangci-lint run --fix ./..."))
        check(
            "deny-unsafe-commands.sh blocks golangci-lint --fix with exit 2",
            sh_block.returncode == BLOCKED_RC and "BLOCKED [EXEC-01]" in sh_block.stderr,
            f"rc={sh_block.returncode} err={sh_block.stderr!r}",
        )
        sh_ok = run_shell_hook(CLAUDE_HOOK_SH, bash_payload("golangci-lint run ./..."))
        check(
            "deny-unsafe-commands.sh allows clean golangci-lint run",
            sh_ok.returncode == 0,
            f"rc={sh_ok.returncode} err={sh_ok.stderr!r}",
        )


def _verify_exec02_active_packet_cases(
    scope_mod: object,
    harness_run_mod: object,
    repo: pathlib.Path,
    outside: pathlib.Path,
    env: dict[str, str],
) -> None:
    pkt_file = repo / "pkt.md"
    pkt_file.write_text(
        sample_packet_md(
            "PKT-SCOPE",
            allowed_paths=["scripts/agent-harness/**"],
            forbidden_paths=["scripts/agent-harness/forbidden.py"],
        ),
        encoding="utf-8",
    )
    assert (
        invoke_main(harness_run_mod, argv=["activate-packet", str(pkt_file)], env=env).returncode
        == 0
    )

    res_ok = invoke_main(
        scope_mod,
        stdin_text=edit_payload("scripts/agent-harness/allowed.py"),
        env=env,
    )
    check(
        "EXEC-02 allows file inside active packet allowed_paths",
        res_ok.returncode == 0,
        res_ok.stderr,
    )

    res_forbid = invoke_main(
        scope_mod,
        stdin_text=edit_payload("scripts/agent-harness/forbidden.py"),
        env=env,
    )
    check(
        "EXEC-02 blocks file matching forbidden_paths",
        res_forbid.returncode == BLOCKED_RC and "forbidden_paths" in res_forbid.stderr,
        res_forbid.stderr,
    )

    res_outside_pkt = invoke_main(
        scope_mod,
        stdin_text=edit_payload("alt-backend/app/main.go"),
        env=env,
    )
    check(
        "EXEC-02 blocks repo file outside allowed_paths",
        res_outside_pkt.returncode == BLOCKED_RC and "allowed_paths" in res_outside_pkt.stderr,
        res_outside_pkt.stderr,
    )

    res_out_with_pkt = invoke_main(
        scope_mod,
        stdin_text=edit_payload(str(outside)),
        env=env,
    )
    check(
        "EXEC-02 blocks outside-repo path when active packet exists",
        res_out_with_pkt.returncode == BLOCKED_RC,
        res_out_with_pkt.stderr,
    )

    pkt_optin = repo / "pkt-optin.md"
    pkt_optin.write_text(
        sample_packet_md(
            "PKT-OPTIN",
            allowed_paths=["rag-orchestrator/openapi/**", "stopped-using-k8s/**"],
            forbidden_paths=["scripts/lint/run.sh"],
            allow_generated=True,
        ),
        encoding="utf-8",
    )
    assert (
        invoke_main(harness_run_mod, argv=["activate-packet", str(pkt_optin)], env=env).returncode
        == 0
    )

    res_gen_optin = invoke_main(
        scope_mod,
        stdin_text=edit_payload("rag-orchestrator/openapi/server.gen.go"),
        env=env,
    )
    check(
        "EXEC-02 allows DO NOT EDIT when active packet sets allow_generated_code_edits: true",
        res_gen_optin.returncode == 0,
        res_gen_optin.stderr,
    )
    res_k8s_optin = invoke_main(
        scope_mod,
        stdin_text=edit_payload("stopped-using-k8s/deploy.yaml"),
        env=env,
    )
    check(
        "EXEC-02 allows stopped-using-k8s/ when explicitly in allowed_paths",
        res_k8s_optin.returncode == 0,
        res_k8s_optin.stderr,
    )

    res_read = invoke_main(
        scope_mod,
        stdin_text=json.dumps(
            {"tool_name": "Read", "tool_input": {"file_path": "stopped-using-k8s/deploy.yaml"}}
        ),
        env=env,
    )
    check("EXEC-02 allows non-edit tool Read", res_read.returncode == 0, res_read.stderr)

    res_bad = invoke_main(scope_mod, stdin_text="{not-json", env=env)
    check("EXEC-02 fail-closed on malformed JSON", res_bad.returncode == BLOCKED_RC, res_bad.stderr)


def test_exec02_scope(scope_mod: object, harness_run_mod: object, task_packet_mod: object) -> None:
    sys.stdout.write(
        "EXEC-02: guard_packet_scope.py DO NOT EDIT, stopped-using-k8s, packet scope\n"
    )

    with tempfile.TemporaryDirectory() as tmp:
        repo = pathlib.Path(tmp) / "repo"
        outside = pathlib.Path(tmp) / "outside.txt"
        outside.write_text("outside\n", encoding="utf-8")
        repo.mkdir()
        init_git_repo(task_packet_mod, repo)
        env = {"ALT_HARNESS_REPO_ROOT": str(repo)}

        git_commit_file(
            task_packet_mod,
            repo,
            "rag-orchestrator/openapi/server.gen.go",
            "// Code generated by oapi-codegen v2.7.0 DO NOT EDIT.\npackage openapi\n",
            "add generated server",
        )
        git_commit_file(
            task_packet_mod,
            repo,
            "scripts/agent-harness/allowed.py",
            "x = 1\n",
            "add normal file",
        )
        git_commit_file(
            task_packet_mod,
            repo,
            "stopped-using-k8s/deploy.yaml",
            "kind: Deployment\n",
            "add historical k8s",
        )
        git_commit_file(
            task_packet_mod,
            repo,
            "scripts/harness/corpus/C-16/after.go",
            "// Code generated by oapi-codegen DO NOT EDIT.\npackage openapi\n",
            "add corpus fixture",
        )

        for tool in ("Edit", "Write", "apply_patch"):
            res = invoke_main(
                scope_mod,
                stdin_text=edit_payload("rag-orchestrator/openapi/server.gen.go", tool_name=tool),
                env=env,
            )
            check(
                f"EXEC-02 blocks DO NOT EDIT via {tool}",
                res.returncode == BLOCKED_RC and "DO NOT EDIT" in res.stderr,
                f"rc={res.returncode} err={res.stderr!r}",
            )

        gen_disk = repo / "rag-orchestrator/openapi/server.gen.go"
        gen_disk.write_text("package openapi\n// stripped header\n", encoding="utf-8")
        res_head = invoke_main(
            scope_mod,
            stdin_text=edit_payload("rag-orchestrator/openapi/server.gen.go"),
            env=env,
        )
        check(
            "EXEC-02 blocks DO NOT EDIT even if header stripped on disk (HEAD check)",
            res_head.returncode == BLOCKED_RC and "DO NOT EDIT" in res_head.stderr,
            f"rc={res_head.returncode} err={res_head.stderr!r}",
        )
        gen_disk.write_text(
            "// Code generated by oapi-codegen v2.7.0 DO NOT EDIT.\npackage openapi\n",
            encoding="utf-8",
        )

        res_corpus = invoke_main(
            scope_mod,
            stdin_text=edit_payload("scripts/harness/corpus/C-16/after.go"),
            env=env,
        )
        check(
            "EXEC-02 allows scripts/harness/corpus/ fixture",
            res_corpus.returncode == 0,
            res_corpus.stderr,
        )

        res_k8s = invoke_main(
            scope_mod,
            stdin_text=edit_payload("stopped-using-k8s/deploy.yaml"),
            env=env,
        )
        check(
            "EXEC-02 blocks stopped-using-k8s/ without active packet",
            res_k8s.returncode == BLOCKED_RC and "stopped-using-k8s" in res_k8s.stderr,
            f"rc={res_k8s.returncode} err={res_k8s.stderr!r}",
        )

        res_out_host = invoke_main(scope_mod, stdin_text=edit_payload(str(outside)), env=env)
        check(
            "EXEC-02 allows outside-repo path when no active packet",
            res_out_host.returncode == 0,
            res_out_host.stderr,
        )

        esc_link = repo / "scripts" / "escape_link.txt"
        esc_link.symlink_to(outside)
        res_sym = invoke_main(
            scope_mod, stdin_text=edit_payload("scripts/escape_link.txt"), env=env
        )
        check(
            "EXEC-02 blocks symlink escaping repo root",
            res_sym.returncode == BLOCKED_RC,
            f"rc={res_sym.returncode} err={res_sym.stderr!r}",
        )

        _verify_exec02_active_packet_cases(scope_mod, harness_run_mod, repo, outside, env)


def _verify_subagent_and_parallelism(
    harness_run_mod: object,
    repo: pathlib.Path,
    run_dir: pathlib.Path,
    env: dict[str, str],
) -> None:
    transcript_ref = str(repo / "transcript.jsonl")
    sub_payload = json.dumps(
        {
            "hook_event_name": "SubagentStop",
            "session_id": "sess-1",
            "agent_id": "worker-9",
            "agent_transcript_path": transcript_ref,
            "last_assistant_message": "All 12 tests passed.",
        }
    )
    res_sub = invoke_main(harness_run_mod, argv=["subagent-stop"], stdin_text=sub_payload, env=env)
    check("subagent-stop records JSONL and exits 0", res_sub.returncode == 0, res_sub.stderr)
    sub_lines = (run_dir / "subagents.jsonl").read_text(encoding="utf-8").splitlines()
    check(
        "subagents.jsonl contains agent_id and transcript path",
        len(sub_lines) == 1
        and json.loads(sub_lines[0]).get("agent_id") == "worker-9"
        and json.loads(sub_lines[0]).get("agent_transcript_path") == transcript_ref,
        repr(sub_lines),
    )

    res_sub_empty = invoke_main(harness_run_mod, argv=["subagent-stop"], stdin_text="", env=env)
    res_sub_bad = invoke_main(harness_run_mod, argv=["subagent-stop"], stdin_text="{bad", env=env)
    check(
        "subagent-stop exits 0 gracefully on empty or invalid JSON",
        res_sub_empty.returncode == 0 and res_sub_bad.returncode == 0,
    )

    res_status_txt = invoke_main(harness_run_mod, argv=["status"], env=env)
    check(
        "status text output succeeds and mentions parallelism",
        res_status_txt.returncode == 0 and "parallelism" in res_status_txt.stdout.lower(),
        res_status_txt.stdout,
    )

    for idx in range(2, 11):
        res_l = invoke_main(
            harness_run_mod,
            argv=[
                "log",
                "--phase",
                "handoff",
                "--decision",
                f"handoff PKT-{idx:02d}: adopted",
                "--why",
                "clean pass",
                "--evidence",
                f".harness/runs/feat-test-branch/handoffs/PKT-{idx:02d}.md",
                "--result",
                "adopted",
            ],
            env=env,
        )
        assert res_l.returncode == 0

    res_st_up = invoke_main(harness_run_mod, argv=["status", "--format", "json"], env=env)
    st_up = json.loads(res_st_up.stdout)
    check(
        "LOOP-06 scales up to 4 on 10 clean outcomes",
        res_st_up.returncode == 0 and st_up["current_run"]["parallelism"]["recommended"] == PAR_UP,
        res_st_up.stdout,
    )

    calc = harness_run_mod.compute_recommended_parallelism  # type: ignore[attr-defined]
    check("LOOP-06 empty outcomes stays at base 2", calc([])["recommended"] == PAR_BASE)
    check(
        "LOOP-06 9 adopted + 1 reverted scales +2 -> 4",
        calc(["adopted"] * 9 + ["reverted"])["recommended"] == PAR_UP,
    )
    check("LOOP-06 caps at 8", calc(["adopted"] * 10, base=7)["recommended"] == PAR_CAP)
    check(
        "LOOP-06 3 reverted scales -2 (floor 1)",
        calc(["reverted"] * 3 + ["adopted"] * 7)["recommended"] == 1,
    )
    check(
        "LOOP-06 2 mismatch scales -2 from base 4 -> 2",
        calc(["mismatch"] * 2 + ["adopted"] * 8, base=PAR_UP)["recommended"] == PAR_BASE,
    )
    check(
        "LOOP-06 2 reverted + 0 mismatch holds steady",
        calc(["reverted"] * 2 + ["adopted"] * 8, base=PAR_UP)["recommended"] == PAR_UP,
    )


def test_loop03_exec03_loop06_runs(harness_run_mod: object, task_packet_mod: object) -> None:
    sys.stdout.write(
        "LOOP-03 / EXEC-03 / LOOP-06: harness_run.py durable runs, subagents, parallelism\n"
    )

    with tempfile.TemporaryDirectory() as tmp:
        repo = pathlib.Path(tmp)
        init_git_repo(task_packet_mod, repo)
        env = {"ALT_HARNESS_REPO_ROOT": str(repo)}

        res_init = invoke_main(
            harness_run_mod,
            argv=[
                "init",
                "--policy",
                "block-new",
                "--goal",
                "Harden agent harness",
                "--completion",
                "./scripts/lint/run.sh harness exits 0",
            ],
            env=env,
        )
        check("harness_run init exits 0", res_init.returncode == 0, res_init.stderr)

        run_dir = repo / ".harness" / "runs" / "feat-test-branch"
        for subdir in ("packets", "handoffs", "verify", "evidence"):
            check(f"init created {subdir}/", (run_dir / subdir).is_dir())

        tsv_lines = (run_dir / "decisions.tsv").read_text(encoding="utf-8").splitlines()
        check(
            "init wrote decisions.tsv header + row 1 policy/completion",
            len(tsv_lines) == INIT_TSV_LINES
            and tsv_lines[0] == "ts\tphase\tdecision\twhy\tevidence\tresult"
            and "block-new" in tsv_lines[1]
            and "./scripts/lint/run.sh harness exits 0" in tsv_lines[1],
            repr(tsv_lines),
        )

        res_bad_ev = invoke_main(
            harness_run_mod,
            argv=[
                "log",
                "--phase",
                "review",
                "--decision",
                "approve",
                "--why",
                "checked",
                "--evidence",
                "looks good to me",
                "--result",
                "info",
            ],
            env=env,
        )
        check(
            "log rejects non-pointer prose evidence",
            res_bad_ev.returncode == BLOCKED_RC,
            res_bad_ev.stderr,
        )

        res_bad_tab = invoke_main(
            harness_run_mod,
            argv=[
                "log",
                "--phase",
                "review",
                "--decision",
                "bad\tdecision",
                "--why",
                "reason",
                "--evidence",
                "commit:abc1234",
                "--result",
                "info",
            ],
            env=env,
        )
        check(
            "log rejects tab characters in fields",
            res_bad_tab.returncode == BLOCKED_RC,
            res_bad_tab.stderr,
        )

        res_log = invoke_main(
            harness_run_mod,
            argv=[
                "log",
                "--phase",
                "verify",
                "--decision",
                "green build",
                "--why",
                "all tests passed",
                "--evidence",
                "cmd:./scripts/lint/run.sh scripts",
                "--result",
                "info",
            ],
            env=env,
        )
        check("log accepts concrete cmd: pointer", res_log.returncode == 0, res_log.stderr)

        pkt_path = repo / "pkt-loop.md"
        pkt_path.write_text(sample_packet_md("PKT-LOOP-01"), encoding="utf-8")
        res_act = invoke_main(harness_run_mod, argv=["activate-packet", str(pkt_path)], env=env)
        check("activate-packet exits 0", res_act.returncode == 0, res_act.stderr)
        active_json = json.loads((run_dir / "active-packet.json").read_text(encoding="utf-8"))
        check(
            "active-packet.json records active=true and packet_id",
            active_json.get("active") is True and active_json.get("packet_id") == "PKT-LOOP-01",
            repr(active_json),
        )
        check(
            "packet copied into .harness/runs/<branch>/packets/",
            (run_dir / "packets" / "PKT-LOOP-01.md").is_file(),
        )

        ho_path = repo / "handoff-loop.md"
        ho_path.write_text(sample_handoff_md("PKT-LOOP-01"), encoding="utf-8")
        res_ho = invoke_main(
            harness_run_mod,
            argv=["record-handoff", str(ho_path), "--result", "adopted"],
            env=env,
        )
        check("record-handoff exits 0", res_ho.returncode == 0, res_ho.stderr)
        active_after = json.loads((run_dir / "active-packet.json").read_text(encoding="utf-8"))
        check("record-handoff deactivates active-packet.json", active_after.get("active") is False)
        check("handoff copied into handoffs/", (run_dir / "handoffs" / "PKT-LOOP-01.md").is_file())

        _verify_subagent_and_parallelism(harness_run_mod, repo, run_dir, env)


def main() -> int:
    task_packet_mod = importlib.import_module("task_packet")
    exec_mod = importlib.import_module("guard_exec_commands")
    scope_mod = importlib.import_module("guard_packet_scope")
    harness_run_mod = importlib.import_module("harness_run")

    test_exec01_commands(exec_mod, harness_run_mod, task_packet_mod)
    test_exec02_scope(scope_mod, harness_run_mod, task_packet_mod)
    test_loop03_exec03_loop06_runs(harness_run_mod, task_packet_mod)

    sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
    return 1 if RESULTS.failed else 0


if __name__ == "__main__":
    sys.exit(main())
