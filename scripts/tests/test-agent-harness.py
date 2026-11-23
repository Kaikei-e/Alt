#!/usr/bin/env python3
"""Tests for scripts/agent-harness/ guards and self-check.

Run:
    python3 scripts/tests/test-agent-harness.py
"""

from __future__ import annotations

import importlib.util
import json
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
import types
import unittest.mock

ROOT = pathlib.Path(__file__).resolve().parents[2]
HARNESS_DIR = ROOT / "scripts" / "agent-harness"
PROTECTED_SCRIPT = HARNESS_DIR / "guard_protected_paths.py"
EVENT_TIME_SCRIPT = HARNESS_DIR / "guard_event_time.py"
OPS_COMMANDS_SCRIPT = HARNESS_DIR / "guard_ops_commands.py"
SELFCHECK_SCRIPT = HARNESS_DIR / "harness_selfcheck.py"

GIT_BIN = shutil.which("git")
assert GIT_BIN is not None, "git executable not found in PATH"

BLOCKED_RC = 2
USAGE_RC = 2


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


def load_module(name: str, path: pathlib.Path) -> types.ModuleType:
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None, f"missing module: {path}"
    assert spec.loader is not None, f"missing module: {path}"
    mod = importlib.util.module_from_spec(spec)
    with unittest.mock.patch.dict(sys.modules, {name: mod}):
        spec.loader.exec_module(mod)
    return mod


def run_script(
    script: pathlib.Path,
    *,
    stdin_text: str | None = None,
    stdin_bytes: bytes | None = None,
    args: list[str] | None = None,
    cwd: pathlib.Path | None = None,
) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(script), *(args or [])]
    if stdin_bytes is not None:
        raw_res = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd,
            input=stdin_bytes,
            capture_output=True,
            cwd=str(cwd) if cwd else None,
            check=False,
        )
        return subprocess.CompletedProcess(
            args=raw_res.args,
            returncode=raw_res.returncode,
            stdout=raw_res.stdout.decode("utf-8", errors="replace"),
            stderr=raw_res.stderr.decode("utf-8", errors="replace"),
        )
    return subprocess.run(  # noqa: S603 - fixed argv built in this test
        cmd,
        input=stdin_text,
        capture_output=True,
        text=True,
        cwd=str(cwd) if cwd else None,
        check=False,
    )


def claude_write(path: str, content: str = "package main\n") -> str:
    return json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "Write",
            "tool_input": {"file_path": path, "content": content},
        }
    )


def claude_edit(path: str, old_string: str = "a", new_string: str = "b") -> str:
    return json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "Edit",
            "tool_input": {
                "file_path": path,
                "old_string": old_string,
                "new_string": new_string,
            },
        }
    )


def claude_notebook(path: str) -> str:
    return json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "NotebookEdit",
            "tool_input": {"notebook_path": path, "new_source": "print(1)"},
        }
    )


def codex_patch(patch_text: str) -> str:
    return json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "apply_patch",
            "tool_input": {"command": patch_text},
        }
    )


def bash_payload(command: str, tool_name: str = "Bash") -> str:
    return json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": tool_name,
            "tool_input": {"command": command},
        }
    )


