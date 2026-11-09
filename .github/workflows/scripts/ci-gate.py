#!/usr/bin/env python3
"""Always-reporting merge gate verdict and polling script for GitHub Actions."""

from __future__ import annotations

import argparse
from collections.abc import Callable, Mapping, Sequence
import json
import os
import sys
import time
from typing import Any
import urllib.error
import urllib.request

PENDING = "PENDING"
SUCCESS = "SUCCESS"
FAILURE = "FAILURE"

DEFAULT_POLL_INTERVAL = 60.0
DEFAULT_TIMEOUT_SECONDS = 85 * 60.0

FAILURE_CONCLUSIONS = frozenset(
    {
        "failure",
        "timed_out",
        "cancelled",
        "action_required",
        "startup_failure",
        "stale",
    }
)
SUCCESS_CONCLUSIONS = frozenset(
    {
        "success",
        "neutral",
        "skipped",
    }
)


def _is_self_run(run: Mapping[str, Any], self_run_id: int | str | None) -> bool:
    if self_run_id is None or str(self_run_id) == "":
        return False
    return str(run.get("id")) == str(self_run_id)


def _format_run(run: Mapping[str, Any]) -> str:
    name = str(run.get("name") or run.get("display_title") or "<unnamed>")
    html_url = run.get("html_url")
    if isinstance(html_url, str) and html_url:
        return f"{name} ({html_url})"
    return name


def evaluate_workflow_runs(
    workflow_runs: Sequence[Mapping[str, Any]],
    self_run_id: int | str | None = None,
) -> tuple[str, list[str]]:
    """Return (PENDING|SUCCESS|FAILURE, offending_runs) for a commit's workflow runs."""
    other_runs = [r for r in workflow_runs if not _is_self_run(r, self_run_id)]

    pending_runs = [
        _format_run(r) for r in other_runs if r.get("status") != "completed"
    ]
    if pending_runs:
        return PENDING, pending_runs

    failed_runs = [
        _format_run(r)
        for r in other_runs
        if r.get("conclusion") in FAILURE_CONCLUSIONS
        or r.get("conclusion") not in SUCCESS_CONCLUSIONS
    ]
    if failed_runs:
        return FAILURE, failed_runs

    return SUCCESS, []


def _complete_set_signature(
    workflow_runs: Sequence[Mapping[str, Any]],
    self_run_id: int | str | None,
) -> frozenset[tuple[Any, Any, Any]]:
    return frozenset(
        (r.get("workflow_id"), r.get("id"), r.get("run_attempt", 1))
        for r in workflow_runs
        if not _is_self_run(r, self_run_id)
    )


def _rate_limit_wait_seconds(
    exc: urllib.error.HTTPError,
    time_fn: Callable[[], float],
) -> float | None:
    if exc.code not in (403, 429) or exc.headers is None:
        return None
    retry_after = exc.headers.get("retry-after")
    if retry_after is not None:
        try:
            return max(1.0, float(retry_after.strip()))
        except ValueError:
            return DEFAULT_POLL_INTERVAL

    remaining = exc.headers.get("x-ratelimit-remaining")
    if remaining is not None and remaining.strip() == "0":
        reset = exc.headers.get("x-ratelimit-reset")
        if reset is not None:
            try:
                return max(1.0, float(reset.strip()) - time_fn())
            except ValueError:
                return DEFAULT_POLL_INTERVAL
        return DEFAULT_POLL_INTERVAL

    return None


def fetch_workflow_runs(
    repo: str,
    sha: str,
    token: str,
    *,
    api_base: str = "https://api.github.com",
    per_page: int = 100,
    opener: Callable[..., Any] = urllib.request.urlopen,
    sleep_fn: Callable[[float], None] = time.sleep,
    time_fn: Callable[[], float] = time.time,
    clock_fn: Callable[[], float] = time.monotonic,
    deadline: float | None = None,
) -> list[dict[str, Any]]:
    """Fetch workflow runs for head_sha, paginating only when total_count exceeds a page."""
    all_runs: list[dict[str, Any]] = []
    page = 1

    while True:
        url = (
            f"{api_base.rstrip('/')}/repos/{repo}/actions/runs"
            f"?head_sha={sha}&per_page={per_page}&page={page}"
        )
        req = urllib.request.Request(
            url,
            headers={
                "Accept": "application/vnd.github+json",
                "Authorization": f"Bearer {token}",
                "X-GitHub-Api-Version": "2022-11-28",
                "User-Agent": "alt-ci-gate",
            },
            method="GET",
        )

        payload: dict[str, Any] | None = None
        transient_attempts = 0
        while True:
            try:
                with opener(req, timeout=30) as resp:
                    payload = json.loads(resp.read().decode("utf-8"))
                break
            except urllib.error.HTTPError as exc:
                wait_sec = _rate_limit_wait_seconds(exc, time_fn)
                if wait_sec is not None:
                    if deadline is not None and clock_fn() + wait_sec > deadline:
                        raise RuntimeError(
                            f"GitHub API rate limit reset in {wait_sec:.0f}s exceeds gate deadline."
                        ) from exc
                    print(
                        f"ci-gate: rate limited (HTTP {exc.code}); waiting {wait_sec:.0f}s for reset.",
                        file=sys.stderr,
                    )
                    sleep_fn(wait_sec)
                    continue
                transient_attempts += 1
                if exc.code >= 500 and transient_attempts < 3:
                    sleep_fn(2.0 * transient_attempts)
                    continue
                raise
            except urllib.error.URLError:
                transient_attempts += 1
                if transient_attempts < 3:
                    sleep_fn(2.0 * transient_attempts)
                    continue
                raise

        assert payload is not None
        page_runs = payload.get("workflow_runs") or []
        all_runs.extend(page_runs)

        total_count = payload.get("total_count")
        if not page_runs or len(page_runs) < per_page:
            break
        if isinstance(total_count, int) and len(all_runs) >= total_count:
            break
        page += 1

    return all_runs


