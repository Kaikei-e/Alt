#!/usr/bin/env python3
"""Unit tests for .github/workflows/scripts/ci-gate.py."""

from __future__ import annotations

from email.message import Message
import importlib.util
import io
import json
from pathlib import Path
import unittest
import urllib.error
import urllib.parse

SCRIPT_PATH = Path(__file__).resolve().parent / "ci-gate.py"


def load_ci_gate():
    if not SCRIPT_PATH.is_file():
        raise FileNotFoundError(f"Missing script under test: {SCRIPT_PATH}")
    spec = importlib.util.spec_from_file_location("ci_gate", SCRIPT_PATH)
    assert spec is not None and spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def make_http_error(url: str, code: int, headers: dict[str, str]) -> urllib.error.HTTPError:
    msg = Message()
    for k, v in headers.items():
        msg[k] = v
    return urllib.error.HTTPError(url, code, f"HTTP {code}", msg, io.BytesIO(b"{}"))


class TestEvaluateWorkflowRuns(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = load_ci_gate()

    def test_excludes_own_workflow_run_by_run_id(self) -> None:
        runs = [
            {
                "id": 999,
                "workflow_id": 10,
                "run_attempt": 1,
                "name": "ci-gate",
                "status": "in_progress",
                "conclusion": None,
                "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/999",
            },
            {
                "id": 1000,
                "workflow_id": 20,
                "run_attempt": 1,
                "name": "unit-test",
                "status": "completed",
                "conclusion": "success",
                "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/1000",
            },
        ]
        verdict, offending = self.mod.evaluate_workflow_runs(runs, self_run_id=999)
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])

        verdict_str, offending_str = self.mod.evaluate_workflow_runs(runs, self_run_id="999")
        self.assertEqual(verdict_str, "SUCCESS")
        self.assertEqual(offending_str, [])

    def test_any_uncompleted_run_returns_pending(self) -> None:
        for status in ("queued", "in_progress", "waiting", "requested", "pending"):
            with self.subTest(status=status):
                runs = [
                    {
                        "id": 999,
                        "workflow_id": 10,
                        "run_attempt": 1,
                        "name": "ci-gate",
                        "status": "in_progress",
                        "conclusion": None,
                        "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/999",
                    },
                    {
                        "id": 1001,
                        "workflow_id": 21,
                        "run_attempt": 1,
                        "name": "lint",
                        "status": "completed",
                        "conclusion": "success",
                        "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/1001",
                    },
                    {
                        "id": 1002,
                        "workflow_id": 22,
                        "run_attempt": 1,
                        "name": "build",
                        "status": status,
                        "conclusion": None,
                        "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/1002",
                    },
                ]
                verdict, offending = self.mod.evaluate_workflow_runs(runs, self_run_id=999)
                self.assertEqual(verdict, "PENDING")
                self.assertEqual(len(offending), 1)
                self.assertIn("build", offending[0])

    def test_pending_takes_precedence_while_any_run_incomplete(self) -> None:
        runs = [
            {
                "id": 2001,
                "workflow_id": 31,
                "run_attempt": 1,
                "name": "fast-fail",
                "status": "completed",
                "conclusion": "failure",
                "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/2001",
            },
            {
                "id": 2002,
                "workflow_id": 32,
                "run_attempt": 1,
                "name": "still-running",
                "status": "in_progress",
                "conclusion": None,
                "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/2002",
            },
        ]
        verdict, offending = self.mod.evaluate_workflow_runs(runs, self_run_id=999)
        self.assertEqual(verdict, "PENDING")
        self.assertEqual(len(offending), 1)
        self.assertIn("still-running", offending[0])

    def test_failure_conclusions_report_workflow_name_and_html_url(self) -> None:
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
                url = f"https://github.com/Kaikei-e/Alt/actions/runs/3000-{conclusion}"
                runs = [
                    {
                        "id": 999,
                        "workflow_id": 10,
                        "run_attempt": 1,
                        "name": "ci-gate",
                        "status": "in_progress",
                        "conclusion": None,
                        "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/999",
                    },
                    {
                        "id": 3001,
                        "workflow_id": 40,
                        "run_attempt": 1,
                        "name": "pass-wf",
                        "status": "completed",
                        "conclusion": "success",
                        "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/3001",
                    },
                    {
                        "id": 3002,
                        "workflow_id": 41,
                        "run_attempt": 1,
                        "name": f"bad-{conclusion}",
                        "status": "completed",
                        "conclusion": conclusion,
                        "html_url": url,
                    },
                ]
                verdict, offending = self.mod.evaluate_workflow_runs(runs, self_run_id=999)
                self.assertEqual(verdict, "FAILURE")
                self.assertEqual(len(offending), 1)
                self.assertIn(f"bad-{conclusion}", offending[0])
                self.assertIn(url, offending[0])

    def test_passing_conclusions_return_success(self) -> None:
        runs = [
            {
                "id": 999,
                "workflow_id": 10,
                "run_attempt": 1,
                "name": "ci-gate",
                "status": "in_progress",
                "conclusion": None,
            },
            {
                "id": 4001,
                "workflow_id": 51,
                "run_attempt": 1,
                "name": "ok-success",
                "status": "completed",
                "conclusion": "success",
            },
            {
                "id": 4002,
                "workflow_id": 52,
                "run_attempt": 1,
                "name": "ok-neutral",
                "status": "completed",
                "conclusion": "neutral",
            },
            {
                "id": 4003,
                "workflow_id": 53,
                "run_attempt": 1,
                "name": "ok-skipped",
                "status": "completed",
                "conclusion": "skipped",
            },
        ]
        verdict, offending = self.mod.evaluate_workflow_runs(runs, self_run_id=999)
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])