def git(repo: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    assert GIT_BIN is not None
    return subprocess.run(  # noqa: S603 - fixed argv built in this test
        [GIT_BIN, "-C", str(repo), *args],
        capture_output=True,
        text=True,
        check=True,
    )


def init_git_repo(repo: pathlib.Path) -> pathlib.Path:
    git(repo, "init")
    git(repo, "config", "user.email", "test@example.com")
    git(repo, "config", "user.name", "Harness Test")
    return repo


def write_and_track(repo: pathlib.Path, rel_path: str, content: str = "") -> pathlib.Path:
    target = repo / rel_path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")
    git(repo, "add", rel_path)
    return target


def build_clean_selfcheck_repo(repo: pathlib.Path) -> pathlib.Path:
    init_git_repo(repo)
    write_and_track(
        repo, "scripts/agent-harness/guard_protected_paths.py", "#!/usr/bin/env python3\n"
    )
    write_and_track(repo, "scripts/agent-harness/guard_event_time.py", "#!/usr/bin/env python3\n")
    write_and_track(repo, "scripts/agent-harness/guard_packet_scope.py", "#!/usr/bin/env python3\n")
    write_and_track(repo, "scripts/agent-harness/guard_ops_commands.py", "#!/usr/bin/env python3\n")
    write_and_track(
        repo, "scripts/agent-harness/guard_exec_commands.py", "#!/usr/bin/env python3\n"
    )
    write_and_track(
        repo, ".claude/skills/adr-preflight/scripts/adr_preflight.py", "#!/usr/bin/env python3\n"
    )
    write_and_track(
        repo,
        "knowledge-sovereign/app/usecase/knowledge_home_projector/projector.go",
        "package knowledge_home_projector\n",
    )
    write_and_track(
        repo,
        "knowledge-sovereign/app/usecase/knowledge_trail_projector/projector.go",
        "package knowledge_trail_projector\n",
    )
    write_and_track(repo, "alt-backend/app/main.go", "package main\n")
    write_and_track(repo, "alt-frontend-sv/src/routes/(app)/+page.svelte", "<main />\n")
    write_and_track(
        repo,
        ".claude/rules/paths.md",
        '---\npaths:\n  - "alt-backend/**/*.go"\n  - "alt-frontend-sv/src/routes/(app)/*.{svelte,ts}"\n---\nRule body\n',
    )
    hooks_cfg = {
        "hooks": {
            "PreToolUse": [
                {
                    "matcher": "apply_patch|Edit|Write",
                    "hooks": [
                        {
                            "type": "command",
                            "command": 'python3 "$(git rev-parse --show-toplevel)/scripts/agent-harness/guard_protected_paths.py"',
                            "timeout": 10,
                            "statusMessage": "Checking protected paths",
                        },
                        {
                            "type": "command",
                            "command": 'python3 "$(git rev-parse --show-toplevel)/scripts/agent-harness/guard_event_time.py"',
                            "timeout": 10,
                            "statusMessage": "Checking event time",
                        },
                        {
                            "type": "command",
                            "command": 'python3 "$(git rev-parse --show-toplevel)/scripts/agent-harness/guard_packet_scope.py"',
                            "timeout": 10,
                            "statusMessage": "Checking packet scope",
                        },
                    ],
                },
                {
                    "matcher": "Bash",
                    "hooks": [
                        {
                            "type": "command",
                            "command": 'python3 "$(git rev-parse --show-toplevel)/scripts/agent-harness/guard_ops_commands.py" --on-match deny',
                            "timeout": 10,
                            "statusMessage": "Checking container and compose commands",
                        },
                        {
                            "type": "command",
                            "command": 'python3 "$(git rev-parse --show-toplevel)/scripts/agent-harness/guard_exec_commands.py"',
                            "timeout": 10,
                            "statusMessage": "Checking exec commands",
                        },
                    ],
                },
            ]
        }
    }
    write_and_track(repo, ".codex/hooks.json", json.dumps(hooks_cfg, indent=2) + "\n")

    shared = (
        "# Shared\n<!-- shared:core:start -->\nNever skip verification.\n<!-- shared:core:end -->\n"
    )
    write_and_track(repo, "CLAUDE.md", shared)
    write_and_track(repo, "AGENTS.md", shared)
    write_and_track(repo, ".agents/skills/sample-skill/SKILL.md", "---\nname: sample-skill\n---\n")
    write_and_track(repo, "docs/ADR/000928.md", "# ADR-000928\n")
    write_and_track(repo, "docs/postmortems/PM-2026-031-sample.md", "# PM-2026-031\n")
    write_and_track(
        repo,
        ".claude/skills/failure-pattern-review/references/patterns.md",
        (
            "# 失敗の型（FP-01 〜 FP-02）\n\n"
            "## FP-01 Silent failure\n\n"
            "**機構.** Something fails silently.\n\n"
            "**先例.**\n- ADR-000928 — example precedent.\n\n"
            "**問い.**\n1. Is it checked?\n\n"
            "**確認コマンド.**\n```bash\nrg -n 'foo' <dir>/\n```\n\n"
            "**機械的に見ているもの.** `scripts/agent-harness/guard_event_time.py`\n\n"
            "## FP-02 Contract drift\n\n"
            "**機構.** One side changes.\n\n"
            "**先例.**\n- PM-2026-031 — example precedent.\n\n"
            "**問い.**\n1. Are both sides updated?\n\n"
            "**確認コマンド.**\n```bash\nrg -n 'bar' <dir>/\n```\n\n"
            "**機械的に見ているもの.** `scripts/agent-harness/guard_protected_paths.py`\n"
        ),
    )
    write_and_track(
        repo,
        ".claude/skills/failure-pattern-review/SKILL.md",
        (
            "# Failure Pattern Review\n\n"
            "## 経路表\n\n"
            "| 差分が触るもの | 読む型 |\n"
            "|---|---|\n"
            "| DI | FP-01 |\n"
            "| proto | FP-02 |\n"
        ),
    )
    write_and_track(
        repo,
        "scripts/lint/ast-grep/rules/go-no-raw-http-server.yml",
        "id: go-no-raw-http-server\nlanguage: go\n",
    )
    write_and_track(
        repo,
        "scripts/harness/corpus/C-01/meta.toml",
        'id = "C-01"\n',
    )
    write_and_track(
        repo,
        ".agents/rules/rule-ledger.md",
        (
            "---\n"
            "max_judgment_rules: 1\n"
            "---\n\n"
            "# Rule Ledger\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Projectors must use event time | `scripts/agent-harness/guard_event_time.py` | ADR-000928 |\n"
            "| R-02 | Use canonical HTTP server helper | `ast-grep:go-no-raw-http-server` | C-01 |\n"
            "| R-03 | Follow TDD RED-GREEN-REFACTOR | 判断（プロセス規律） | AGENTS.md |\n"
        ),
    )
    git(repo, "commit", "-m", "initial clean fixture")
    return repo


def write_claude_settings(repo: pathlib.Path, hooks_cfg: dict) -> None:
    cfg_path = repo / ".claude" / "settings.json"
    cfg_path.parent.mkdir(parents=True, exist_ok=True)
    cfg_path.write_text(json.dumps(hooks_cfg, indent=2) + "\n", encoding="utf-8")


# Load modules directly to verify module-level contracts (will fail before implementation).
hook_payload_mod = load_module("hook_payload", HARNESS_DIR / "hook_payload.py")
protected_mod = load_module("guard_protected_paths", PROTECTED_SCRIPT)
event_time_mod = load_module("guard_event_time", EVENT_TIME_SCRIPT)
selfcheck_mod = load_module("harness_selfcheck", SELFCHECK_SCRIPT)

# ---------------------------------------------------------------------------
# 1. Protected paths tests (SPEC 1)
# ---------------------------------------------------------------------------
sys.stdout.write("protected_paths: allow & block rules across Write / Edit / apply_patch\n")

PROTECTED_BLOCK_CASES = [
    ".env",
    "/repo/.env",
    ".env.local",
    "compose/.env.production",
    "irodori-tts/configs/C1.env",
    "certs/server.key",
    "certs/ca.pem",
    "secrets/clickhouse_password.txt",
    "/repo/secrets/token.txt",
    ".kisage/state.json",
    ".git/config",
]
PROTECTED_ALLOW_CASES = [
    ".env.example",
    "compose/.env.example",
    ".env.template",
    ".env.sample",
    "alt-backend/app/config/secrets.go",
    "e2e/fixtures/staging-secrets/x.txt",
    "alt-backend/app/main.go",
]

for path in PROTECTED_BLOCK_CASES:
    res_w = run_script(PROTECTED_SCRIPT, stdin_text=claude_write(path))
    check(
        f"Write blocks {path}",
        res_w.returncode == BLOCKED_RC and path in res_w.stderr,
        res_w.stderr,
    )

    res_e = run_script(PROTECTED_SCRIPT, stdin_text=claude_edit(path))
    check(
        f"Edit blocks {path}", res_e.returncode == BLOCKED_RC and path in res_e.stderr, res_e.stderr
    )

    patch = f"*** Begin Patch\n*** Update File: {path}\n@@\n+x=1\n*** End Patch\n"
    res_p = run_script(PROTECTED_SCRIPT, stdin_text=codex_patch(patch))
    check(
        f"apply_patch blocks {path}",
        res_p.returncode == BLOCKED_RC and path in res_p.stderr,
        res_p.stderr,
    )

for path in PROTECTED_ALLOW_CASES:
    res_w = run_script(PROTECTED_SCRIPT, stdin_text=claude_write(path))
    check(f"Write allows {path}", res_w.returncode == 0, res_w.stderr)

    res_e = run_script(PROTECTED_SCRIPT, stdin_text=claude_edit(path))
    check(f"Edit allows {path}", res_e.returncode == 0, res_e.stderr)

    patch = f"*** Begin Patch\n*** Add File: {path}\n+hello\n*** End Patch\n"
    res_p = run_script(PROTECTED_SCRIPT, stdin_text=codex_patch(patch))
    check(f"apply_patch allows {path}", res_p.returncode == 0, res_p.stderr)

# Multi-file apply_patch (one allowed, one blocked) must block.
mixed_patch = (
    "*** Begin Patch\n"
    "*** Update File: alt-backend/app/main.go\n@@\n+x := 1\n"
    "*** Move to: secrets/leaked.txt\n"
    "*** End Patch\n"
)
res_mixed = run_script(PROTECTED_SCRIPT, stdin_text=codex_patch(mixed_patch))
check("apply_patch Move to secrets/ blocks", res_mixed.returncode == BLOCKED_RC, res_mixed.stderr)

sys.stdout.write("protected_paths: migrations tracked vs untracked\n")
with tempfile.TemporaryDirectory() as tmp:
    repo = init_git_repo(pathlib.Path(tmp))
    tracked_mig = write_and_track(repo, "db/migrations/001_init.sql", "CREATE TABLE t (id INT);\n")
    git(repo, "commit", "-m", "add migration")
    untracked_mig = repo / "db" / "migrations" / "002_new.sql"
    untracked_nonexistent_parent = repo / "new_svc" / "migrations" / "001_init.sql"

    for label, payload in [
        ("Write", claude_write(str(tracked_mig))),
        ("Edit", claude_edit(str(tracked_mig))),
        (
            "apply_patch",
            codex_patch(
                "*** Begin Patch\n*** Update File: db/migrations/001_init.sql\n@@\n+ALTER TABLE t;\n*** End Patch\n"
            ),
        ),
    ]:
        res = run_script(PROTECTED_SCRIPT, stdin_text=payload, cwd=repo)
        check(
            f"{label} blocks tracked migration",
            res.returncode == BLOCKED_RC and "atlas" in res.stderr.lower(),
            res.stderr,
        )

    for label, payload in [
        ("Write", claude_write(str(untracked_mig))),
        ("Edit", claude_edit(str(untracked_mig))),
        (
            "apply_patch",
            codex_patch(
                "*** Begin Patch\n*** Add File: db/migrations/002_new.sql\n+CREATE TABLE u (id INT);\n*** End Patch\n"
            ),
        ),
    ]:
        res = run_script(PROTECTED_SCRIPT, stdin_text=payload, cwd=repo)
        check(f"{label} allows untracked migration", res.returncode == 0, res.stderr)

    res_new_dir = run_script(
        PROTECTED_SCRIPT,
        stdin_text=claude_write(str(untracked_nonexistent_parent)),
        cwd=repo,
    )
    check(
        "Write allows untracked migration in new migrations dir",
        res_new_dir.returncode == 0,
        res_new_dir.stderr,
    )

with tempfile.TemporaryDirectory() as non_git_tmp:
    non_git_mig = pathlib.Path(non_git_tmp) / "db" / "migrations" / "001_init.sql"
    non_git_mig.parent.mkdir(parents=True, exist_ok=True)
    res_nongit = run_script(PROTECTED_SCRIPT, stdin_text=claude_write(str(non_git_mig)))
    check(
        "migration outside git repo blocks fail-closed",
        res_nongit.returncode == BLOCKED_RC,
        res_nongit.stderr,
    )

sys.stdout.write("protected_paths: fail-closed and other tools\n")
FAIL_CLOSED_INPUTS = [
    ("empty stdin", ""),
    ("invalid json", "{bad"),
    ("json array", "[]"),
    ("json scalar", '"hello"'),
    ("empty object", "{}"),
    ("Edit without path", json.dumps({"tool_name": "Edit", "tool_input": {"new_string": "x"}})),
    (
        "Write with blank path",
        json.dumps({"tool_name": "Write", "tool_input": {"file_path": "   "}}),
    ),
    ("NotebookEdit without path", json.dumps({"tool_name": "NotebookEdit", "tool_input": {}})),
    ("apply_patch without envelope", codex_patch("just plain text")),
]
for label, raw in FAIL_CLOSED_INPUTS:
    res = run_script(PROTECTED_SCRIPT, stdin_text=raw)
    check(
        f"protected_paths fail-closed on {label}",
        res.returncode == BLOCKED_RC and "payload" in res.stderr.lower(),
        res.stderr,
    )

res_other = run_script(
    PROTECTED_SCRIPT,
    stdin_text=json.dumps({"tool_name": "Read", "tool_input": {"file_path": ".env"}}),
)
check("protected_paths allows other tool_name", res_other.returncode == 0, res_other.stderr)

# ---------------------------------------------------------------------------
# 2. Event-time guard tests (SPEC 2)
# ---------------------------------------------------------------------------
sys.stdout.write("event_time: GUARDED_GLOBS, wall-clock, GetLatest, comments, exemptions\n")
check(
    "GUARDED_GLOBS matches spec",
    event_time_mod.GUARDED_GLOBS
    == [
        "knowledge-sovereign/app/usecase/knowledge_home_projector/*.go",
        "knowledge-sovereign/app/usecase/knowledge_trail_projector/*.go",
    ],
)

HOME_GO = "/abs/worktree/knowledge-sovereign/app/usecase/knowledge_home_projector/projector.go"
TRAIL_GO = "knowledge-sovereign/app/usecase/knowledge_trail_projector/folds.go"
TEST_GO = "knowledge-sovereign/app/usecase/knowledge_home_projector/projector_test.go"
UNGUARDED_GO = "knowledge-sovereign/app/driver/repo.go"

for call_name, snippet in [
    ("time.Now(", "now := time.Now()\n"),
    ("time.Since(", "d := time.Since(ev.OccurredAt)\n"),
    ("time.Until(", "d := time.Until(deadline)\n"),
    ("GetLatestVersion", "v := repo.GetLatestVersion(ctx)\n"),
]:
    for target in (HOME_GO, TRAIL_GO):
        res_w = run_script(EVENT_TIME_SCRIPT, stdin_text=claude_write(target, snippet))
        check(
            f"Write blocks {call_name} in {pathlib.Path(target).name}",
            res_w.returncode == BLOCKED_RC
            and "ADR-000468" in res_w.stderr
            and "ADR-000447" in res_w.stderr
            and "OccurredAt" in res_w.stderr,
            res_w.stderr,
        )
        res_e = run_script(EVENT_TIME_SCRIPT, stdin_text=claude_edit(target, "x := 1", snippet))
        check(
            f"Edit blocks {call_name} in {pathlib.Path(target).name}",
            res_e.returncode == BLOCKED_RC,
            res_e.stderr,
        )

# Allowed cases
res_test_go = run_script(
    EVENT_TIME_SCRIPT,
    stdin_text=claude_write(TEST_GO, "now := time.Now()\nv := repo.GetLatestItem(ctx)\n"),
)
check("allows _test.go", res_test_go.returncode == 0, res_test_go.stderr)

res_unguarded = run_script(
    EVENT_TIME_SCRIPT,
    stdin_text=claude_write(UNGUARDED_GO, "now := time.Now()\nv := repo.GetLatestItem(ctx)\n"),
)
check("allows unguarded path", res_unguarded.returncode == 0, res_unguarded.stderr)

res_comment = run_script(
    EVENT_TIME_SCRIPT,
    stdin_text=claude_write(
        HOME_GO,
        "// Never call time.Now() or GetLatestVersion here.\n"
        "t := ev.OccurredAt // instead of time.Since(x)\n",
    ),
)
check(
    "allows wall-clock/GetLatest in Go line comments",
    res_comment.returncode == 0,
    res_comment.stderr,
)

res_edit_old_only = run_script(
    EVENT_TIME_SCRIPT,
    stdin_text=claude_edit(HOME_GO, "now := time.Now()", "now := ev.OccurredAt"),
)
check("Edit inspects only new_string", res_edit_old_only.returncode == 0, res_edit_old_only.stderr)

# apply_patch touching one guarded (clean) and one unguarded (with time.Now) -> allow
patch_clean_guarded = (
    "*** Begin Patch\n"
    f"*** Update File: {UNGUARDED_GO}\n@@\n+now := time.Now()\n"
    f"*** Update File: {TRAIL_GO}\n@@\n+ts := ev.OccurredAt\n"
    "*** End Patch\n"
)
res_p_ok = run_script(EVENT_TIME_SCRIPT, stdin_text=codex_patch(patch_clean_guarded))
check(
    "apply_patch allows time.Now in unguarded while touching clean guarded",
    res_p_ok.returncode == 0,
    res_p_ok.stderr,
)

# apply_patch touching one unguarded (clean) and one guarded (with GetLatestState) -> block
patch_bad_guarded = (
    "*** Begin Patch\n"
    f"*** Update File: {UNGUARDED_GO}\n@@\n+ts := ev.OccurredAt\n"
    f"*** Update File: {TRAIL_GO}\n@@\n+s := store.GetLatestState(ctx)\n"
    "*** End Patch\n"
)
res_p_bad = run_script(EVENT_TIME_SCRIPT, stdin_text=codex_patch(patch_bad_guarded))
check(
    "apply_patch blocks GetLatestState in guarded file",
    res_p_bad.returncode == BLOCKED_RC,
    res_p_bad.stderr,
)

for label, raw in FAIL_CLOSED_INPUTS:
    res = run_script(EVENT_TIME_SCRIPT, stdin_text=raw)
    check(
        f"event_time fail-closed on {label}",
        res.returncode == BLOCKED_RC and "payload" in res.stderr.lower(),
        res.stderr,
    )

res_et_other = run_script(
    EVENT_TIME_SCRIPT,
    stdin_text=json.dumps({"tool_name": "Bash", "tool_input": {"command": "echo hi"}}),
)
check("event_time allows other tool_name", res_et_other.returncode == 0, res_et_other.stderr)

# ---------------------------------------------------------------------------
# 3. Selfcheck tests (SPEC 3)
# ---------------------------------------------------------------------------
sys.stdout.write("harness_selfcheck: clean fixture and H1..H9 fixtures\n")

with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    res_clean = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "clean fixture exits 0 and notes absent .claude/settings.json",
        res_clean.returncode == 0
        and "settings.json" in res_clean.stdout
        and "not checked" in res_clean.stdout.lower(),
        res_clean.stdout + res_clean.stderr,
    )

    res_hook_ok = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo), "--hook"])
    hook_lines = [ln for ln in res_hook_ok.stdout.splitlines() if ln.strip()]
    check(
        "--hook clean prints exactly one OK line and exits 0",
        res_hook_ok.returncode == 0
        and len(hook_lines) == 1
        and hook_lines[0].startswith("agent_harness_selfcheck: OK"),
        res_hook_ok.stdout,
    )

    # Adding a clean .claude/settings.json (with guard_ops_commands.py & guard_exec_commands.py on Bash) also passes with 0 findings.
    write_claude_settings(
        repo,
        {
            "hooks": {
                "PreToolUse": [
                    {
                        "matcher": "Edit|Write|NotebookEdit",
                        "hooks": [
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_protected_paths.py"',
                                "onFailure": "block",
                            },
                            {
                                "type": "command",
                                "command": 'python3 "${CLAUDE_PROJECT_DIR}/scripts/agent-harness/guard_event_time.py"',
                                "onFailure": "block",
                            },
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_packet_scope.py"',
                                "onFailure": "block",
                            },
                        ],
                    },
                    {
                        "matcher": "Bash",
                        "hooks": [
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_ops_commands.py"',
                                "onFailure": "block",
                            },
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_exec_commands.py"',
                                "onFailure": "block",
                            },
                        ],
                    },
                ]
            }
        },
    )
    res_clean_claude = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "clean fixture with .claude/settings.json exits 0",
        res_clean_claude.returncode == 0,
        res_clean_claude.stdout,
    )

