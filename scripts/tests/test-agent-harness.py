#!/usr/bin/env python3
"""Tests for scripts/agent-harness/ guards and self-check.

Run:
    python3 scripts/tests/test-agent-harness.py
"""

from __future__ import annotations

import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
HARNESS_DIR = ROOT / "scripts" / "agent-harness"
PROTECTED_SCRIPT = HARNESS_DIR / "guard_protected_paths.py"
EVENT_TIME_SCRIPT = HARNESS_DIR / "guard_event_time.py"
SELFCHECK_SCRIPT = HARNESS_DIR / "harness_selfcheck.py"

PASS = 0
FAIL = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        suffix = f" ({detail})" if detail else ""
        print(f"  FAIL  {name}{suffix}")
        FAIL += 1


def load_module(name: str, path: pathlib.Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None, f"missing module: {path}"
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


def run_script(
    script: pathlib.Path,
    *,
    stdin_text: str | None = None,
    args: list[str] | None = None,
    cwd: pathlib.Path | None = None,
) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(script), *(args or [])]
    return subprocess.run(
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


def git(repo: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(repo), *args],
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
    write_and_track(repo, "scripts/agent-harness/guard_protected_paths.py", "#!/usr/bin/env python3\n")
    write_and_track(repo, "scripts/agent-harness/guard_event_time.py", "#!/usr/bin/env python3\n")
    write_and_track(repo, ".claude/skills/adr-preflight/scripts/adr_preflight.py", "#!/usr/bin/env python3\n")
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
        "---\npaths:\n  - \"alt-backend/**/*.go\"\n  - \"alt-frontend-sv/src/routes/(app)/*.{svelte,ts}\"\n---\nRule body\n",
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
                    ],
                }
            ]
        }
    }
    write_and_track(repo, ".codex/hooks.json", json.dumps(hooks_cfg, indent=2) + "\n")

    shared = (
        "# Shared\n"
        "<!-- shared:core:start -->\n"
        "Never skip verification.\n"
        "<!-- shared:core:end -->\n"
    )
    write_and_track(repo, "CLAUDE.md", shared)
    write_and_track(repo, "AGENTS.md", shared)
    write_and_track(repo, ".agents/skills/sample-skill/SKILL.md", "---\nname: sample-skill\n---\n")
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
print("protected_paths: allow & block rules across Write / Edit / apply_patch")

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
    check(f"Write blocks {path}", res_w.returncode == 2 and path in res_w.stderr, res_w.stderr)

    res_e = run_script(PROTECTED_SCRIPT, stdin_text=claude_edit(path))
    check(f"Edit blocks {path}", res_e.returncode == 2 and path in res_e.stderr, res_e.stderr)

    patch = f"*** Begin Patch\n*** Update File: {path}\n@@\n+x=1\n*** End Patch\n"
    res_p = run_script(PROTECTED_SCRIPT, stdin_text=codex_patch(patch))
    check(f"apply_patch blocks {path}", res_p.returncode == 2 and path in res_p.stderr, res_p.stderr)

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
check("apply_patch Move to secrets/ blocks", res_mixed.returncode == 2, res_mixed.stderr)

print("protected_paths: migrations tracked vs untracked")
with tempfile.TemporaryDirectory() as tmp:
    repo = init_git_repo(pathlib.Path(tmp))
    tracked_mig = write_and_track(repo, "db/migrations/001_init.sql", "CREATE TABLE t (id INT);\n")
    git(repo, "commit", "-m", "add migration")
    untracked_mig = repo / "db" / "migrations" / "002_new.sql"
    untracked_nonexistent_parent = repo / "new_svc" / "migrations" / "001_init.sql"

    for label, payload in [
        ("Write", claude_write(str(tracked_mig))),
        ("Edit", claude_edit(str(tracked_mig))),
        ("apply_patch", codex_patch("*** Begin Patch\n*** Update File: db/migrations/001_init.sql\n@@\n+ALTER TABLE t;\n*** End Patch\n")),
    ]:
        res = run_script(PROTECTED_SCRIPT, stdin_text=payload, cwd=repo)
        check(
            f"{label} blocks tracked migration",
            res.returncode == 2 and "atlas" in res.stderr.lower(),
            res.stderr,
        )

    for label, payload in [
        ("Write", claude_write(str(untracked_mig))),
        ("Edit", claude_edit(str(untracked_mig))),
        ("apply_patch", codex_patch("*** Begin Patch\n*** Add File: db/migrations/002_new.sql\n+CREATE TABLE u (id INT);\n*** End Patch\n")),
    ]:
        res = run_script(PROTECTED_SCRIPT, stdin_text=payload, cwd=repo)
        check(f"{label} allows untracked migration", res.returncode == 0, res.stderr)

    res_new_dir = run_script(
        PROTECTED_SCRIPT,
        stdin_text=claude_write(str(untracked_nonexistent_parent)),
        cwd=repo,
    )
    check("Write allows untracked migration in new migrations dir", res_new_dir.returncode == 0, res_new_dir.stderr)

