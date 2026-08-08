"""Unit tests for the notification outbox domain rules.

These are the parts that must not drift: the dedupe key is derived from the
business fact (so a retry at any layer re-derives it), the payload is a signal
rather than a delivery channel, and the retry schedule is full jitter with a
hard death after a bounded number of attempts.
"""

from __future__ import annotations

import random
from uuid import UUID

import pytest

from acolyte.domain.notification import (
    BACKOFF_BASE_SECONDS,
    BACKOFF_CAP_SECONDS,
    MAX_ATTEMPTS,
    REPORT_READY_KIND,
    backoff_delay_seconds,
    is_exhausted,
    report_ready_dedupe_key,
    report_ready_payload,
)

_RUN_ID = UUID("11111111-1111-1111-1111-111111111111")
_REPORT_ID = UUID("22222222-2222-2222-2222-222222222222")


def test_dedupe_key_is_derived_from_the_run_not_the_attempt() -> None:
    first = report_ready_dedupe_key(_RUN_ID)
    second = report_ready_dedupe_key(_RUN_ID)
    assert first == second == f"acolyte:{_RUN_ID}"


def test_payload_carries_only_a_discriminator_and_a_navigate_target() -> None:
    payload = report_ready_payload(_REPORT_ID)
    assert set(payload) == {"kind", "url"}
    assert payload["kind"] == REPORT_READY_KIND
    assert payload["url"] == f"/acolyte/reports/{_REPORT_ID}"


def test_kind_is_the_agreed_wire_string() -> None:
    assert REPORT_READY_KIND == "acolyte_report_ready"


@pytest.mark.parametrize("attempt", [0, 1, 2, 3, 4, 5, 6, 7, 20])
def test_backoff_is_full_jitter_within_the_capped_window(attempt: int) -> None:
    rng = random.Random(1234)  # noqa: S311 — jitter, not a security decision
    ceiling = min(BACKOFF_CAP_SECONDS, BACKOFF_BASE_SECONDS * 2**attempt)
    for _ in range(200):
        delay = backoff_delay_seconds(attempt, rng=rng)
        assert 0.0 <= delay <= ceiling


def test_backoff_ceiling_doubles_per_attempt_until_the_cap() -> None:
    always_max = random.Random()  # noqa: S311
    always_max.random = lambda: 1.0  # type: ignore[method-assign]
    assert backoff_delay_seconds(0, rng=always_max) == BACKOFF_BASE_SECONDS
    assert backoff_delay_seconds(1, rng=always_max) == BACKOFF_BASE_SECONDS * 2
    assert backoff_delay_seconds(2, rng=always_max) == BACKOFF_BASE_SECONDS * 4
    assert backoff_delay_seconds(30, rng=always_max) == BACKOFF_CAP_SECONDS


def test_max_attempts_is_eight_and_exhaustion_is_inclusive() -> None:
    assert MAX_ATTEMPTS == 8
    assert is_exhausted(7) is False
    assert is_exhausted(8) is True
    assert is_exhausted(9) is True