# H1: CLAUDE_TOOL_INPUT / CLAUDE_FILE_PATHS
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][0]["hooks"].append(
        {"type": "command", "command": "echo $CLAUDE_FILE_PATHS"}
    )
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H1 detected", res.returncode == 1 and "H1 " in res.stdout, res.stdout)

    res_hook = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo), "--hook"])
    lines = [ln for ln in res_hook.stdout.splitlines() if ln.strip()]
    check(
        "--hook on failure prints 1 FAILED line and exits 0",
        res_hook.returncode == 0
        and len(lines) == 1
        and lines[0].startswith("agent_harness_selfcheck_FAILED:")
        and "H1" in lines[0],
        res_hook.stdout,
    )

# H2: missing repo script referenced in hook command
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][0]["hooks"].append(
        {
            "type": "command",
            "command": 'python3 "$(git rev-parse --show-toplevel)/scripts/does_not_exist.py"',
        }
    )
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H2 detected", res.returncode == 1 and "H2 " in res.stdout, res.stdout)

# H3: .claude/settings.json PreToolUse command hook without onFailure: block
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_claude_settings(
        repo,
        {
            "hooks": {
                "PreToolUse": [
                    {
                        "matcher": "Edit|Write",
                        "hooks": [
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_protected_paths.py"',
                            },
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_event_time.py"',
                                "onFailure": "block",
                            },
                        ],
                    },
                    {
                        "matcher": "Bash",
                        "hooks": [
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_ops_commands.py"',
                                "onFailure": "block",
                            },
                        ],
                    },
                ]
            }
        },
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H3 detected", res.returncode == 1 and "H3 " in res.stdout, res.stdout)