with tempfile.TemporaryDirectory() as non_git_tmp:
    non_git_mig = pathlib.Path(non_git_tmp) / "db" / "migrations" / "001_init.sql"
    non_git_mig.parent.mkdir(parents=True, exist_ok=True)
    res_nongit = run_script(PROTECTED_SCRIPT, stdin_text=claude_write(str(non_git_mig)))
    check("migration outside git repo blocks fail-closed", res_nongit.returncode == 2, res_nongit.stderr)

print("protected_paths: fail-closed and other tools")
FAIL_CLOSED_INPUTS = [
    ("empty stdin", ""),
    ("invalid json", "{bad"),
    ("json array", "[]"),
    ("json scalar", '"hello"'),
    ("empty object", "{}"),
    ("Edit without path", json.dumps({"tool_name": "Edit", "tool_input": {"new_string": "x"}})),
    ("Write with blank path", json.dumps({"tool_name": "Write", "tool_input": {"file_path": "   "}})),
    ("NotebookEdit without path", json.dumps({"tool_name": "NotebookEdit", "tool_input": {}})),
    ("apply_patch without envelope", codex_patch("just plain text")),
]
for label, raw in FAIL_CLOSED_INPUTS:
    res = run_script(PROTECTED_SCRIPT, stdin_text=raw)
    check(f"protected_paths fail-closed on {label}", res.returncode == 2 and "payload" in res.stderr.lower(), res.stderr)

res_other = run_script(
    PROTECTED_SCRIPT,
    stdin_text=json.dumps({"tool_name": "Read", "tool_input": {"file_path": ".env"}}),
)
check("protected_paths allows other tool_name", res_other.returncode == 0, res_other.stderr)

# ---------------------------------------------------------------------------
# 2. Event-time guard tests (SPEC 2)
# ---------------------------------------------------------------------------
print("event_time: GUARDED_GLOBS, wall-clock, GetLatest, comments, exemptions")
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
            res_w.returncode == 2
            and "ADR-000468" in res_w.stderr
            and "ADR-000447" in res_w.stderr
            and "OccurredAt" in res_w.stderr,
            res_w.stderr,
        )
        res_e = run_script(EVENT_TIME_SCRIPT, stdin_text=claude_edit(target, "x := 1", snippet))
        check(f"Edit blocks {call_name} in {pathlib.Path(target).name}", res_e.returncode == 2, res_e.stderr)

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
check("allows wall-clock/GetLatest in Go line comments", res_comment.returncode == 0, res_comment.stderr)

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
check("apply_patch allows time.Now in unguarded while touching clean guarded", res_p_ok.returncode == 0, res_p_ok.stderr)

# apply_patch touching one unguarded (clean) and one guarded (with GetLatestState) -> block
patch_bad_guarded = (
    "*** Begin Patch\n"
    f"*** Update File: {UNGUARDED_GO}\n@@\n+ts := ev.OccurredAt\n"
    f"*** Update File: {TRAIL_GO}\n@@\n+s := store.GetLatestState(ctx)\n"
    "*** End Patch\n"
)
res_p_bad = run_script(EVENT_TIME_SCRIPT, stdin_text=codex_patch(patch_bad_guarded))
check("apply_patch blocks GetLatestState in guarded file", res_p_bad.returncode == 2, res_p_bad.stderr)

for label, raw in FAIL_CLOSED_INPUTS:
    res = run_script(EVENT_TIME_SCRIPT, stdin_text=raw)
    check(f"event_time fail-closed on {label}", res.returncode == 2 and "payload" in res.stderr.lower(), res.stderr)

res_et_other = run_script(
    EVENT_TIME_SCRIPT,
    stdin_text=json.dumps({"tool_name": "Bash", "tool_input": {"command": "echo hi"}}),
)
check("event_time allows other tool_name", res_et_other.returncode == 0, res_et_other.stderr)

