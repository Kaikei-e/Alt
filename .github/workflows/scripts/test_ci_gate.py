#!/usr/bin/env python3
"""Unit tests for .github/workflows/scripts/ci-gate.py."""

from __future__ import annotations

import importlib.util
import io
import json
from pathlib import Path
import unittest

SCRIPT_PATH = Path(__file__).resolve().parent / "ci-gate.py"


def load_ci_gate():
    if not SCRIPT_PATH.is_file():
        raise FileNotFoundError(f"Missing script under test: {SCRIPT_PATH}")
    spec = importlib.util.spec_from_file_location("ci_gate", SCRIPT_PATH)
    assert spec is not None and spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


class TestEvaluateCheckRuns(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = load_ci_gate()

    def test_ignores_own_check_run_when_in_progress(self) -> None:
        runs = [
            {"name": "ci-gate", "status": "in_progress", "conclusion": None},
            {"name": "unit-test", "status": "completed", "conclusion": "success"},
        ]
        verdict, offending = self.mod.evaluate_check_runs(runs, "ci-gate")
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])

    def test_any_uncompleted_run_returns_pending_with_names(self) -> None:
        for status in ("queued", "in_progress", "waiting", "requested", "pending"):
            with self.subTest(status=status):
                runs = [
                    {"name": "ci-gate", "status": "in_progress", "conclusion": None},
                    {"name": "lint", "status": "completed", "conclusion": "success"},
                    {"name": "build", "status": status, "conclusion": None},
                ]
                verdict, offending = self.mod.evaluate_check_runs(runs, "ci-gate")
                self.assertEqual(verdict, "PENDING")
                self.assertEqual(offending, ["build"])

    def test_pending_takes_precedence_while_any_run_incomplete(self) -> None:
        runs = [
            {"name": "fast-fail", "status": "completed", "conclusion": "failure"},
            {"name": "still-running", "status": "in_progress", "conclusion": None},
        ]
        verdict, offending = self.mod.evaluate_check_runs(runs, "ci-gate")
        self.assertEqual(verdict, "PENDING")
        self.assertEqual(offending, ["still-running"])

    def test_failure_conclusions_return_failure_with_offending_names(self) -> None:
        bad_conclusions = (
            "failure",
            "timed_out",
            "cancelled",
            "action_required",
            "startup_failure",
            "stale",
        )
        for conclusion in bad_conclusions:
            with self.subTest(conclusion=conclusion):
                runs = [
                    {"name": "ci-gate", "status": "in_progress", "conclusion": None},
                    {"name": "pass-job", "status": "completed", "conclusion": "success"},
                    {"name": f"bad-{conclusion}", "status": "completed", "conclusion": conclusion},
                ]
                verdict, offending = self.mod.evaluate_check_runs(runs, "ci-gate")
                self.assertEqual(verdict, "FAILURE")
                self.assertEqual(offending, [f"bad-{conclusion}"])

    def test_passing_conclusions_return_success(self) -> None:
        runs = [
            {"name": "ci-gate", "status": "in_progress", "conclusion": None},
            {"name": "ok-success", "status": "completed", "conclusion": "success"},
            {"name": "ok-neutral", "status": "completed", "conclusion": "neutral"},
            {"name": "ok-skipped", "status": "completed", "conclusion": "skipped"},
        ]
        verdict, offending = self.mod.evaluate_check_runs(runs, "ci-gate")
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])