# H4: guard_ops_commands.py missing from .claude/settings.json PreToolUse
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_claude_settings(
        repo,
        {
            "hooks": {
                "PreToolUse": [
                    {
                        "matcher": "Edit|Write|NotebookEdit",
                        "hooks": [
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_protected_paths.py"',
                                "onFailure": "block",
                            },
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_event_time.py"',
                                "onFailure": "block",
                            },
                        ],
                    }
                ]
            }
        },
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H4 detected when guard_ops_commands.py missing from Claude PreToolUse",
        res.returncode == 1
        and "H4 " in res.stdout
        and "guard_ops_commands.py" in res.stdout
        and "Bash" in res.stdout,
        res.stdout,
    )

# H4: guard_ops_commands.py in .claude/settings.json with matcher not matching Bash
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_claude_settings(
        repo,
        {
            "hooks": {
                "PreToolUse": [
                    {
                        "matcher": "Edit|Write",
                        "hooks": [
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_protected_paths.py"',
                                "onFailure": "block",
                            },
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_event_time.py"',
                                "onFailure": "block",
                            },
                            {
                                "type": "command",
                                "command": 'python3 "$CLAUDE_PROJECT_DIR/scripts/agent-harness/guard_ops_commands.py"',
                                "onFailure": "block",
                            },
                        ],
                    }
                ]
            }
        },
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H4 detected when Claude guard_ops_commands.py matcher does not match Bash",
        res.returncode == 1 and "H4 " in res.stdout and "Bash" in res.stdout,
        res.stdout,
    )

# H4: guard_event_time.py missing from .codex/hooks.json PreToolUse
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][0]["hooks"] = [cfg["hooks"]["PreToolUse"][0]["hooks"][0]]
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H4 detected", res.returncode == 1 and "H4 " in res.stdout, res.stdout)

# H4: guard_ops_commands.py missing from .codex/hooks.json PreToolUse
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"] = [cfg["hooks"]["PreToolUse"][0]]
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H4 detected when guard_ops_commands.py missing from Codex PreToolUse",
        res.returncode == 1 and "H4 " in res.stdout and "guard_ops_commands.py" in res.stdout,
        res.stdout,
    )

# H4: guard_ops_commands.py in .codex/hooks.json without --on-match deny
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][1]["hooks"][0]["command"] = (
        'python3 "$(git rev-parse --show-toplevel)/scripts/agent-harness/guard_ops_commands.py"'
    )
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H4 detected when Codex guard_ops_commands.py lacks --on-match deny",
        res.returncode == 1
        and "H4 " in res.stdout
        and "--on-match deny" in res.stdout
        and "ask" in res.stdout.lower(),
        res.stdout,
    )

# H4: guard_ops_commands.py in .codex/hooks.json with matcher not matching Bash
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][1]["matcher"] = "Edit|Write"
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H4 detected when Codex guard_ops_commands.py matcher does not match Bash",
        res.returncode == 1 and "H4 " in res.stdout and "Bash" in res.stdout,
        res.stdout,
    )

# H4: guard_packet_scope.py missing from .codex/hooks.json PreToolUse
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][0]["hooks"] = [
        h
        for h in cfg["hooks"]["PreToolUse"][0]["hooks"]
        if "guard_packet_scope.py" not in h["command"]
    ]
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H4 detected when guard_packet_scope.py missing from Codex PreToolUse",
        res.returncode == 1 and "H4 " in res.stdout and "guard_packet_scope.py" in res.stdout,
        res.stdout,
    )

# H4: guard_exec_commands.py missing from .codex/hooks.json PreToolUse
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][1]["hooks"] = [
        h
        for h in cfg["hooks"]["PreToolUse"][1]["hooks"]
        if "guard_exec_commands.py" not in h["command"]
    ]
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H4 detected when guard_exec_commands.py missing from Codex PreToolUse",
        res.returncode == 1
        and "H4 " in res.stdout
        and "guard_exec_commands.py" in res.stdout
        and "Bash" in res.stdout,
        res.stdout,
    )

# H5: GUARDED_GLOBS entry matches no tracked non-test file
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    git(repo, "rm", "-f", "knowledge-sovereign/app/usecase/knowledge_trail_projector/projector.go")
    write_and_track(
        repo,
        "knowledge-sovereign/app/usecase/knowledge_trail_projector/projector_test.go",
        "package knowledge_trail_projector\n",
    )
    git(repo, "commit", "-m", "only test file remains")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H5 detected when only _test.go exists",
        res.returncode == 1 and "H5 " in res.stdout,
        res.stdout,
    )

# H6: .claude/rules paths entry matches no tracked file
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_and_track(
        repo,
        ".claude/rules/dead.md",
        '---\npaths:\n  - "nonexistent-dir/**/*.{go,py}"\n---\nDead rule\n',
    )
    git(repo, "commit", "-m", "add dead rule")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H6 detected", res.returncode == 1 and "H6 " in res.stdout, res.stdout)

# H7: mismatched shared block content between CLAUDE.md and AGENTS.md
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    (repo / "AGENTS.md").write_text(
        "# Shared\n<!-- shared:core:start -->\nDifferent content.\n<!-- shared:core:end -->\n",
        encoding="utf-8",
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H7 detected on mismatched block", res.returncode == 1 and "H7 " in res.stdout, res.stdout
    )

# H8: CLAUDE.md > 200 lines or AGENTS.md > 32768 bytes
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    base_claude = (repo / "CLAUDE.md").read_text(encoding="utf-8")
    (repo / "CLAUDE.md").write_text(base_claude + ("line\n" * 205), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H8 detected on CLAUDE.md > 200 lines",
        res.returncode == 1 and "H8 " in res.stdout,
        res.stdout,
    )

with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    base_agents = (repo / "AGENTS.md").read_text(encoding="utf-8")
    (repo / "AGENTS.md").write_text(base_agents + ("x" * 33000), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H8 detected on AGENTS.md > 32768 bytes",
        res.returncode == 1 and "H8 " in res.stdout,
        res.stdout,
    )

# H9: .agents/skills entry does not resolve to dir with SKILL.md
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    (repo / ".agents" / "skills" / "broken-skill").symlink_to(repo / "does-not-exist")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H9 detected on broken symlink", res.returncode == 1 and "H9 " in res.stdout, res.stdout)

# ---------------------------------------------------------------------------
# 4. Selfcheck H10 fixtures (failure-pattern catalog consistency)
# ---------------------------------------------------------------------------
sys.stdout.write("harness_selfcheck: H10 catalog fixtures\n")

PATTERNS_REL = ".claude/skills/failure-pattern-review/references/patterns.md"
SKILL_REL = ".claude/skills/failure-pattern-review/SKILL.md"


def make_fp_section(
    fp_id: str,
    title: str,
    precedent: str,
    cmd_note: str = "`scripts/agent-harness/guard_event_time.py`",
    check_cmd: str = "rg -n 'foo' <dir>/",
) -> str:
    return (
        f"## {fp_id} {title}\n\n"
        "**機構.** Something fails silently.\n\n"
        f"**先例.**\n- {precedent} — example precedent.\n\n"
        "**問い.**\n1. Is it checked?\n\n"
        f"**確認コマンド.**\n```bash\n{check_cmd}\n```\n\n"
        f"**機械的に見ているもの.** {cmd_note}\n"
    )


