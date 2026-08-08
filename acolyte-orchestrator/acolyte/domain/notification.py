"""Notification outbox domain rules.

The outbox row is a *signal*, not a delivery: it says a report is ready and
where to go and see it. Titles and section text stay out, so a device
notification never becomes a second, unversioned copy of the report.
"""

from __future__ import annotations

import random
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from datetime import datetime
    from uuid import UUID

REPORT_READY_KIND = "acolyte_report_ready"

# Full jitter: sleep = random(0, min(cap, base * 2**attempt)).
BACKOFF_BASE_SECONDS = 10.0
BACKOFF_CAP_SECONDS = 3600.0
MAX_ATTEMPTS = 8


@dataclass(frozen=True, slots=True)
class PendingNotification:
    """One claimed outbox row, as the relay sees it."""

    notification_id: UUID
    dedupe_key: str
    user_id: UUID
    kind: str
    payload: dict[str, str]
    occurred_at: datetime
    attempts: int


def report_ready_dedupe_key(run_id: UUID) -> str:
    """Derive the dedupe key from the run, never from the send attempt.

    A retry at any layer — the producer transaction replaying, the relay
    re-forwarding a row it already sent — re-derives the same key, which is
    what makes the downstream enqueue idempotent.
    """
    return f"acolyte:{run_id}"


def report_ready_payload(report_id: UUID) -> dict[str, str]:
    """A type discriminator and a navigate target. Nothing else."""
    return {"kind": REPORT_READY_KIND, "url": f"/acolyte/reports/{report_id}"}


def backoff_delay_seconds(attempt: int, *, rng: random.Random | None = None) -> float:
    """Full-jitter delay before retrying *attempt* (0 = first failure).

    Full jitter rather than exponential-with-jitter because every relay in the
    fleet retries the same DataHub endpoint: a narrow band around the same
    ceiling re-synchronises them into a thundering herd, a uniform draw does
    not.
    """
    ceiling = min(BACKOFF_CAP_SECONDS, BACKOFF_BASE_SECONDS * 2**attempt)
    draw = rng if rng is not None else random.SystemRandom()
    return draw.random() * ceiling


def is_exhausted(attempts: int) -> bool:
    """True once *attempts* recorded failures mean the row is dead."""
    return attempts >= MAX_ATTEMPTS