class TestPollingStabilityAndRateLimits(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = load_ci_gate()

    def test_default_poll_interval_is_60s_and_no_endpoint_url_subclass(self) -> None:
        self.assertEqual(self.mod.DEFAULT_POLL_INTERVAL, 60.0)
        self.assertFalse(hasattr(self.mod, "_EndpointUrl"))

    def test_requires_two_consecutive_identical_complete_sets_keyed_on_workflow_run_attempt(
        self,
    ) -> None:
        polls = [
            [
                {
                    "id": 999,
                    "workflow_id": 1,
                    "run_attempt": 1,
                    "name": "ci-gate",
                    "status": "in_progress",
                    "conclusion": None,
                },
                {
                    "id": 100,
                    "workflow_id": 2,
                    "run_attempt": 1,
                    "name": "wf-a",
                    "status": "completed",
                    "conclusion": "success",
                },
            ],
            # Poll 2: wf-a was re-run (run_attempt=2), so signature changed -> resets stability
            [
                {
                    "id": 999,
                    "workflow_id": 1,
                    "run_attempt": 1,
                    "name": "ci-gate",
                    "status": "in_progress",
                    "conclusion": None,
                },
                {
                    "id": 100,
                    "workflow_id": 2,
                    "run_attempt": 2,
                    "name": "wf-a",
                    "status": "completed",
                    "conclusion": "success",
                },
            ],
            # Poll 3: identical (workflow_id=2, id=100, run_attempt=2) -> stable SUCCESS
            [
                {
                    "id": 999,
                    "workflow_id": 1,
                    "run_attempt": 1,
                    "name": "ci-gate",
                    "status": "in_progress",
                    "conclusion": None,
                },
                {
                    "id": 100,
                    "workflow_id": 2,
                    "run_attempt": 2,
                    "name": "wf-a",
                    "status": "completed",
                    "conclusion": "success",
                },
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
            self_run_id=999,
            timeout_seconds=600.0,
            sleep_fn=sleeps.append,
            clock_fn=lambda: sum(sleeps),
        )
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])
        self.assertEqual(idx, 3)
        self.assertEqual(sleeps, [60.0, 60.0])

    def test_intervening_pending_poll_resets_stability(self) -> None:
        polls = [
            [
                {
                    "id": 10,
                    "workflow_id": 100,
                    "run_attempt": 1,
                    "name": "wf-a",
                    "status": "completed",
                    "conclusion": "success",
                }
            ],
            [
                {
                    "id": 10,
                    "workflow_id": 100,
                    "run_attempt": 1,
                    "name": "wf-a",
                    "status": "completed",
                    "conclusion": "success",
                },
                {
                    "id": 11,
                    "workflow_id": 101,
                    "run_attempt": 1,
                    "name": "wf-b",
                    "status": "in_progress",
                    "conclusion": None,
                },
            ],
            [
                {
                    "id": 10,
                    "workflow_id": 100,
                    "run_attempt": 1,
                    "name": "wf-a",
                    "status": "completed",
                    "conclusion": "success",
                },
                {
                    "id": 11,
                    "workflow_id": 101,
                    "run_attempt": 1,
                    "name": "wf-b",
                    "status": "completed",
                    "conclusion": "success",
                },
            ],
            [
                {
                    "id": 10,
                    "workflow_id": 100,
                    "run_attempt": 1,
                    "name": "wf-a",
                    "status": "completed",
                    "conclusion": "success",
                },
                {
                    "id": 11,
                    "workflow_id": 101,
                    "run_attempt": 1,
                    "name": "wf-b",
                    "status": "completed",
                    "conclusion": "success",
                },
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
            self_run_id=999,
            timeout_seconds=600.0,
            sleep_fn=sleeps.append,
            clock_fn=lambda: sum(sleeps),
        )
        self.assertEqual(verdict, "SUCCESS")
        self.assertEqual(offending, [])
        self.assertEqual(idx, 4)

    def test_timeout_while_pending_returns_failure(self) -> None:
        polls = [
            [
                {
                    "id": 10,
                    "workflow_id": 100,
                    "run_attempt": 1,
                    "name": "stuck-wf",
                    "status": "in_progress",
                    "conclusion": None,
                    "html_url": "https://github.com/Kaikei-e/Alt/actions/runs/10",
                }
            ]
        ]
        sleeps: list[float] = []

        verdict, offending = self.mod.poll_until_settled(
            lambda: polls[0],
            self_run_id=999,
            poll_interval=60.0,
            timeout_seconds=120.0,
            sleep_fn=sleeps.append,
            clock_fn=lambda: sum(sleeps),
        )
        self.assertEqual(verdict, "FAILURE")
        self.assertEqual(len(offending), 1)
        self.assertIn("stuck-wf", offending[0])

    def test_paginates_workflow_runs_only_when_total_count_exceeds_page(self) -> None:
        sha = "deadbeef" * 5
        page1 = {
            "total_count": 101,
            "workflow_runs": [
                {
                    "id": i,
                    "workflow_id": i,
                    "run_attempt": 1,
                    "name": f"wf-{i}",
                    "status": "completed",
                    "conclusion": "success",
                }
                for i in range(100)
            ],
        }
        page2 = {
            "total_count": 101,
            "workflow_runs": [
                {
                    "id": 100,
                    "workflow_id": 100,
                    "run_attempt": 1,
                    "name": "wf-100",
                    "status": "completed",
                    "conclusion": "success",
                }
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
            parsed = urllib.parse.urlsplit(url)
            qs = urllib.parse.parse_qs(parsed.query)
            if qs.get("page") == ["1"]:
                return FakeResponse(page1)
            if qs.get("page") == ["2"]:
                return FakeResponse(page2)
            raise AssertionError(f"Unexpected page query in {url}")

        runs = self.mod.fetch_workflow_runs(
            "Kaikei-e/Alt",
            sha,
            "test-token",
            opener=fake_urlopen,
        )
        self.assertEqual(len(runs), 101)
        self.assertEqual(len(seen_urls), 2)

        for expected_page, url in zip(("1", "2"), seen_urls, strict=True):
            parsed = urllib.parse.urlsplit(url)
            self.assertEqual(parsed.path, "/repos/Kaikei-e/Alt/actions/runs")
            qs = urllib.parse.parse_qs(parsed.query)
            self.assertEqual(qs.get("head_sha"), [sha])
            self.assertEqual(qs.get("per_page"), ["100"])
            self.assertEqual(qs.get("page"), [expected_page])

        # Single-page response (even with 100 items) must not request page 2
        single_page_urls: list[str] = []

        def fake_single_page(req, timeout=30):
            single_page_urls.append(req.full_url)
            return FakeResponse(
                {"total_count": 100, "workflow_runs": page1["workflow_runs"]}
            )

        runs_single = self.mod.fetch_workflow_runs(
            "Kaikei-e/Alt",
            sha,
            "test-token",
            opener=fake_single_page,
        )
        self.assertEqual(len(runs_single), 100)
        self.assertEqual(len(single_page_urls), 1)

    def test_waits_on_rate_limit_reset_or_retry_after_and_fails_other_4xx(self) -> None:
        sha = "cafebabe" * 5
        ok_payload = {
            "total_count": 1,
            "workflow_runs": [
                {
                    "id": 1,
                    "workflow_id": 1,
                    "run_attempt": 1,
                    "name": "wf-1",
                    "status": "completed",
                    "conclusion": "success",
                }
            ],
        }

        class FakeResponse:
            def read(self) -> bytes:
                return json.dumps(ok_payload).encode("utf-8")

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

        # 1) 403 with x-ratelimit-remaining: 0 and x-ratelimit-reset waits until reset
        sleeps_reset: list[float] = []
        calls_reset = 0

        def opener_reset(req, timeout=30):
            nonlocal calls_reset
            calls_reset += 1
            if calls_reset == 1:
                raise make_http_error(
                    req.full_url,
                    403,
                    {
                        "x-ratelimit-remaining": "0",
                        "x-ratelimit-reset": "1700000090",
                    },
                )
            return FakeResponse()

        runs = self.mod.fetch_workflow_runs(
            "Kaikei-e/Alt",
            sha,
            "test-token",
            opener=opener_reset,
            sleep_fn=sleeps_reset.append,
            time_fn=lambda: 1700000000.0,
            clock_fn=lambda: sum(sleeps_reset),
            deadline=300.0,
        )
        self.assertEqual(len(runs), 1)
        self.assertEqual(calls_reset, 2)
        self.assertEqual(sleeps_reset, [90.0])

        # 2) 429 with Retry-After waits retry-after seconds
        sleeps_retry: list[float] = []
        calls_retry = 0

        def opener_retry_after(req, timeout=30):
            nonlocal calls_retry
            calls_retry += 1
            if calls_retry == 1:
                raise make_http_error(req.full_url, 429, {"Retry-After": "45"})
            return FakeResponse()

        runs2 = self.mod.fetch_workflow_runs(
            "Kaikei-e/Alt",
            sha,
            "test-token",
            opener=opener_retry_after,
            sleep_fn=sleeps_retry.append,
            time_fn=lambda: 1700000000.0,
            clock_fn=lambda: sum(sleeps_retry),
            deadline=300.0,
        )
        self.assertEqual(len(runs2), 1)
        self.assertEqual(sleeps_retry, [45.0])

        # 3) Rate-limit wait exceeding deadline fails without sleeping past deadline
        sleeps_exceeded: list[float] = []

        def opener_exceeds_deadline(req, timeout=30):
            raise make_http_error(
                req.full_url,
                403,
                {
                    "x-ratelimit-remaining": "0",
                    "x-ratelimit-reset": "1700000500",
                },
            )

        with self.assertRaises(RuntimeError):
            self.mod.fetch_workflow_runs(
                "Kaikei-e/Alt",
                sha,
                "test-token",
                opener=opener_exceeds_deadline,
                sleep_fn=sleeps_exceeded.append,
                time_fn=lambda: 1700000000.0,
                clock_fn=lambda: 0.0,
                deadline=100.0,
            )
        self.assertEqual(sleeps_exceeded, [])

        # 4) Non-rate-limit 403 / 404 fails loudly immediately
        sleeps_4xx: list[float] = []

        def opener_forbidden(req, timeout=30):
            raise make_http_error(
                req.full_url, 403, {"x-ratelimit-remaining": "42"}
            )

        with self.assertRaises(urllib.error.HTTPError):
            self.mod.fetch_workflow_runs(
                "Kaikei-e/Alt",
                sha,
                "test-token",
                opener=opener_forbidden,
                sleep_fn=sleeps_4xx.append,
            )
        self.assertEqual(sleeps_4xx, [])


if __name__ == "__main__":
    unittest.main()