def build_h10_catalog_repo(
    repo: pathlib.Path,
    *,
    patterns_text: str | None = None,
    skill_text: str | None = None,
    extra_tracked: dict[str, str] | None = None,
) -> pathlib.Path:
    build_clean_selfcheck_repo(repo)
    if extra_tracked:
        for rel, body in extra_tracked.items():
            write_and_track(repo, rel, body)

    if patterns_text is None:
        patterns_text = (
            "# 失敗の型（FP-01 〜 FP-02）\n\n---\n\n"
            + make_fp_section(
                "FP-01",
                "Silent failure",
                "ADR-000928",
                "`scripts/agent-harness/guard_event_time.py --check` と `scripts/<svc>/placeholder.py`",
            )
            + "\n---\n\n"
            + make_fp_section(
                "FP-02",
                "Contract drift",
                "PM-2026-031",
                "`scripts/agent-harness/guard_protected_paths.py`",
            )
        )
    if skill_text is None:
        skill_text = (
            "# Failure Pattern Review\n\n"
            "## 経路表\n\n"
            "| 差分が触るもの | 読む型 |\n"
            "|---|---|\n"
            "| DI | FP-01 |\n"
            "| proto | FP-02 |\n"
        )
    write_and_track(repo, PATTERNS_REL, patterns_text)
    write_and_track(repo, SKILL_REL, skill_text)
    git(repo, "commit", "--allow-empty", "-m", "update failure-pattern catalog fixture")
    return repo


def h10_findings(stdout: str) -> list[str]:
    return [ln for ln in stdout.splitlines() if ln.startswith("H10 ")]


with tempfile.TemporaryDirectory() as tmp:
    repo = build_h10_catalog_repo(pathlib.Path(tmp))
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H10 clean two-section catalog passes", res.returncode == 0, res.stdout + res.stderr)

# H10: patterns.md absent
with tempfile.TemporaryDirectory() as tmp:
    repo = build_h10_catalog_repo(pathlib.Path(tmp))
    (repo / PATTERNS_REL).unlink()
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects missing patterns.md",
        res.returncode == 1
        and len(lines) == 1
        and "patterns.md" in lines[0]
        and "missing" in lines[0],
        res.stdout,
    )
    res_hook = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo), "--hook"])
    hook_lines = [ln for ln in res_hook.stdout.splitlines() if ln.strip()]
    check(
        "H10 --hook mentions H10 when H10 fails",
        res_hook.returncode == 0
        and len(hook_lines) == 1
        and hook_lines[0].startswith("agent_harness_selfcheck_FAILED:")
        and "H10" in hook_lines[0],
        res_hook.stdout,
    )

# H10: SKILL.md absent
with tempfile.TemporaryDirectory() as tmp:
    repo = build_h10_catalog_repo(pathlib.Path(tmp))
    (repo / SKILL_REL).unlink()
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects missing SKILL.md",
        res.returncode == 1
        and len(lines) == 1
        and "SKILL.md" in lines[0]
        and "missing" in lines[0],
        res.stdout,
    )

# H10 rule 1a: section gap (FP-01, FP-03)
with tempfile.TemporaryDirectory() as tmp:
    p_gap = (
        "# 失敗の型（FP-01 〜 FP-03）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928")
        + "\n"
        + make_fp_section("FP-03", "Third", "PM-2026-031")
    )
    s_gap = "# Skill\n\n## 経路表\n\n| A | B |\n|---|---|\n| a | FP-01 |\n| b | FP-03 |\n"
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_gap, skill_text=s_gap)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects section gap",
        res.returncode == 1 and len(lines) == 1 and "missing section 'FP-02'" in lines[0],
        res.stdout,
    )

# H10 rule 1b: duplicate section heading
with tempfile.TemporaryDirectory() as tmp:
    p_dup = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928")
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
        + "\n"
        + make_fp_section("FP-02", "Duplicate", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_dup)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects duplicate section",
        res.returncode == 1 and len(lines) == 1 and "duplicate section 'FP-02'" in lines[0],
        res.stdout,
    )

# H10 rule 1c: invalid section heading (single finding)
with tempfile.TemporaryDirectory() as tmp:
    p_bad_head = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928")
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
        + "\n## FP-1 Bad\n\nExtra text.\n"
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_bad_head)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects bad heading format",
        res.returncode == 1
        and len(lines) == 1
        and "invalid section heading '## FP-1 Bad'" in lines[0],
        res.stdout,
    )

# H10 rule 1d: FP-00 rejected (must start at 01)
with tempfile.TemporaryDirectory() as tmp:
    p_fp00 = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-00", "Zero", "ADR-000928")
        + "\n"
        + make_fp_section("FP-01", "First", "ADR-000928")
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    s_fp00 = "# Skill\n\n## 経路表\n\n| A | B |\n|---|---|\n| a | FP-00 |\n| b | FP-01 |\n| c | FP-02 |\n"
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_fp00, skill_text=s_fp00)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 rejects FP-00 section",
        res.returncode == 1
        and len(lines) == 1
        and "FP-00" in lines[0]
        and "start at FP-01" in lines[0],
        res.stdout,
    )

# H10 rule 2: title range mismatch (including prefix trap FP-01 〜 FP-020)
with tempfile.TemporaryDirectory() as tmp:
    p_title = (
        "# 失敗の型（FP-01 〜 FP-020）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928")
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_title)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects title range mismatch (prefix trap)",
        res.returncode == 1
        and len(lines) == 1
        and "first-line title must contain 'FP-01 〜 FP-02'" in lines[0],
        res.stdout,
    )

# H10 rule 3a: missing lead-in
with tempfile.TemporaryDirectory() as tmp:
    p_miss_lead = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928").replace(
            "**問い.**\n1. Is it checked?\n\n", ""
        )
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_miss_lead)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects missing lead-in",
        res.returncode == 1
        and len(lines) == 1
        and "FP-01 is missing lead-in **問い.**" in lines[0],
        res.stdout,
    )

# H10 rule 3c: duplicate lead-in
with tempfile.TemporaryDirectory() as tmp:
    p_dup_lead = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928").replace(
            "**機構.** Something fails silently.",
            "**機構.** Something fails silently.\n\n**機構.** Duplicate lead-in.",
        )
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_dup_lead)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects duplicate lead-in",
        res.returncode == 1
        and len(lines) == 1
        and "FP-01 contains lead-in **機構.** 2 times" in lines[0],
        res.stdout,
    )

# H10 rule 3b: out-of-order lead-ins
with tempfile.TemporaryDirectory() as tmp:
    swapped_sec = (
        "## FP-01 Swapped\n\n"
        "**先例.**\n- ADR-000928 — precedent.\n\n"
        "**機構.** Mechanism after precedent.\n\n"
        "**問い.**\n1. Question?\n\n"
        "**確認コマンド.**\n```bash\nrg -n x\n```\n\n"
        "**機械的に見ているもの.** `scripts/agent-harness/guard_event_time.py`\n"
    )
    p_order = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + swapped_sec
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_order)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects out-of-order lead-ins",
        res.returncode == 1 and len(lines) == 1 and "FP-01 lead-ins are out of order" in lines[0],
        res.stdout,
    )

# H10 rule 4a: missing ADR-000NNN precedent file (adjacent to Japanese text)
with tempfile.TemporaryDirectory() as tmp:
    p_adr = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000999は失敗例")
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_adr)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects missing ADR precedent file adjacent to Japanese text",
        res.returncode == 1
        and len(lines) == 1
        and "ADR-000999" in lines[0]
        and "docs/ADR/000999.md" in lines[0],
        res.stdout,
    )

# H10 rule 4b: missing PM-YYYY-NNN precedent file (adjacent to Japanese text)
with tempfile.TemporaryDirectory() as tmp:
    p_pm = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928")
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-999で再発")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_pm)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects missing PM precedent file adjacent to Japanese text",
        res.returncode == 1 and len(lines) == 1 and "PM-2026-999" in lines[0],
        res.stdout,
    )

# H10 rule 4c: duplicate PM-YYYY-NNN precedent files
with tempfile.TemporaryDirectory() as tmp:
    repo = build_h10_catalog_repo(
        pathlib.Path(tmp),
        extra_tracked={"docs/postmortems/PM-2026-031-duplicate.md": "# dup\n"},
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects duplicate PM precedent file",
        res.returncode == 1
        and len(lines) == 1
        and "PM-2026-031" in lines[0]
        and "multiple" in lines[0],
        res.stdout,
    )

