"""pki_enrollment_* metric family."""

from __future__ import annotations

from collections.abc import Sequence
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from prometheus_client import CollectorRegistry, generate_latest

from tag_generator.infra.pki.config import MODE_ENABLED, EnrollmentConfig
from tag_generator.infra.pki.metrics import PromObserver
from tag_generator.infra.pki.start import start_enrollment_with
from tag_generator.infra.pki.state import CertState
from tests.unit.infra.pki.helpers import self_signed_pem


def test_prom_observer_exports_lifecycle_metrics() -> None:
    reg = CollectorRegistry()
    obs = PromObserver("tag-generator", reg)
    obs.on_classified(CertState.FRESH, timedelta(hours=8))
    body = generate_latest(reg).decode()
    assert 'pki_enrollment_healthy{subject="tag-generator"} 1.0' in body
    assert 'pki_enrollment_cert_remaining_seconds{subject="tag-generator"} 28800.0' in body

    obs.on_renewed(True)
    obs.on_renewed(False)
    obs.on_reissued("expired")
    obs.on_classified(CertState.EXPIRED, timedelta(minutes=-1))
    body = generate_latest(reg).decode()
    assert 'pki_enrollment_healthy{subject="tag-generator"} 0.0' in body
    assert 'pki_enrollment_renewal_total{result="success",subject="tag-generator"} 1.0' in body
    assert 'pki_enrollment_renewal_total{result="failure",subject="tag-generator"} 1.0' in body
    assert 'pki_enrollment_reissue_total{reason="expired",subject="tag-generator"} 1.0' in body


@pytest.mark.asyncio
async def test_start_with_observer_wires_prom_when_enabled(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("INBOUND_TLS_ENABLED", "true")
    cfg = EnrollmentConfig(
        mode=MODE_ENABLED,
        subject="tag-generator-metrics",
        sans=("tag-generator-metrics",),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-tag-generator-metrics",
        password_file="/run/secrets/pki-agent-tag-generator-metrics-jwk",
        renew_at_fraction=0.66,
        tick_interval_seconds=3600,
        retry_backoff_seconds=0.001,
        retry_attempts=1,
    )
    reg = CollectorRegistry()

    class _Issuer:
        async def issue(self, subject: str, sans: Sequence[str]) -> tuple[bytes, bytes]:
            _ = sans
            now = datetime.now(tz=UTC)
            return self_signed_pem(subject, now - timedelta(minutes=1), now + timedelta(hours=24))

    handle = await start_enrollment_with(cfg, _Issuer(), observer=PromObserver(cfg.subject, reg))
    assert handle is not None
    try:
        body = generate_latest(reg).decode()
        assert 'pki_enrollment_healthy{subject="tag-generator-metrics"} 1.0' in body
    finally:
        await handle.aclose()
