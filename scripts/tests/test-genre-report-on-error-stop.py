#!/usr/bin/env python3
"""Unit test verifying run_sql_query passes -v ON_ERROR_STOP=1 to psql.

Run:
    python3 scripts/tests/test-genre-report-on-error-stop.py
"""

from __future__ import annotations

import importlib.util
import itertools
import pathlib
import subprocess
import sys
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "generate_genre_verification_report_docker.py"

spec = importlib.util.spec_from_file_location(
    "generate_genre_verification_report_docker", SCRIPT_PATH
)
assert spec is not None, "script missing"
assert spec.loader is not None, "script loader missing"
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


def main() -> int:
    """Assert run_sql_query includes -v ON_ERROR_STOP=1 in the psql command."""
    captured: list[list[str]] = []

    def fake_run(cmd: list[str], **_kwargs: object) -> subprocess.CompletedProcess[str]:
        captured.append(list(cmd))
        return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="[]", stderr="")

    with (
        mock.patch.object(mod.shutil, "which", return_value="/usr/bin/docker"),
        mock.patch.object(mod.subprocess, "run", side_effect=fake_run),
    ):
        res = mod.run_sql_query("SELECT 1", params={"hours": 24})

    if res != []:
        sys.stderr.write(f"FAIL: unexpected result: {res!r}\n")
        return 1
    if len(captured) != 1:
        sys.stderr.write(f"FAIL: expected 1 subprocess.run call, got {len(captured)}\n")
        return 1

    cmd = captured[0]
    if ("-v", "ON_ERROR_STOP=1") not in itertools.pairwise(cmd):
        sys.stderr.write(f"FAIL: missing ('-v', 'ON_ERROR_STOP=1') in psql cmd: {cmd!r}\n")
        return 1

    sys.stdout.write("  PASS  run_sql_query passes -v ON_ERROR_STOP=1 to psql\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