# H10 rule 4d: abbreviated id in 先例 block
with tempfile.TemporaryDirectory() as tmp:
    p_abbrev = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section("FP-01", "First", "ADR-000928")
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031 / 999")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_abbrev)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects abbreviated precedent id in 先例",
        res.returncode == 1
        and len(lines) == 1
        and "999" in lines[0]
        and "full" in lines[0].lower(),
        res.stdout,
    )

# H10 rule 5a: missing backticked repository path behind command word
with tempfile.TemporaryDirectory() as tmp:
    p_path = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section(
            "FP-01", "First", "ADR-000928", "`python3 scripts/no_such_tool.py verify`"
        )
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_path)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects missing path behind python3",
        res.returncode == 1 and len(lines) == 1 and "scripts/no_such_tool.py" in lines[0],
        res.stdout,
    )

# H10 rule 5b: missing repository path inside fenced code block
with tempfile.TemporaryDirectory() as tmp:
    p_fenced = (
        "# 失敗の型（FP-01 〜 FP-02）\n\n"
        + make_fp_section(
            "FP-01",
            "First",
            "ADR-000928",
            check_cmd="python3 scripts/missing_in_fence.py --check",
        )
        + "\n"
        + make_fp_section("FP-02", "Second", "PM-2026-031")
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), patterns_text=p_fenced)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects missing path inside fenced block",
        res.returncode == 1 and len(lines) == 1 and "scripts/missing_in_fence.py" in lines[0],
        res.stdout,
    )

# H10 rule 6a: SKILL.md routing table references non-existent FP id (adjacent to Japanese)
with tempfile.TemporaryDirectory() as tmp:
    s_extra = (
        "# Skill\n\n## 経路表\n\n| A | B |\n|---|---|\n| a | FP-01 |\n| b | FP-02, FP-99と他 |\n"
    )
    repo = build_h10_catalog_repo(pathlib.Path(tmp), skill_text=s_extra)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects unknown FP id in SKILL.md table adjacent to Japanese",
        res.returncode == 1 and len(lines) == 1 and "FP-99" in lines[0],
        res.stdout,
    )

# H10 rule 6b: patterns.md section omitted from SKILL.md routing table
with tempfile.TemporaryDirectory() as tmp:
    s_missing = "# Skill\n\n## 経路表\n\n| A | B |\n|---|---|\n| a | FP-01 |\n"
    repo = build_h10_catalog_repo(pathlib.Path(tmp), skill_text=s_missing)
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h10_findings(res.stdout)
    check(
        "H10 detects patterns.md section missing from SKILL.md table",
        res.returncode == 1
        and len(lines) == 1
        and "FP-02" in lines[0]
        and "does not appear" in lines[0],
        res.stdout,
    )

# ---------------------------------------------------------------------------
# 4b. Selfcheck H11 fixtures (Rule Ledger & judgment-rule ratchet)
# ---------------------------------------------------------------------------
sys.stdout.write("harness_selfcheck: H11 rule ledger fixtures\n")

RULE_LEDGER_REL = ".agents/rules/rule-ledger.md"


def h11_findings(stdout: str) -> list[str]:
    return [ln for ln in stdout.splitlines() if ln.startswith("H11 ")]


def write_rule_ledger(repo: pathlib.Path, content: str) -> None:
    write_and_track(repo, RULE_LEDGER_REL, content)
    git(repo, "commit", "--allow-empty", "-m", "update rule ledger fixture")


# H11: clean fixture passes
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H11 clean rule ledger passes", res.returncode == 0, res.stdout + res.stderr)

# H11: missing .agents/rules/rule-ledger.md
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    (repo / RULE_LEDGER_REL).unlink()
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects missing rule-ledger.md",
        res.returncode == 1 and len(lines) == 1 and "rule-ledger.md" in lines[0],
        res.stdout,
    )

# H11: missing max_judgment_rules metadata
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "# Rule Ledger\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Sample | `scripts/agent-harness/guard_event_time.py` | ADR-000928 |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects missing max_judgment_rules metadata",
        res.returncode == 1 and any("max_judgment_rules" in ln for ln in lines),
        res.stdout,
    )

# H11: missing table / wrong columns
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 0\n"
            "---\n\n"
            "# Rule Ledger\n\n"
            "| ID | Rule | How |\n"
            "|---|---|---|\n"
            "| R-01 | Sample | `scripts/agent-harness/guard_event_time.py` |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects missing canonical 4-column table",
        res.returncode == 1 and len(lines) >= 1,
        res.stdout,
    )

# H11: missing script path in 強制手段
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 0\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Missing script | `scripts/lint/does_not_exist.py` | ADR-000928 |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects missing script path in 強制手段",
        res.returncode == 1 and any("scripts/lint/does_not_exist.py" in ln for ln in lines),
        res.stdout,
    )

# H11: missing ast-grep rule ID in 強制手段
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 0\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Missing ast-grep | `ast-grep:no-such-rule` | C-01 |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects missing ast-grep rule ID in 強制手段",
        res.returncode == 1 and any("no-such-rule" in ln for ln in lines),
        res.stdout,
    )

# H11: non-判断 row without any backticked repo path or ast-grep ID
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 0\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Vague enforcement | CI checks it somehow | AGENTS.md |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects non-判断 row without concrete path or ast-grep ID",
        res.returncode == 1 and any("R-01" in ln for ln in lines),
        res.stdout,
    )

# H11: missing corpus C-99 and missing ADR-000999 in 由来
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 0\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Bad refs | `scripts/agent-harness/guard_event_time.py` | C-99 / ADR-000999 |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects missing corpus C-99 and ADR-000999 in 由来",
        res.returncode == 1
        and any("C-99" in ln for ln in lines)
        and any("ADR-000999" in ln for ln in lines),
        res.stdout,
    )

# H11: judgment ratchet exceeded (judgment_rows > max_judgment_rules)
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 1\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | One | 判断 | AGENTS.md |\n"
            "| R-02 | Two | 判断（レビュー） | AGENTS.md |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects judgment rows exceeding max_judgment_rules",
        res.returncode == 1 and any("max_judgment_rules" in ln for ln in lines),
        res.stdout,
    )

# H11: judgment ratchet not lowered when judgment_rows < max_judgment_rules
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 2\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Mechanized | `scripts/agent-harness/guard_event_time.py` | ADR-000928 |\n"
            "| R-02 | One judgment | 判断 | AGENTS.md |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 detects unlowered max_judgment_rules when judgment rows decrease",
        res.returncode == 1 and any("max_judgment_rules" in ln for ln in lines),
        res.stdout,
    )

# H11: directory-only backtick ref (e.g. `scripts/`) is rejected
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 0\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Directory only | `scripts/` | ADR-000928 |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 rejects directory-only backtick ref (`scripts/`)",
        res.returncode == 1 and any("scripts/" in ln for ln in lines),
        res.stdout,
    )

# H11: non-existent backticked file token without slash paired with real path is rejected
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_rule_ledger(
        repo,
        (
            "---\n"
            "max_judgment_rules: 0\n"
            "---\n\n"
            "| ID | 規則 | 強制手段 | 由来 |\n"
            "|---|---|---|---|\n"
            "| R-01 | Fake token | `scripts/agent-harness/guard_event_time.py`, `fake_nonexistent.py` | ADR-000928 |\n"
        ),
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines = h11_findings(res.stdout)
    check(
        "H11 rejects non-existent backticked .py token without slash",
        res.returncode == 1 and any("fake_nonexistent.py" in ln for ln in lines),
        res.stdout,
    )

# H11: cross-checks [governance].max_judgment_rules in scripts/lint/ratchet/suppressions.toml
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_and_track(
        repo,
        "scripts/lint/ratchet/suppressions.toml",
        "version = 1\n\n[governance]\nmax_judgment_rules = 99\n",
    )
    git(repo, "commit", "-m", "mismatched suppressions.toml governance")
    res_bad = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    lines_bad = h11_findings(res_bad.stdout)
    check(
        "H11 detects mismatch between rule-ledger.md and suppressions.toml max_judgment_rules",
        res_bad.returncode == 1 and any("suppressions.toml" in ln for ln in lines_bad),
        res_bad.stdout,
    )

    write_and_track(
        repo,
        "scripts/lint/ratchet/suppressions.toml",
        "version = 1\n\n[governance]\nmax_judgment_rules = 1\n",
    )
    git(repo, "commit", "-m", "matched suppressions.toml governance")
    res_ok = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "H11 passes when suppressions.toml max_judgment_rules matches rule-ledger.md",
        res_ok.returncode == 0,
        res_ok.stdout + res_ok.stderr,
    )

