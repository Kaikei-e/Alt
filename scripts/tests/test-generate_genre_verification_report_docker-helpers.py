#!/usr/bin/env python3
"""Unit tests for pure helpers in scripts/generate_genre_verification_report_docker.py.

Run:
    python3 scripts/tests/test-generate_genre_verification_report_docker-helpers.py
"""

from __future__ import annotations

import importlib.util
import pathlib
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

COUNTS = {"passed": 0, "failed": 0}


def check(name: str, condition: bool, detail: str = "") -> None:
    """Record a single assertion result."""
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        COUNTS["passed"] += 1
        return
    sys.stdout.write(f"  FAIL  {name}\n")
    if detail:
        sys.stdout.write(f"        {detail}\n")
    COUNTS["failed"] += 1


def test_parse_psql_json_output_variants() -> None:
    """Normalizes empty, array, row_to_json, json_agg, and plain dict outputs."""
    check("empty output returns []", mod._parse_psql_json_output("") == [])
    check("json list returns list", mod._parse_psql_json_output('[{"a": 1}]') == [{"a": 1}])
    check(
        "row_to_json empty dict returns [{}]",
        mod._parse_psql_json_output('{"row_to_json": {}}') == [{}],
    )
    check(
        "row_to_json null returns []",
        mod._parse_psql_json_output('{"row_to_json": null}') == [],
    )
    check(
        "json_agg list returns inner list",
        mod._parse_psql_json_output('{"json_agg": [{"k": 2}]}') == [{"k": 2}],
    )
    check(
        "json_agg null returns []",
        mod._parse_psql_json_output('{"json_agg": null}') == [],
    )
    check(
        "plain dict wraps in list",
        mod._parse_psql_json_output('{"total": 5}') == [{"total": 5}],
    )


def test_build_psql_cmd_validates_params() -> None:
    """Rejects non-identifier psql parameter names and produces exact docker compose psql argv."""
    raised = False
    try:
        mod._build_psql_cmd("recap-db", "u", "db", {"bad-name": 1})
    except ValueError:
        raised = True
    check("invalid psql param name raises ValueError", raised)

    with mock.patch.object(mod.shutil, "which", return_value="/usr/bin/docker"):
        argv = mod._build_psql_cmd("recap-db", "recap_user", "recap", {"hours": 24, "limit": 20})
    expected = [
        "/usr/bin/docker",
        "compose",
        "exec",
        "-T",
        "recap-db",
        "psql",
        "-U",
        "recap_user",
        "-d",
        "recap",
        "-t",
        "-A",
        "-v",
        "hours=24",
        "-v",
        "limit=20",
    ]
    check("build_psql_cmd produces exact byte-identical argv", argv == expected, repr(argv))


def main() -> int:
    """Run all helper tests."""
    test_parse_psql_json_output_variants()
    test_build_psql_cmd_validates_params()
    sys.stdout.write(f"{COUNTS['passed']} passed, {COUNTS['failed']} failed\n")
    return 1 if COUNTS["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
