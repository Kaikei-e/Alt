#!/usr/bin/env python3
"""Branch-coverage tests for guard_ops_commands.py (exercising the Go alt-harness CLI)."""

from __future__ import annotations

import json
import pathlib
import subprocess
import sys

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
GUARD_PATH = REPO_ROOT / "scripts" / "agent-harness" / "guard_ops_commands.py"


def check(counts: list[int], name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        counts[0] += 1
    else:
        suffix = f" ({detail})" if detail else ""
        sys.stdout.write(f"  FAIL  {name}{suffix}\n")
        counts[1] += 1


def classify_via_cli(cmd: str) -> str | None:
    payload = json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "Bash",
            "tool_input": {"command": cmd},
        }
    )
    res = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [sys.executable, str(GUARD_PATH)],
        input=payload,
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode != 0:
        return f"EXIT_{res.returncode}:{res.stderr.strip()}"
    if not res.stdout.strip():
        return None
    parsed = json.loads(res.stdout)
    reason = parsed.get("hookSpecificOutput", {}).get("permissionDecisionReason", "")
    for rule in ("O1", "O2", "O3", "O4"):
        if f"Rule {rule} " in reason:
            return rule
    return f"UNKNOWN:{reason}"


CASES: list[tuple[str, str, str | None]] = [
    # DOUBLE state escapes and line continuations
    ("double quote escaped quote", 'sh -c "docker rm \\"x\\""', "O2"),
    ("double quote line continuation", 'sh -c "docker rm \\\nx"', "O2"),
    # NORMAL state backslash escapes, line continuations, trailing backslash
    ("normal backslash line continuation", "docker \\\nrm x", "O2"),
    ("normal escaped space in arg", "docker rm my\\ container", "O2"),
    (
        "normal escaped semicolon stops split (today verdict: O2 via token scan)",
        "echo a\\; docker rm x",
        "O2",
    ),
    ("normal trailing backslash", "docker ps\\", None),
    # NORMAL state comments and mid-token '#'
    ("comment line skips rm then allows ps", "# docker rm x\ndocker ps", None),
    ("comment line followed by rm asks O2", "# benign comment\ndocker rm x", "O2"),
    ("mid-token hash preserves token", "docker rm foo#1", "O2"),
    # NORMAL state subshell parens
    ("subshell parens split docker rm", "(docker rm x)", "O2"),
    # Substitutions branches
    (
        "escaped dollar-paren still split by parens (today verdict: O2)",
        "echo \\$(docker rm x)",
        "O2",
    ),
    ("single-quoted dollar-paren is not extracted", "echo '$(docker rm x)'", None),
    ("single quote inside $(...) protects inner quotes", "echo $(sh -c 'docker rm x')", "O2"),
    ("double quote + escape inside $(...)", 'echo $(sh -c "docker rm \\"x\\"")', "O2"),
    ("backslash escape inside $(...)", "echo $(echo \\) && docker rm x)", "O2"),
    ("nested parens inside $(...)", "echo $((1+1)) && echo $( (docker rm x) )", "O2"),
    ("backtick substitution", "echo `docker rm x`", "O2"),
    ("escaped backtick inside backticks", "echo `echo \\` && docker rm x`", "O2"),
    # Compose branches
    ("compose -- before subcmd classifies up as O3", "docker compose -- up -d", "O3"),
    (
        "compose -- before subcmd classifies up --build as O4",
        "docker compose -- up --build -d",
        "O4",
    ),
    ("compose -- before subcmd classifies down as O3", "docker compose -- down", "O3"),
    ("compose -- before subcmd classifies stop as O2", "docker compose -- stop alt-backend", "O2"),
    (
        "compose -- before subcmd classifies restart as O3",
        "docker compose -- restart alt-backend",
        "O3",
    ),
    (
        "compose -- after up treats --build as positional service name (O3)",
        "docker compose up -- --build",
        "O3",
    ),
    (
        "compose -- after up does not treat positional -p alt-test as safe project (O3)",
        "docker compose up -- -p alt-test",
        "O3",
    ),
    ("compose -- with no subcmd allows", "docker compose --", None),
    ("compose dangling -p falls back to default (O3)", "docker compose up -p", "O3"),
    (
        "compose dangling -p with staging env fails closed to O3",
        "COMPOSE_PROJECT_NAME=staging docker compose up -p",
        "O3",
    ),
    ("compose -p=alt-test allows", "docker compose -p=alt-test up -d", None),
    ("compose -p=alt asks O3", "docker compose -p=alt up -d", "O3"),
    ("compose empty -p='' treats as default O3", 'docker compose -p "" up -d', "O3"),
    (
        "compose empty -p='' does NOT fall back to env staging (O3)",
        'COMPOSE_PROJECT_NAME=staging docker compose -p "" up -d',
        "O3",
    ),
    (
        "compose empty --project-name='' does NOT fall back to env staging (O4)",
        'COMPOSE_PROJECT_NAME=staging docker compose --project-name="" up --build -d',
        "O4",
    ),
    (
        "compose empty -p='' equals form does NOT fall back to env staging (O3)",
        'COMPOSE_PROJECT_NAME=staging docker compose -p="" up -d',
        "O3",
    ),
    ("compose down --build is O3 not O4", "docker compose -p alt down --build", "O3"),
    ("compose restart on prod project asks O3", "docker compose -p alt restart alt-backend", "O3"),
    ("compose restart on default project asks O3", "docker compose restart alt-backend", "O3"),
    (
        "compose restart on isolated project allows",
        "docker compose -p alt-test restart alt-backend",
        None,
    ),
    # Docker segment branches
    ("docker -- before subcmd classifies rm as O2", "docker -- rm x", "O2"),
    ("docker -- before compose classifies up as O3", "docker -- compose -- up -d", "O3"),
    ("docker -- without subcmd allows", "docker --", None),
    ("docker boolean global flag before rm", "docker --debug rm x", "O2"),
    ("bare docker without subcmd allows", "docker", None),
    ("docker --help without subcmd allows", "docker --help", None),
    (
        "docker container -- before action classifies rm as O2",
        "docker container -- rm x",
        "O2",
    ),
    ("docker system -- before action classifies prune as O1", "docker system -- prune -af", "O1"),
    ("docker container flag before action", "docker container --help", None),
    ("docker container prune asks O1", "docker container prune -f", "O1"),
    ("docker volume prune asks O1", "docker volume prune -f", "O1"),
    ("docker network remove asks O1", "docker network remove alt-net", "O1"),
    ("docker container kill asks O2", "docker container kill alt-backend", "O2"),
    # Non-docker exe
    ("non-docker exe podman returns None", "podman rm x", None),
    # Quoted and ANSI-C command bypass hardening
    ("quoted double docker rm asks O2", '"docker" rm x', "O2"),
    ("quoted single docker rm asks O2", "'docker' rm x", "O2"),
    ("mid-word quoted docker rm asks O2", "d'o'cker rm x", "O2"),
    ("ANSI-C $'docker' rm asks O2", "$'docker' rm x", "O2"),
    ("ANSI-C hex $'\\x64ocker' rm asks O2", "$'\\x64ocker' rm x", "O2"),
    ('rg "docker rm" stays allowed', 'rg "docker rm" docs/', None),
    ('rg "docker" rm stays allowed', 'rg "docker" rm docs/', None),
]


def main() -> int:
    counts = [0, 0]
    sys.stdout.write("guard_ops_commands uncovered branch suite (Go CLI)\n")
    for label, cmd, expected in CASES:
        got = classify_via_cli(cmd)
        check(counts, label, got == expected, f"cmd={cmd!r} expected={expected!r} got={got!r}")

    sys.stdout.write(f"\n{counts[0]} passed, {counts[1]} failed\n")
    return 1 if counts[1] else 0


if __name__ == "__main__":
    sys.exit(main())