class TestPollingStabilityRule(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = load_ci_gate()

    def test_requires_two_consecutive_identical_complete_sets_for_success(self) -> None:
        polls = [
            [
                {"id": 1, "name": "ci-gate", "status": "in_progress", "conclusion": None},
                {"id": 2, "name": "job-a", "status": "completed", "conclusion": "success"},
            ],
            [
                {"id": 1, "name": "ci-gate", "status": "in_progress", "conclusion": None},
                {"id": 2, "name": "job-a", "status": "completed", "conclusion": "success"},
            ],
        ]
        idx = 0
        sleeps: list[float] = []

        def fetch():
            nonlocal idx
            res = polls[idx]
            idx += 1
            return res

        verdict, offending = self.mod.poll_until_settled(
            fetch,
            gate_name="ci-gate",
            poll_interval=30.0,
            timeout_seconds=300.0,
            sleep_fn=sleeps.append,
            clock_fn=lambda: len(sleeps) * 30.0,
        )
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])
        self.assertEqual(idx, 2)
        self.assertEqual(sleeps, [30.0])

    def test_new_check_run_appearing_on_second_poll_resets_stability(self) -> None:
        polls = [
            # Poll 1: only job-a exists and is already complete
            [
                {"id": 10, "name": "job-a", "status": "completed", "conclusion": "success"},
            ],
            # Poll 2: job-b registered and also completed -> set changed, not stable yet
            [
                {"id": 10, "name": "job-a", "status": "completed", "conclusion": "success"},
                {"id": 11, "name": "job-b", "status": "completed", "conclusion": "success"},
            ],
            # Poll 3: same complete set {job-a, job-b} -> now stable
            [
                {"id": 10, "name": "job-a", "status": "completed", "conclusion": "success"},
                {"id": 11, "name": "job-b", "status": "completed", "conclusion": "success"},
            ],
        ]
        idx = 0
        sleeps: list[float] = []

        def fetch():
            nonlocal idx
            res = polls[idx]
            idx += 1
            return res

        verdict, offending = self.mod.poll_until_settled(
            fetch,
            gate_name="ci-gate",
            poll_interval=30.0,
            timeout_seconds=300.0,
            sleep_fn=sleeps.append,
            clock_fn=lambda: len(sleeps) * 30.0,
        )
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])
        self.assertEqual(idx, 3)
        self.assertEqual(sleeps, [30.0, 30.0])

    def test_intervening_pending_poll_requires_two_fresh_consecutive_polls(self) -> None:
        polls = [
            [{"id": 10, "name": "job-a", "status": "completed", "conclusion": "success"}],
            [
                {"id": 10, "name": "job-a", "status": "completed", "conclusion": "success"},
                {"id": 11, "name": "job-b", "status": "in_progress", "conclusion": None},
            ],
            [
                {"id": 10, "name": "job-a", "status": "completed", "conclusion": "success"},
                {"id": 11, "name": "job-b", "status": "completed", "conclusion": "success"},
            ],
            [
                {"id": 10, "name": "job-a", "status": "completed", "conclusion": "success"},
                {"id": 11, "name": "job-b", "status": "completed", "conclusion": "success"},
            ],
        ]
        idx = 0
        sleeps: list[float] = []

        def fetch():
            nonlocal idx
            res = polls[idx]
            idx += 1
            return res

        verdict, offending = self.mod.poll_until_settled(
            fetch,
            gate_name="ci-gate",
            poll_interval=30.0,
            timeout_seconds=300.0,
            sleep_fn=sleeps.append,
            clock_fn=lambda: len(sleeps) * 30.0,
        )
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])
        self.assertEqual(idx, 4)

    def test_timeout_while_pending_returns_failure_with_pending_names(self) -> None:
        polls = [
            [{"id": 10, "name": "stuck-job", "status": "in_progress", "conclusion": None}],
            [{"id": 10, "name": "stuck-job", "status": "in_progress", "conclusion": None}],
            [{"id": 10, "name": "stuck-job", "status": "in_progress", "conclusion": None}],
        ]
        idx = 0
        sleeps: list[float] = []

        def fetch():
            nonlocal idx
            res = polls[min(idx, len(polls) - 1)]
            idx += 1
            return res

        verdict, offending = self.mod.poll_until_settled(
            fetch,
            gate_name="ci-gate",
            poll_interval=30.0,
            timeout_seconds=60.0,
            sleep_fn=sleeps.append,
            clock_fn=lambda: len(sleeps) * 30.0,
        )
        self.assertEqual(verdict, "FAILURE")
        self.assertIn("stuck-job", offending)

    def test_paginates_check_runs_endpoint(self) -> None:
        page1 = {
            "total_count": 101,
            "check_runs": [
                {"id": i, "name": f"job-{i}", "status": "completed", "conclusion": "success"}
                for i in range(100)
            ],
        }
        page2 = {
            "total_count": 101,
            "check_runs": [
                {"id": 100, "name": "job-100", "status": "completed", "conclusion": "success"}
            ],
        }
        seen_urls: list[str] = []

        class FakeResponse:
            def __init__(self, payload: dict) -> None:
                self._buf = io.BytesIO(json.dumps(payload).encode("utf-8"))

            def read(self) -> bytes:
                return self._buf.read()

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

        def fake_urlopen(req, timeout=30):
            url = req.full_url
            seen_urls.append(url)
            if "page=1" in url:
                return FakeResponse(page1)
            return FakeResponse(page2)

        runs = self.mod.fetch_check_runs(
            "Kaikei-e/Alt",
            "deadbeef" * 5,
            "test-token",
            opener=fake_urlopen,
        )
        self.assertEqual(len(runs), 101)
        self.assertEqual(len(seen_urls), 2)
        self.assertIn("per_page=100", seen_urls[0])
        self.assertIn("page=1", seen_urls[0])
        self.assertIn("page=2", seen_urls[1])


if __name__ == "__main__":
    unittest.main()
