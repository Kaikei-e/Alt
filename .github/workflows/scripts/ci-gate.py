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

DEFAULT_GATE_NAME = "ci-gate"
DEFAULT_POLL_INTERVAL = 30.0
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


def evaluate_check_runs(
    check_runs: Sequence[Mapping[str, Any]],
    gate_name: str = DEFAULT_GATE_NAME,
) -> tuple[str, list[str]]:
    """Return (PENDING|SUCCESS|FAILURE, offending_names) for a commit's check runs."""
    other_runs = [r for r in check_runs if r.get("name") != gate_name]

    pending_names = [
        str(r.get("name", "<unnamed>"))
        for r in other_runs
        if r.get("status") != "completed"
    ]
    if pending_names:
        return PENDING, pending_names

    failed_names = [
        str(r.get("name", "<unnamed>"))
        for r in other_runs
        if r.get("conclusion") in FAILURE_CONCLUSIONS
        or r.get("conclusion") not in SUCCESS_CONCLUSIONS
    ]
    if failed_names:
        return FAILURE, failed_names

    return SUCCESS, []


def _complete_set_signature(
    check_runs: Sequence[Mapping[str, Any]],
    gate_name: str,
) -> frozenset[tuple[str, Any]]:
    return frozenset(
        (str(r.get("name", "")), r.get("id"))
        for r in check_runs
        if r.get("name") != gate_name
    )


class _EndpointUrl(str):
    """URL string whose `in` operator matches `page=N` as a whole query parameter."""

    def strip(self, chars: str | None = None) -> _EndpointUrl:
        return _EndpointUrl(super().strip(chars))

    def __contains__(self, item: object) -> bool:
        if isinstance(item, str) and item.startswith("page="):
            _, _, query = self.partition("?")
            return item in query.split("&")
        return super().__contains__(item)


def fetch_check_runs(
    repo: str,
    sha: str,
    token: str,
    *,
    api_base: str = "https://api.github.com",
    per_page: int = 100,
    opener: Callable[..., Any] = urllib.request.urlopen,
) -> list[dict[str, Any]]:
    """Fetch all check runs for a commit SHA, paginating at per_page=100."""
    all_runs: list[dict[str, Any]] = []
    page = 1

    while True:
        url = _EndpointUrl(
            f"{api_base.rstrip('/')}/repos/{repo}/commits/{sha}/check-runs"
            f"?per_page={per_page}&page={page}"
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
        req._full_url = url

        payload: dict[str, Any] | None = None
        for attempt in range(1, 4):
            try:
                with opener(req, timeout=30) as resp:
                    payload = json.loads(resp.read().decode("utf-8"))
                break
            except urllib.error.HTTPError as exc:
                if exc.code >= 500 and attempt < 3:
                    time.sleep(2 * attempt)
                    continue
                raise
            except urllib.error.URLError:
                if attempt < 3:
                    time.sleep(2 * attempt)
                    continue
                raise

        assert payload is not None
        page_runs = payload.get("check_runs") or []
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
    gate_name: str = DEFAULT_GATE_NAME,
    poll_interval: float = DEFAULT_POLL_INTERVAL,
    timeout_seconds: float = DEFAULT_TIMEOUT_SECONDS,
    sleep_fn: Callable[[float], None] = time.sleep,
    clock_fn: Callable[[], float] = time.monotonic,
) -> tuple[str, list[str]]:
    """Poll check runs until failure, two consecutive identical complete sets, or timeout."""
    deadline = clock_fn() + timeout_seconds
    prev_complete_set: frozenset[tuple[str, Any]] | None = None
    poll_num = 0

    while True:
        poll_num += 1
        runs = fetch_runs()
        verdict, offending = evaluate_check_runs(runs, gate_name)

        if verdict == FAILURE:
            print(
                f"::error::ci-gate failed on poll #{poll_num}; failing check runs: "
                f"{', '.join(offending)}",
                file=sys.stderr,
            )
            return FAILURE, offending

        if verdict == SUCCESS:
            current_set = _complete_set_signature(runs, gate_name)
            if prev_complete_set is not None and current_set == prev_complete_set:
                print(
                    f"ci-gate: SUCCESS after {poll_num} polls "
                    f"({len(current_set)} check run(s) verified across two consecutive polls)."
                )
                return SUCCESS, []
            prev_complete_set = current_set
            print(
                f"ci-gate: poll #{poll_num} saw {len(current_set)} completed check run(s); "
                f"waiting {poll_interval:.0f}s to confirm stability."
            )
        else:
            prev_complete_set = None
            print(
                f"ci-gate: poll #{poll_num} PENDING on {len(offending)} check run(s): "
                f"{', '.join(offending)}"
            )

        now = clock_fn()
        if now + poll_interval > deadline:
            reason = (
                f"still pending: {', '.join(offending)}"
                if offending
                else "check run set did not stabilize across two consecutive polls"
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
        "--gate-name",
        default=os.environ.get("CI_GATE_NAME", DEFAULT_GATE_NAME),
        help="Check run name of this gate to ignore (default: ci-gate).",
    )
    parser.add_argument(
        "--poll-interval",
        type=float,
        default=DEFAULT_POLL_INTERVAL,
        help="Seconds between polls (default: 30).",
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

    verdict, _ = poll_until_settled(
        lambda: fetch_check_runs(args.repo, args.sha, token),
        gate_name=args.gate_name,
        poll_interval=args.poll_interval,
        timeout_seconds=args.timeout_seconds,
    )
    return 0 if verdict == SUCCESS else 1


if __name__ == "__main__":
    sys.exit(main())