def poll_until_settled(
    fetch_runs: Callable[[], Sequence[Mapping[str, Any]]],
    *,
    self_run_id: int | str | None = None,
    poll_interval: float = DEFAULT_POLL_INTERVAL,
    timeout_seconds: float = DEFAULT_TIMEOUT_SECONDS,
    sleep_fn: Callable[[float], None] = time.sleep,
    clock_fn: Callable[[], float] = time.monotonic,
) -> tuple[str, list[str]]:
    """Poll workflow runs until failure, two consecutive identical complete sets, or timeout."""
    deadline = clock_fn() + timeout_seconds
    prev_complete_set: frozenset[tuple[Any, Any, Any]] | None = None
    poll_num = 0

    while True:
        poll_num += 1
        try:
            runs = fetch_runs()
        except RuntimeError as exc:
            print(f"::error::ci-gate aborted: {exc}", file=sys.stderr)
            return FAILURE, []

        verdict, offending = evaluate_workflow_runs(runs, self_run_id)

        if verdict == FAILURE:
            print(
                f"::error::ci-gate failed on poll #{poll_num}; failing workflow runs: "
                f"{', '.join(offending)}",
                file=sys.stderr,
            )
            return FAILURE, offending

        if verdict == SUCCESS:
            current_set = _complete_set_signature(runs, self_run_id)
            if prev_complete_set is not None and current_set == prev_complete_set:
                print(
                    f"ci-gate: SUCCESS after {poll_num} polls "
                    f"({len(current_set)} workflow run(s) verified across two consecutive polls)."
                )
                return SUCCESS, []
            prev_complete_set = current_set
            print(
                f"ci-gate: poll #{poll_num} saw {len(current_set)} completed workflow run(s); "
                f"waiting {poll_interval:.0f}s to confirm stability."
            )
        else:
            prev_complete_set = None
            print(
                f"ci-gate: poll #{poll_num} PENDING on {len(offending)} workflow run(s): "
                f"{', '.join(offending)}"
            )

        now = clock_fn()
        if now + poll_interval > deadline:
            reason = (
                f"still pending: {', '.join(offending)}"
                if offending
                else "workflow run set did not stabilize across two consecutive polls"
            )
            print(
                f"::error::ci-gate timed out after {timeout_seconds:.0f}s ({reason}).",
                file=sys.stderr,
            )
            return FAILURE, offending

        sleep_fn(poll_interval)


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--repo",
        default=os.environ.get("GITHUB_REPOSITORY", ""),
        help="GitHub repository in owner/name format (default: $GITHUB_REPOSITORY).",
    )
    parser.add_argument(
        "--sha",
        default=os.environ.get("HEAD_SHA") or os.environ.get("GITHUB_SHA", ""),
        help="Commit SHA to inspect (default: $HEAD_SHA or $GITHUB_SHA).",
    )
    parser.add_argument(
        "--run-id",
        default=os.environ.get("GITHUB_RUN_ID", ""),
        help="Workflow run ID of this gate to exclude (default: $GITHUB_RUN_ID).",
    )
    parser.add_argument(
        "--poll-interval",
        type=float,
        default=DEFAULT_POLL_INTERVAL,
        help="Seconds between polls (default: 60).",
    )
    parser.add_argument(
        "--timeout-seconds",
        type=float,
        default=DEFAULT_TIMEOUT_SECONDS,
        help="Maximum seconds to poll before failing (default: 5100).",
    )
    args = parser.parse_args(argv)

    token = os.environ.get("GITHUB_TOKEN", "")
    if not args.repo:
        print("::error::Missing --repo / GITHUB_REPOSITORY.", file=sys.stderr)
        return 1
    if not args.sha:
        print("::error::Missing --sha / HEAD_SHA / GITHUB_SHA.", file=sys.stderr)
        return 1
    if not token:
        print("::error::Missing GITHUB_TOKEN environment variable.", file=sys.stderr)
        return 1

    deadline = time.monotonic() + args.timeout_seconds
    verdict, _ = poll_until_settled(
        lambda: fetch_workflow_runs(
            args.repo,
            args.sha,
            token,
            deadline=deadline,
        ),
        self_run_id=args.run_id or None,
        poll_interval=args.poll_interval,
        timeout_seconds=args.timeout_seconds,
    )
    return 0 if verdict == SUCCESS else 1


if __name__ == "__main__":
    sys.exit(main())
