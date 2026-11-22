#!/usr/bin/env python3
"""Unit tests for pure report-formatting helpers in scripts/check_japanese_summaries.py.

Run:
    python3 scripts/tests/test-check_japanese_summaries-helpers.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import pathlib
import sys
import types
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "check_japanese_summaries.py"

spec = importlib.util.spec_from_file_location("check_japanese_summaries", SCRIPT_PATH)
assert spec is not None, "script missing"
assert spec.loader is not None, "script loader missing"
mod = importlib.util.module_from_spec(spec)
with contextlib.ExitStack() as _stack:
    if importlib.util.find_spec("psycopg2") is None:
        psycopg2_stub = types.ModuleType("psycopg2")
        psycopg2_extras_stub = types.ModuleType("psycopg2.extras")
        psycopg2_extras_stub.RealDictCursor = object  # type: ignore[attr-defined]
        _stack.enter_context(
            patch.dict(
                sys.modules,
                {
                    "psycopg2": psycopg2_stub,
                    "psycopg2.extras": psycopg2_extras_stub,
                },
            )
        )
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


def test_generate_report_formatting() -> None:
    """Verifies summary counts, quality emoji thresholds, issues, and content preview."""
    summaries = [
        {
            "summary_id": 1,
            "article_id": "a1",
            "article_title": "Good Title",
            "created_at": "2026-01-01 00:00:00",
            "article_url": "https://example.com/1",
            "summary_japanese": "十分な長さのサマリー本文です。",
            "article_content": "本文テキスト",
            "quality": {"score": 80.0, "issues": []},
        },
        {
            "summary_id": 2,
            "article_id": "a2",
            "article_title": "Medium Title",
            "created_at": "2026-01-01 00:01:00",
            "summary_japanese": "短い要約",
            "quality": {"score": 60.0, "issues": ["短すぎる"]},
        },
        {
            "summary_id": 3,
            "article_id": "a3",
            "article_title": "Low Title",
            "created_at": "2026-01-01 00:02:00",
            "summary_japanese": "bad",
            "article_content": "x" * 600,
            "quality": {"score": 30.0, "issues": ["問題1", "問題2"]},
        },
    ]

    report = mod.generate_report(summaries, output_file=None)
    check("report counts target summaries", "**評価対象**: 3件" in report, report)
    check("report computes avg score 56.7", "**平均品質スコア**: 56.7/100" in report, report)
    check(
        "report tallies high/medium/low buckets",
        "**高品質（70点以上）**: 1件" in report
        and "**中品質（50-69点）**: 1件" in report
        and "**低品質（50点未満）**: 1件" in report,
        report,
    )
    check(
        "report uses score emojis",
        "🟢 80.0/100" in report and "🟡 60.0/100" in report and "🔴 30.0/100" in report,
        report,
    )
    check("report truncates long article content at 500 chars", ("x" * 500 + "...") in report)
    check(
        "report formats empty input without ZeroDivisionError",
        "**平均品質スコア**: 0.0/100" in mod.generate_report([]),
    )


def main() -> int:
    """Run all helper tests."""
    test_generate_report_formatting()
    sys.stdout.write(f"{COUNTS['passed']} passed, {COUNTS['failed']} failed\n")
    return 1 if COUNTS["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
