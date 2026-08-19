"""Pure remaining-lifetime classification."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

from tag_generator.infra.pki.state import CertState, classify_remaining


def test_classify_remaining_table() -> None:
    nb = datetime(2026, 4, 16, tzinfo=UTC)
    na = nb + timedelta(hours=24)
    cases = [
        ("fresh at start", nb + timedelta(hours=1), 0.66, CertState.FRESH),
        ("fresh just before threshold", nb + timedelta(hours=15), 0.66, CertState.FRESH),
        ("near_expiry at 2/3", nb + timedelta(hours=16), 0.66, CertState.NEAR_EXPIRY),
        ("near_expiry past threshold", nb + timedelta(hours=20), 0.66, CertState.NEAR_EXPIRY),
        ("expired at not_after", na, 0.66, CertState.EXPIRED),
        ("expired after not_after", na + timedelta(minutes=1), 0.66, CertState.EXPIRED),
    ]
    for name, now, fraction, want in cases:
        assert classify_remaining(nb, na, now, fraction) is want, name


def test_classify_remaining_zero_window() -> None:
    nb = datetime(2026, 4, 16, tzinfo=UTC)
    assert classify_remaining(nb, nb, nb - timedelta(seconds=1), 0.66) is CertState.EXPIRED
