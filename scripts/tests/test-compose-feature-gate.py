#!/usr/bin/env python3
"""Tests for scripts/compose-feature-gate.py version parsing and report shape.

Live Docker probes are recorded in scripts/compose-feature-gate.report.json
from the operator host; these tests pin the version gate so a 5.0.0
desktop build cannot be reported as supporting pre_start.

Run:
    python3 scripts/tests/test-compose-feature-gate.py
"""

from __future__ import annotations

import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

spec = importlib.util.spec_from_file_location(
    "feature_gate", SCRIPTS / "compose-feature-gate.py"
)
assert spec is not None and spec.loader is not None
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)

PASS = 0
FAIL = 0


def check(name, condition):
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}")
        FAIL += 1


print("parse_compose_version")
check(
    "strips v and desktop suffix",
    gate.parse_compose_version("Docker Compose version v5.0.0-desktop.1")
    == (5, 0, 0),
)
check(
    "plain v5.4.0",
    gate.parse_compose_version("Docker Compose version v5.4.0") == (5, 4, 0),
)
check(
    "v2.29.1 still parses",
    gate.parse_compose_version("Docker Compose version v2.29.1") == (2, 29, 1),
)

print("pre_start_supported")
check("5.0.0 is below the floor", not gate.pre_start_supported((5, 0, 0)))
check("5.3.0 is below the floor", not gate.pre_start_supported((5, 3, 0)))
check("5.4.0 is the floor", gate.pre_start_supported((5, 4, 0)))
check("5.5.0 is above the floor", gate.pre_start_supported((5, 5, 0)))

print("gate_report")
report = gate.gate_report(
    "Docker Compose version v5.0.0-desktop.1",
    live={
        "pre_start_up_no_deps": {
            "supported": False,
            "detail": "validating compose: additional properties 'pre_start' not allowed",
        },
        "create_host_path_false_missing_file": {
            "refuses": True,
            "created_empty_dir": False,
        },
        "create_host_path_false_missing_directory": {
            "refuses": True,
            "created_empty_dir": False,
        },
    },
)
check("records the raw version string", report["compose_version"] == "Docker Compose version v5.0.0-desktop.1")
check("pre_start_ok is false on 5.0.0", report["pre_start"]["ok"] is False)
check(
    "create_host_path is not reported as a no-op when the live probe refused",
    report["create_host_path_false"]["ok"] is True
    and report["create_host_path_false"]["noop"] is False,
)
report_550 = gate.gate_report(
    "Docker Compose version v5.5.0",
    live={
        "pre_start_up_no_deps": {
            "supported": True,
            "detail": "up succeeded",
        },
        "create_host_path_false_missing_file": {
            "refuses": True,
            "created_empty_dir": False,
        },
        "create_host_path_false_missing_directory": {
            "refuses": True,
            "created_empty_dir": False,
        },
    },
)
check("pre_start_ok is true on 5.5.0 with a live probe", report_550["pre_start"]["ok"] is True)
check(
    "5.5.0 still records create_host_path as working",
    report_550["create_host_path_false"]["ok"] is True
    and report_550["create_host_path_false"]["noop"] is False,
)
check(
    "a missing live probe does not pretend the feature works",
    gate.gate_report("Docker Compose version v5.0.0-desktop.1", live=None)[
        "create_host_path_false"
    ]["ok"]
    is False,
)

print("committed_report_violations")
COMMITTED = json.loads(
    (SCRIPTS / "compose-feature-gate.report.json").read_text(encoding="utf-8")
)
unprobed = gate.gate_report("Docker Compose version v5.5.0", live=None)
check(
    "committed operator-host report holds the 5.4/5.5 contract",
    gate.committed_report_violations(COMMITTED) == [],
)
check(
    "committed report is the v5.5 measured contract",
    tuple(COMMITTED["compose_version_tuple"]) >= (5, 5, 0)
    and COMMITTED["pre_start"]["ok"] is True
    and COMMITTED["create_host_path_false"]["ok"] is True
    and COMMITTED["create_host_path_false"]["noop"] is False
    and COMMITTED["pre_start"]["live"] is not None,
)
check(
    "a live=None report is refused even on Compose 5.5.0",
    any("live" in v for v in gate.committed_report_violations(unprobed)),
)

corrupt_ok = json.loads(json.dumps(COMMITTED))
corrupt_ok["pre_start"]["ok"] = False
check(
    "pre_start.ok false is refused",
    any("pre_start.ok" in v for v in gate.committed_report_violations(corrupt_ok)),
)
corrupt_chp = json.loads(json.dumps(COMMITTED))
corrupt_chp["create_host_path_false"]["ok"] = False
check(
    "create_host_path_false.ok false is refused",
    any(
        "create_host_path_false.ok" in v
        for v in gate.committed_report_violations(corrupt_chp)
    ),
)
corrupt_noop = json.loads(json.dumps(COMMITTED))
corrupt_noop["create_host_path_false"]["noop"] = True
check(
    "create_host_path_false.noop true is refused",
    any("noop" in v for v in gate.committed_report_violations(corrupt_noop)),
)
corrupt_ver = json.loads(json.dumps(COMMITTED))
corrupt_ver["compose_version_tuple"] = [5, 3, 0]
check(
    "compose_version_tuple below 5.4.0 is refused",
    any("5.4" in v or "5, 4" in v for v in gate.committed_report_violations(corrupt_ver)),
)
corrupt_live = json.loads(json.dumps(COMMITTED))
corrupt_live["pre_start"]["live"] = None
check(
    "pre_start.live null is refused",
    any("live" in v for v in gate.committed_report_violations(corrupt_live)),
)
check(
    "a missing report object is refused",
    gate.committed_report_violations(None) != [],
)


def _gate_cli(report_path: pathlib.Path | None) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(SCRIPTS / "compose-feature-gate.py"), "--gate-report"]
    if report_path is not None:
        cmd.extend(["--committed-report", str(report_path)])
    return subprocess.run(cmd, capture_output=True, text=True, check=False)


print("--gate-report CLI")
cli_ok = _gate_cli(SCRIPTS / "compose-feature-gate.report.json")
check(
    "CLI exits 0 on the committed operator-host report",
    cli_ok.returncode == 0,
)
with tempfile.TemporaryDirectory(prefix="alt-gate-") as raw:
    tmp = pathlib.Path(raw)
    missing = tmp / "absent.json"
    cli_missing = _gate_cli(missing)
    check(
        "CLI exits nonzero when the committed report file is missing",
        cli_missing.returncode != 0,
    )
    bad = tmp / "bad.json"
    bad.write_text(json.dumps(corrupt_live), encoding="utf-8")
    cli_bad = _gate_cli(bad)
    check(
        "CLI exits nonzero on a live=null fixture",
        cli_bad.returncode != 0,
    )
    noop = tmp / "noop.json"
    noop.write_text(json.dumps(corrupt_noop), encoding="utf-8")
    cli_noop = _gate_cli(noop)
    check(
        "CLI exits nonzero when create_host_path_false.noop is true",
        cli_noop.returncode != 0,
    )

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
