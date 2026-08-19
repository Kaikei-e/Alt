"""Leaf classification against the 2/3-lifetime renewal policy."""

from __future__ import annotations

from datetime import datetime
from enum import StrEnum


class State(StrEnum):
    MISSING = "missing"
    FRESH = "fresh"
    NEAR_EXPIRY = "near_expiry"
    EXPIRED = "expired"
    CORRUPT = "corrupt"


def classify_remaining(
    not_before: datetime, not_after: datetime, now: datetime, renew_at_fraction: float
) -> State:
    """Pure function: identical inputs produce identical outputs."""
    if now >= not_after:
        return State.EXPIRED
    total = (not_after - not_before).total_seconds()
    if total <= 0:
        return State.EXPIRED
    elapsed = (now - not_before).total_seconds()
    if elapsed / total >= renew_at_fraction:
        return State.NEAR_EXPIRY
    return State.FRESH
