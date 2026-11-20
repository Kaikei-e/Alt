#!/usr/bin/env python3
"""Unit tests for pure helpers in scripts/analyze_docker_logs.py.

Run:
    python3 scripts/tests/test-analyze_docker_logs-helpers.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys
from datetime import UTC, datetime, timedelta, timezone

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "analyze_docker_logs.py"

spec = importlib.util.spec_from_file_location("analyze_docker_logs", SCRIPT_PATH)
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


def test_parse_log_line_datetimes() -> None:
    """Covers Z suffix, explicit offset, naive ISO, space-separated, and unparseable lines."""
    svc, ts, msg = mod.parse_log_line("recap-worker | 2025-01-15T10:30:45.123Z | ERROR: boom")
    check(
        "Z suffix produces UTC-aware datetime",
        svc == "recap-worker"
        and msg == "ERROR: boom"
        and ts is not None
        and ts.tzinfo is not None
        and ts == datetime(2025, 1, 15, 10, 30, 45, 123000, tzinfo=UTC),
        repr(ts),
    )

    svc2, ts2, msg2 = mod.parse_log_line("recap-subworker | 2025-01-15T19:30:45+09:00 | WARN: slow")
    jst = timezone(timedelta(hours=9))
    check(
        "explicit offset preserves aware offset",
        svc2 == "recap-subworker"
        and msg2 == "WARN: slow"
        and ts2 is not None
        and ts2.tzinfo is not None
        and ts2.utcoffset() == timedelta(hours=9)
        and ts2 == datetime(2025, 1, 15, 19, 30, 45, tzinfo=jst),
        repr(ts2),
    )

    svc3, ts3, _ = mod.parse_log_line("recap-worker | 2025-01-15T10:30:45.123 | INFO: naive iso")
    expected_local_iso = datetime(2025, 1, 15, 10, 30, 45, 123000).astimezone()
    check(
        "naive ISO becomes local-aware datetime",
        svc3 == "recap-worker"
        and ts3 is not None
        and ts3.tzinfo is not None
        and ts3 == expected_local_iso,
        repr(ts3),
    )

    svc4, ts4, _ = mod.parse_log_line("recap-worker | 2025-01-15 10:30:45 | INFO: space fmt")
    expected_local_space = datetime(2025, 1, 15, 10, 30, 45).astimezone()
    check(
        "space-separated timestamp becomes local-aware datetime",
        svc4 == "recap-worker"
        and ts4 is not None
        and ts4.tzinfo is not None
        and ts4 == expected_local_space,
        repr(ts4),
    )

    check(
        "aware datetimes across branches are comparable without TypeError",
        ts is not None and ts3 is not None and ts4 is not None and (ts3 < ts4 or ts3 >= ts4),
    )

    svc_bad_ts, ts_bad_ts, msg_bad_ts = mod.parse_log_line(
        "recap-worker | not-a-timestamp | INFO: bad ts"
    )
    check(
        "unparseable timestamp returns None timestamp",
        svc_bad_ts == "recap-worker" and ts_bad_ts is None and msg_bad_ts == "INFO: bad ts",
    )

    svc_unfmt, ts_unfmt, msg_unfmt = mod.parse_log_line("unstructured plain line")
    check(
        "unparseable line returns (None, None, raw line)",
        svc_unfmt is None and ts_unfmt is None and msg_unfmt == "unstructured plain line",
    )


def main() -> int:
    """Run all helper tests."""
    test_parse_log_line_datetimes()
    sys.stdout.write(f"{COUNTS['passed']} passed, {COUNTS['failed']} failed\n")
    return 1 if COUNTS["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
