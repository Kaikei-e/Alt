"""Prometheus enrollment metrics on the process /metrics registry."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from prometheus_client import REGISTRY as DEFAULT_REGISTRY
from prometheus_client import CollectorRegistry, generate_latest

from recap_subworker.app.infra.pki.config import MODE_ENABLED, Config
from recap_subworker.app.infra.pki.metrics import PromObserver
from recap_subworker.app.infra.pki.start import start_with_observer
from recap_subworker.app.infra.pki.state import State
from tests.unit.pki.test_manager import FakeIssuer


def test_prom_observer_exports_lifecycle_metrics() -> None:
    reg = CollectorRegistry()
    obs = PromObserver("recap-subworker", reg)
    obs.on_classified(State.FRESH, 8 * 3600)
    body = generate_latest(reg).decode()
    assert 'pki_enrollment_healthy{subject="recap-subworker"} 1.0' in body
    assert 'pki_enrollment_cert_remaining_seconds{subject="recap-subworker"} 28800.0' in body

    obs.on_renewed(True)
    obs.on_renewed(False)
    obs.on_reissued("expired")
    obs.on_classified(State.EXPIRED, -60)
    body = generate_latest(reg).decode()
    for needle in (
        'pki_enrollment_healthy{subject="recap-subworker"} 0.0',
        'pki_enrollment_renewal_total{result="success",subject="recap-subworker"} 1.0',
        'pki_enrollment_renewal_total{result="failure",subject="recap-subworker"} 1.0',
        'pki_enrollment_reissue_total{reason="expired",subject="recap-subworker"} 1.0',
    ):
        assert needle in body, body


def test_prom_observer_rejects_default_registry() -> None:
    with pytest.raises(TypeError, match="private"):
        PromObserver("recap-subworker-metrics-nil", DEFAULT_REGISTRY)


def test_start_with_observer_wires_prom_when_enabled(tmp_path: Path) -> None:
    nb = datetime.now(UTC) - timedelta(minutes=1)
    cfg = Config(
        mode=MODE_ENABLED,
        subject="recap-subworker-metrics",
        sans=("recap-subworker-metrics",),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-recap-subworker-metrics",
        password_file="/run/secrets/pki-agent-recap-subworker-metrics-jwk",
        renew_at_fraction=0.66,
        tick_interval=3600,
        retry_attempts=1,
        retry_backoff=0.001,
    )
    reg = CollectorRegistry()
    iss = FakeIssuer(not_before=nb, lifetime=timedelta(hours=24))
    handle = start_with_observer(cfg, iss, PromObserver(cfg.subject, reg))
    assert handle is not None
    try:
        body = generate_latest(reg).decode()
        assert 'pki_enrollment_healthy{subject="recap-subworker-metrics"}' in body
        assert " 1.0" in body or " 1" in body
    finally:
        handle.stop()