# ---------------------------------------------------------------------------
# 3. Selfcheck tests (SPEC 3)
# ---------------------------------------------------------------------------
print("harness_selfcheck: clean fixture and H1..H9 fixtures")

with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    res_clean = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check(
        "clean fixture exits 0 and notes absent .claude/settings.json",
        res_clean.returncode == 0 and "settings.json" in res_clean.stdout and "not checked" in res_clean.stdout.lower(),
        res_clean.stdout + res_clean.stderr,
    )

    res_hook_ok = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo), "--hook"])
    hook_lines = [ln for ln in res_hook_ok.stdout.splitlines() if ln.strip()]
    check(
        "--hook clean prints exactly one OK line and exits 0",
        res_hook_ok.returncode == 0 and len(hook_lines) == 1 and hook_lines[0].startswith("agent_harness_selfcheck: OK"),
        res_hook_ok.stdout,
    )

    # Adding a clean .claude/settings.json also passes with 0 findings.
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
                        ],
                    }
                ]
            }
        },
    )
    res_clean_claude = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("clean fixture with .claude/settings.json exits 0", res_clean_claude.returncode == 0, res_clean_claude.stdout)

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
        {"type": "command", "command": 'python3 "$(git rev-parse --show-toplevel)/scripts/does_not_exist.py"'}
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
                    }
                ]
            }
        },
    )
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H3 detected", res.returncode == 1 and "H3 " in res.stdout, res.stdout)

# H4: guard_event_time.py missing from .codex/hooks.json PreToolUse
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    cfg = json.loads((repo / ".codex" / "hooks.json").read_text(encoding="utf-8"))
    cfg["hooks"]["PreToolUse"][0]["hooks"] = [cfg["hooks"]["PreToolUse"][0]["hooks"][0]]
    (repo / ".codex" / "hooks.json").write_text(json.dumps(cfg), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H4 detected", res.returncode == 1 and "H4 " in res.stdout, res.stdout)

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
    check("H5 detected when only _test.go exists", res.returncode == 1 and "H5 " in res.stdout, res.stdout)

# H6: .claude/rules paths entry matches no tracked file
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    write_and_track(
        repo,
        ".claude/rules/dead.md",
        "---\npaths:\n  - \"nonexistent-dir/**/*.{go,py}\"\n---\nDead rule\n",
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
    check("H7 detected on mismatched block", res.returncode == 1 and "H7 " in res.stdout, res.stdout)

# H8: CLAUDE.md > 200 lines or AGENTS.md > 32768 bytes
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    base_claude = (repo / "CLAUDE.md").read_text(encoding="utf-8")
    (repo / "CLAUDE.md").write_text(base_claude + ("line\n" * 205), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H8 detected on CLAUDE.md > 200 lines", res.returncode == 1 and "H8 " in res.stdout, res.stdout)

with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    base_agents = (repo / "AGENTS.md").read_text(encoding="utf-8")
    (repo / "AGENTS.md").write_text(base_agents + ("x" * 33000), encoding="utf-8")
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H8 detected on AGENTS.md > 32768 bytes", res.returncode == 1 and "H8 " in res.stdout, res.stdout)

# H9: .agents/skills entry does not resolve to dir with SKILL.md
with tempfile.TemporaryDirectory() as tmp:
    repo = build_clean_selfcheck_repo(pathlib.Path(tmp))
    os.symlink(str(repo / "does-not-exist"), str(repo / ".agents" / "skills" / "broken-skill"))
    res = run_script(SELFCHECK_SCRIPT, args=["--root", str(repo)])
    check("H9 detected on broken symlink", res.returncode == 1 and "H9 " in res.stdout, res.stdout)

# Real repository check: expect no findings other than H7 while shared markers are missing.
print("harness_selfcheck: real repository check")
res_real = run_script(SELFCHECK_SCRIPT, args=["--root", str(ROOT)])
finding_lines = [
    ln
    for ln in res_real.stdout.splitlines()
    if len(ln) >= 3 and ln[0] == "H" and ln[1].isdigit() and ln[2] == " "
]
non_h7 = [ln for ln in finding_lines if not ln.startswith("H7 ")]
check(
    "real repo has no findings other than H7",
    non_h7 == [] and res_real.returncode in (0, 1),
    f"rc={res_real.returncode}, output={res_real.stdout}",
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
