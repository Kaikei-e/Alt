"""Cert remaining-lifetime classification. Pure function."""

from __future__ import annotations

from datetime import datetime
from enum import StrEnum


class CertState(StrEnum):
    """Leaf state against the 2/3-lifetime renewal policy."""

    MISSING = "missing"
    FRESH = "fresh"
    NEAR_EXPIRY = "near_expiry"
    EXPIRED = "expired"
    CORRUPT = "corrupt"


def classify_remaining(
    not_before: datetime,
    not_after: datetime,
    now: datetime,
    renew_at_fraction: float,
) -> CertState:
    """Identical inputs produce identical outputs. 0.66 = renew at 66% elapsed."""
    if now >= not_after:
        return CertState.EXPIRED
    total = (not_after - not_before).total_seconds()
    if total <= 0:
        return CertState.EXPIRED
    elapsed = (now - not_before).total_seconds()
    if elapsed / total >= renew_at_fraction:
        return CertState.NEAR_EXPIRY
    return CertState.FRESH