# Real repository check: require clean exit 0 and zero findings.
sys.stdout.write("harness_selfcheck: real repository check\n")
res_real = run_script(SELFCHECK_SCRIPT, args=["--root", str(ROOT)])
finding_lines = [ln for ln in res_real.stdout.splitlines() if re.match(r"^H\d+ ", ln)]
check(
    "real repo exits 0 with no findings",
    res_real.returncode == 0 and finding_lines == [],
    f"rc={res_real.returncode}, output={res_real.stdout}",
)

# ---------------------------------------------------------------------------
# 5. Ops commands guard tests (O1..O4, G1..G5, negatives, unreadable)
# ---------------------------------------------------------------------------
sys.stdout.write(
    "guard_ops_commands: O1..O4 ask rules, G1..G5, negatives, and unreadable payloads\n"
)

OPS_ASK_CASES = [
    # O1: prune, volume/image/network rm|remove, rmi, buildx prune
    (
        "O1 system prune with global flag",
        "docker --context default system prune -af",
        "O1",
        "PM-2026-054",
    ),
    (
        "O1 image prune after chained cmd",
        "docker ps && sudo docker image prune -a -f",
        "O1",
        "PM-2026-054",
    ),
    (
        "O1 volume rm and rmi",
        "docker volume rm old_certs; docker rmi alt-backend:old",
        "O1",
        "PM-2026-046",
    ),
    (
        "O1 image rm with env",
        "DOCKER_HOST=unix:///var/run/docker.sock docker image rm myimg",
        "O1",
        "PM-2026-054",
    ),
    ("O1 network prune", "docker network prune -f", "O1", "PM-2026-046"),
    ("O1 builder prune", "docker builder prune --all", "O1", "PM-2026-054"),
    ("O1 G2 docker network rm", "docker network rm alt-net", "O1", "PM-2026-046"),
    ("O1 G2 docker image remove", "docker image remove myimg:old", "O1", "PM-2026-054"),
    ("O1 G2 docker volume remove", "docker volume remove old_certs", "O1", "PM-2026-046"),
    ("O1 G2 docker buildx prune", "docker buildx prune -f", "O1", "PM-2026-054"),
    ("O1 G1 sh -c double quotes", 'sh -c "docker system prune -af"', "O1", "PM-2026-054"),
    # O2: rm, stop, kill, container rm|remove|stop|kill, compose stop|kill
    ("O2 docker rm with flag", "docker -l debug rm -f pki-agent", "O2", "PM-2026-028"),
    (
        "O2 chained docker container stop",
        "echo checking && docker container stop -t 5 alt-backend",
        "O2",
        "PM-2026-028",
    ),
    ("O2 docker kill", "sudo -E docker kill alt-data-hub", "O2", "PM-2026-054"),
    ("O2 docker container rm", "docker container rm alt-backend", "O2", "PM-2026-028"),
    ("O2 G2 docker container remove", "docker container remove alt-backend", "O2", "PM-2026-028"),
    (
        "O2 G2 docker compose stop",
        "docker compose -f compose/compose.yaml -p alt stop alt-backend",
        "O2",
        "PM-2026-028",
    ),
    ("O2 G2 docker compose kill", "docker compose kill alt-backend", "O2", "PM-2026-028"),
    (
        "O2 G1 for loop",
        "for c in $(docker ps -aq --filter name=alt-staging-); do docker rm -f $c; done",
        "O2",
        "PM-2026-028",
    ),
    (
        "O2 G1 while read loop",
        "while read c; do docker stop $c; done < ids.txt",
        "O2",
        "PM-2026-028",
    ),
    (
        "O2 G1 pipe xargs -r",
        "docker ps -q --filter name=alt-staging- | xargs -r docker rm -f",
        "O2",
        "PM-2026-028",
    ),
    ("O2 G1 bare xargs", "xargs docker rm", "O2", "PM-2026-028"),
    ("O2 G1 braces block", "{ docker rm x; }", "O2", "PM-2026-028"),
    ("O2 G1 command substitution in double quotes", 'echo "$(docker rm x)"', "O2", "PM-2026-028"),
    ("O2 G1 sudo -iu root", "sudo -iu root docker rm x", "O2", "PM-2026-028"),
    # O3: docker compose / docker-compose up|down|rm on prod, default, or unresolved ($) project
    (
        "O3 compose up with -f and default project",
        "docker compose -f compose/compose.yaml up -d alt-backend",
        "O3",
        "PM-2026-001",
    ),
    (
        "O3 compose down with -p alt",
        "docker compose -f compose/compose.yaml -p alt down",
        "O3",
        "PM-2026-005",
    ),
    (
        "O3 compose rm with COMPOSE_PROJECT_NAME=alt",
        "COMPOSE_PROJECT_NAME=alt docker-compose rm -f",
        "O3",
        "PM-2026-001",
    ),
    (
        "O3 chained compose up --project-name alt",
        "git status || docker compose --project-name alt up -d",
        "O3",
        "PM-2026-005",
    ),
    ("O3 G1 if then fi", "if true; then docker compose -p alt down; fi", "O3", "PM-2026-001"),
    ("O3 G1 bash -c single quotes", "bash -c 'docker compose -p alt down'", "O3", "PM-2026-001"),
    ("O3 G1 time prefix", "time docker compose -p alt down", "O3", "PM-2026-001"),
    ("O3 G3 -p $PROJ", "docker compose -p $PROJ down", "O3", "PM-2026-001"),
    (
        "O3 G3 -p $COMPOSE_PROJECT_NAME",
        'docker compose -p "$COMPOSE_PROJECT_NAME" up -d',
        "O3",
        "PM-2026-001",
    ),
    # O4: docker compose ... up with --build or --force-recreate on O3 project
    ("O4 compose up --build", "docker compose up --build -d alt-backend", "O4", "PM-2026-031"),
    (
        "O4 compose -f -p alt up --force-recreate",
        "docker compose -f compose/compose.yaml -p alt up -d --no-deps --force-recreate alt-backend",
        "O4",
        "PM-2026-031",
    ),
    (
        "O4 chained docker-compose up --build",
        "echo start\nCOMPOSE_PROJECT_NAME=alt docker-compose -f compose/compose.yaml up --build -d",
        "O4",
        "PM-2026-031",
    ),
    (
        "O4 G1 timeout prefix",
        "timeout 300 docker compose -f compose/compose.yaml -p alt up -d --build alt-backend",
        "O4",
        "PM-2026-031",
    ),
    ("O4 G1 nohup background", "nohup docker compose -p alt up -d --build &", "O4", "PM-2026-031"),
]

for label, cmd, expected_rule, expected_pm in OPS_ASK_CASES:
    res = run_script(OPS_COMMANDS_SCRIPT, stdin_text=bash_payload(cmd))
    parsed = None
    try:
        parsed = json.loads(res.stdout) if res.stdout.strip() else None
    except ValueError:
        parsed = None
    hook_out = parsed.get("hookSpecificOutput", {}) if isinstance(parsed, dict) else {}
    reason = hook_out.get("permissionDecisionReason", "")
    ok = (
        res.returncode == 0
        and isinstance(parsed, dict)
        and hook_out.get("hookEventName") == "PreToolUse"
        and hook_out.get("permissionDecision") == "ask"
        and expected_rule in reason
        and expected_pm in reason
    )
    if expected_rule == "O4":
        ok = ok and "release path" in reason and "contract gate" in reason
    check(f"ops ask {label}", ok, f"rc={res.returncode} out={res.stdout!r} err={res.stderr!r}")

# G5: list (argv) form tests
res_g5_bash_lc = run_script(
    OPS_COMMANDS_SCRIPT,
    stdin_text=json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "Bash",
            "tool_input": {"command": ["bash", "-lc", "docker compose -p alt down"]},
        }
    ),
)
parsed_g5 = json.loads(res_g5_bash_lc.stdout) if res_g5_bash_lc.stdout.strip() else {}
hook_g5 = parsed_g5.get("hookSpecificOutput", {}) if isinstance(parsed_g5, dict) else {}
check(
    "ops ask G5 argv bash -lc",
    res_g5_bash_lc.returncode == 0
    and hook_g5.get("hookEventName") == "PreToolUse"
    and hook_g5.get("permissionDecision") == "ask"
    and "O3" in hook_g5.get("permissionDecisionReason", ""),
    f"rc={res_g5_bash_lc.returncode} out={res_g5_bash_lc.stdout!r}",
)

