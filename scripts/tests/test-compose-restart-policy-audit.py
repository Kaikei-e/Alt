#!/usr/bin/env python3
"""Tests for scripts/compose-restart-policy-audit.py.

PM-2026-023 / ADR-000703: a long-running service without a restart policy (or
with restart: "no") stays absent indefinitely after stopping or container
removal. One-shot services (ephemeral as classified by
compose-ops-surface-audit.py) and profiled opt-in services are exempt.

Run:
    python3 scripts/tests/test-compose-restart-policy-audit.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
AUDIT_SCRIPT = SCRIPTS / "compose-restart-policy-audit.py"
sys.path.insert(0, str(SCRIPTS))

spec = importlib.util.spec_from_file_location("restart_policy_audit", AUDIT_SCRIPT)
assert spec is not None, "audit script missing"
assert spec.loader is not None, "audit script missing"
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()


def check(name: str, condition: bool) -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        sys.stdout.write(f"  FAIL  {name}\n")
        RESULTS.failed += 1


def write_tree(base: pathlib.Path, files: dict[str, str]) -> pathlib.Path:
    for rel, content in files.items():
        target = base / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
    return base / "compose" / "compose.yaml"


sys.stdout.write("compliant chain\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": ("include:\n  - core.yaml\n  - workers.yaml\n"),
            "compose/core.yaml": (
                "services:\n"
                "  alt-backend:\n"
                "    image: alt-backend\n"
                "    restart: always\n"
                "  pgbouncer:\n"
                "    image: pgbouncer\n"
                "    restart: unless-stopped\n"
            ),
            "compose/workers.yaml": (
                "services:\n"
                "  search-indexer:\n"
                "    image: search-indexer\n"
                "    restart: on-failure\n"
                "  tag-generator:\n"
                "    image: tag-generator\n"
                "    restart: on-failure:3\n"
            ),
        },
    )
    check(
        "compliant chain with always/unless-stopped/on-failure passes",
        audit.audit_compose(root) == [],
    )

sys.stdout.write("long-running service without restart\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": "include:\n  - workers.yaml\n",
            "compose/workers.yaml": ("services:\n  search-indexer:\n    image: search-indexer\n"),
        },
    )
    found = audit.audit_compose(root)
    check(
        "long-running service omitting restart fails",
        len(found) == 1 and "search-indexer" in found[0] and "workers.yaml" in found[0],
    )

sys.stdout.write("restart: no on a long-running service\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": (
                "services:\n"
                "  quoted-no:\n"
                "    image: app\n"
                '    restart: "no"\n'
                "  unquoted-no:\n"
                "    image: app\n"
                "    restart: no\n"
            ),
        },
    )
    found = audit.audit_compose(root)
    check(
        'quoted restart: "no" on a long-running service fails',
        any("quoted-no" in v for v in found),
    )
    check(
        "unquoted restart: no (YAML boolean false) on a long-running service fails",
        any("unquoted-no" in v for v in found),
    )

sys.stdout.write("exempt init service\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": "include:\n  - init.yaml\n",
            "compose/init.yaml": (
                "services:\n"
                "  migrate:\n"
                "    image: migrator\n"
                '    restart: "no"\n'
                "  step-ca-bootstrap:\n"
                "    image: bootstrap\n"
                "  custom-seeder:\n"
                "    image: seeder\n"
                "  app:\n"
                "    image: app\n"
                "    restart: always\n"
                "    depends_on:\n"
                "      custom-seeder:\n"
                "        condition: service_completed_successfully\n"
            ),
        },
    )
    check(
        "ephemeral migrator, bootstrap, and SCS target init services are exempt",
        audit.audit_compose(root) == [],
    )

sys.stdout.write("service inherited through an include\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": "include:\n  - sub/nested.yaml\n",
            "compose/sub/nested.yaml": "include:\n  - leaf.yaml\n",
            "compose/sub/leaf.yaml": ("services:\n  inherited-worker:\n    image: worker\n"),
        },
    )
    found = audit.audit_compose(root)
    check(
        "service without restart in nested include is caught with its file",
        len(found) == 1 and "inherited-worker" in found[0] and "leaf.yaml" in found[0],
    )

sys.stdout.write("profiled service\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": (
                "services:\n"
                "  restic-backup:\n"
                "    image: restic\n"
                "    profiles: [backup]\n"
                "  one-shot-runner:\n"
                "    image: grafana/k6\n"
                "    profiles: [perf]\n"
                "  profiled-migrator:\n"
                "    image: migrator\n"
                "    profiles: [tools]\n"
                "  api:\n"
                "    image: api\n"
                "    restart: always\n"
            ),
        },
    )
    found = audit.audit_compose(
        root,
        allowlist={"one-shot-runner": "runs one-shot load test command to completion"},
    )
    check(
        "profiled long-running service without restart is a violation",
        len(found) == 1 and "restic-backup" in found[0],
    )
    check(
        "allowlisted profiled one-shot passes",
        not any("one-shot-runner" in v for v in found),
    )
    check(
        "profiled ephemeral name (migrator) is exempt via shared rule",
        not any("profiled-migrator" in v for v in found),
    )

sys.stdout.write("allowlist handling\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": (
                'services:\n  special-svc:\n    image: app\n    restart: "no"\n'
            ),
        },
    )
    check(
        "ONE_SHOT_ALLOWLIST lists only genuine one-shots with reasons",
        set(audit.ONE_SHOT_ALLOWLIST) == {"alt-perf", "k6"}
        and all(isinstance(r, str) and bool(r.strip()) for r in audit.ONE_SHOT_ALLOWLIST.values()),
    )
    check(
        "explicit allowlist with reason exempts the service",
        audit.audit_compose(root, allowlist={"special-svc": "runs one-shot job to completion"})
        == [],
    )
    check(
        "allowlist entry without a reason does not exempt",
        len(audit.audit_compose(root, allowlist={"special-svc": "   "})) == 1,
    )

sys.stdout.write("unresolvable restart and unmodeled compose keys (R2/R3)\n")
with tempfile.TemporaryDirectory() as tmp:
    root = write_tree(
        pathlib.Path(tmp),
        {
            "compose/compose.yaml": (
                "services:\n"
                "  interp-restart:\n"
                "    image: app\n"
                "    restart: ${RESTART_POLICY:-no}\n"
                "  extends-svc:\n"
                "    extends:\n"
                "      file: base.yaml\n"
                "      service: base-app\n"
                "    restart: always\n"
                "  deploy-restart-svc:\n"
                "    image: app\n"
                "    deploy:\n"
                "      restart_policy:\n"
                "        condition: on-failure\n"
            ),
        },
    )
    found = audit.audit_compose(root, allowlist={})
    check(
        "${X:-no} restart policy is rejected as unresolvable asking for literal",
        any("interp-restart" in v and "literal" in v.lower() for v in found),
    )
    check(
        "extends: fails with explicit 'does not model' message",
        any("extends-svc" in v and "does not model" in v.lower() for v in found),
    )
    check(
        "deploy.restart_policy fails with explicit 'does not model' message",
        any("deploy-restart-svc" in v and "does not model" in v.lower() for v in found),
    )

sys.stdout.write("cli exit codes\n")
with tempfile.TemporaryDirectory() as tmp:
    clean_root = write_tree(
        pathlib.Path(tmp) / "ok",
        {
            "compose/compose.yaml": (
                "services:\n  api:\n    image: api\n    restart: unless-stopped\n"
            ),
        },
    )
    bad_root = write_tree(
        pathlib.Path(tmp) / "bad",
        {
            "compose/compose.yaml": ("services:\n  api:\n    image: api\n"),
        },
    )
    res_ok = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [sys.executable, str(AUDIT_SCRIPT), "--compose", str(clean_root)],
        capture_output=True,
        text=True,
        check=False,
    )
    res_bad = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [sys.executable, str(AUDIT_SCRIPT), "--compose", str(bad_root)],
        capture_output=True,
        text=True,
        check=False,
    )
    check("CLI exits 0 on clean compose", res_ok.returncode == 0)
    check(
        "CLI exits 1 on violation and prints service name",
        res_bad.returncode == 1 and "api" in res_bad.stdout,
    )

sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