OPS_ALLOW_CASES = [
    ("read-only docker ps", bash_payload("docker ps -a")),
    ("read-only docker logs", bash_payload("docker logs --tail 50 alt-backend")),
    ("read-only docker inspect", bash_payload("docker inspect alt-backend")),
    ("read-only docker images", bash_payload("docker images")),
    ("read-only docker version", bash_payload("docker version")),
    ("read-only docker exec", bash_payload("docker exec alt-backend /alt-backend healthcheck")),
    ("read-only compose config", bash_payload("docker compose -f compose/compose.yaml config")),
    ("read-only compose ps", bash_payload("docker compose -f compose/compose.yaml -p alt ps")),
    ("read-only compose logs", bash_payload("docker compose -p alt logs -f alt-backend")),
    (
        "different project -p alt-test",
        bash_payload("docker compose -f compose/compose.yaml -p alt-test up --build -d"),
    ),
    (
        "different project -p after subcmd",
        bash_payload("docker compose -f compose/compose.yaml up --build -d -p alt-test"),
    ),
    (
        "different project --project-name=e2e",
        bash_payload("docker compose --project-name=e2e down"),
    ),
    (
        "different project COMPOSE_PROJECT_NAME=staging",
        bash_payload("COMPOSE_PROJECT_NAME=staging docker compose up --force-recreate -d"),
    ),
    (
        "cli -p overrides COMPOSE_PROJECT_NAME=alt",
        bash_payload("COMPOSE_PROJECT_NAME=alt docker compose -p test-proj up -d"),
    ),
    ("different project compose stop", bash_payload("docker compose -p alt-test stop alt-backend")),
    (
        "words inside echo double quotes",
        bash_payload('echo "docker system prune -af && docker compose up --build"'),
    ),
    (
        "words inside git commit message",
        bash_payload(
            "git commit -m 'PM-2026-031: never run docker compose up --build -d alt-backend'"
        ),
    ),
    (
        "G5 argv echo with semicolon",
        json.dumps(
            {
                "hook_event_name": "PreToolUse",
                "tool_name": "Bash",
                "tool_input": {"command": ["echo", "a; docker rm x"]},
            }
        ),
    ),
    ("empty command", bash_payload("")),
    ("whitespace command", bash_payload("   \n  ")),
    (
        "non-shell tool payload",
        json.dumps({"tool_name": "Read", "tool_input": {"file_path": ".env"}}),
    ),
]

for label, raw_stdin in OPS_ALLOW_CASES:
    res = run_script(OPS_COMMANDS_SCRIPT, stdin_text=raw_stdin)
    check(
        f"ops allows {label}",
        res.returncode == 0 and res.stdout == "" and res.stderr == "",
        f"rc={res.returncode} out={res.stdout!r} err={res.stderr!r}",
    )

OPS_UNREADABLE_CASES: list[tuple[str, str | None, bytes | None]] = [
    ("empty stdin", "", None),
    ("invalid json", "{bad", None),
    ("json array", "[]", None),
    ("empty object", "{}", None),
    ("Bash missing tool_input", json.dumps({"tool_name": "Bash"}), None),
    ("Bash missing command", json.dumps({"tool_name": "Bash", "tool_input": {}}), None),
    (
        "Bash non-string command",
        json.dumps({"tool_name": "Bash", "tool_input": {"command": 42}}),
        None,
    ),
    ("G4 invalid UTF-8 bytes", None, b"\xff\xfe"),
    ("G4 deeply nested JSON", ("[" * 2000) + ("]" * 2000), None),
]

for label, raw_stdin, raw_bytes in OPS_UNREADABLE_CASES:
    res = run_script(OPS_COMMANDS_SCRIPT, stdin_text=raw_stdin, stdin_bytes=raw_bytes)
    parsed = None
    try:
        parsed = json.loads(res.stdout) if res.stdout.strip() else None
    except ValueError:
        parsed = None
    hook_out = parsed.get("hookSpecificOutput", {}) if isinstance(parsed, dict) else {}
    reason = hook_out.get("permissionDecisionReason", "")
    ok = (
        res.returncode == 0
        and isinstance(parsed, dict)
        and hook_out.get("hookEventName") == "PreToolUse"
        and hook_out.get("permissionDecision") == "ask"
        and ("unreadable" in reason.lower() or "could not be analysed" in reason.lower())
    )
    check(
        f"ops unreadable asks on {label}",
        ok,
        f"rc={res.returncode} out={res.stdout!r} err={res.stderr!r}",
    )

# --on-match deny mode tests (O1..O4, unreadable, benign, default ask, unknown arg)
OPS_DENY_RULE_CASES = [
    ("O1", "docker system prune -af"),
    ("O2", "docker rm -f alt-backend"),
    ("O3", "docker compose -p alt down"),
    ("O4", "docker compose -p alt up --build -d alt-backend"),
]

for expected_rule, cmd in OPS_DENY_RULE_CASES:
    res_deny = run_script(
        OPS_COMMANDS_SCRIPT,
        stdin_text=bash_payload(cmd),
        args=["--on-match", "deny"],
    )
    parsed_deny = json.loads(res_deny.stdout) if res_deny.stdout.strip() else {}
    hook_deny = parsed_deny.get("hookSpecificOutput", {}) if isinstance(parsed_deny, dict) else {}
    reason_deny = hook_deny.get("permissionDecisionReason", "")
    check(
        f"ops --on-match deny blocks {expected_rule}",
        res_deny.returncode == 0
        and hook_deny.get("hookEventName") == "PreToolUse"
        and hook_deny.get("permissionDecision") == "deny"
        and expected_rule in reason_deny
        and "retry" in reason_deny.lower()
        and "user" in reason_deny.lower()
        and "quote" in reason_deny.lower(),
        f"rc={res_deny.returncode} out={res_deny.stdout!r} err={res_deny.stderr!r}",
    )

    res_default = run_script(OPS_COMMANDS_SCRIPT, stdin_text=bash_payload(cmd))
    parsed_default = json.loads(res_default.stdout) if res_default.stdout.strip() else {}
    hook_default = (
        parsed_default.get("hookSpecificOutput", {}) if isinstance(parsed_default, dict) else {}
    )
    check(
        f"ops default mode still asks on {expected_rule}",
        res_default.returncode == 0
        and hook_default.get("hookEventName") == "PreToolUse"
        and hook_default.get("permissionDecision") == "ask"
        and expected_rule in hook_default.get("permissionDecisionReason", ""),
        f"rc={res_default.returncode} out={res_default.stdout!r}",
    )

res_deny_unreadable = run_script(
    OPS_COMMANDS_SCRIPT,
    stdin_text="{bad",
    args=["--on-match", "deny"],
)
parsed_du = json.loads(res_deny_unreadable.stdout) if res_deny_unreadable.stdout.strip() else {}
hook_du = parsed_du.get("hookSpecificOutput", {}) if isinstance(parsed_du, dict) else {}
check(
    "ops --on-match deny blocks unreadable payload",
    res_deny_unreadable.returncode == 0
    and hook_du.get("hookEventName") == "PreToolUse"
    and hook_du.get("permissionDecision") == "deny"
    and "unreadable" in hook_du.get("permissionDecisionReason", "").lower(),
    f"rc={res_deny_unreadable.returncode} out={res_deny_unreadable.stdout!r}",
)

for mode_label, extra_args in [("default", []), ("--on-match deny", ["--on-match", "deny"])]:
    res_benign = run_script(
        OPS_COMMANDS_SCRIPT,
        stdin_text=bash_payload("docker ps -a"),
        args=extra_args,
    )
    check(
        f"ops benign command silent in {mode_label}",
        res_benign.returncode == 0 and res_benign.stdout == "" and res_benign.stderr == "",
        f"rc={res_benign.returncode} out={res_benign.stdout!r} err={res_benign.stderr!r}",
    )

res_bad_flag = run_script(
    OPS_COMMANDS_SCRIPT,
    stdin_text=bash_payload("docker ps -a"),
    args=["--on-match", "block"],
)
check(
    "ops unknown --on-match exits 2 with usage on stderr",
    res_bad_flag.returncode == USAGE_RC and "usage" in res_bad_flag.stderr.lower(),
    f"rc={res_bad_flag.returncode} out={res_bad_flag.stdout!r} err={res_bad_flag.stderr!r}",
)

sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
