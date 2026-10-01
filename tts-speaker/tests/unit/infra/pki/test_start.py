"""Start: loud disabled log, fail-fast enabled, no step CLI, shared secret rejected."""

from __future__ import annotations

import logging
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from tests.unit.infra.pki.test_manager import FakeIssuer
from tts_speaker.infra.pki.config import MODE_DISABLED, MODE_ENABLED, Config
from tts_speaker.infra.pki.start import start, start_with


def test_start_disabled_logs(monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_DISABLED)
    caplog.set_level(logging.INFO)
    handle = start("tts-speaker")
    assert handle is None
    assert "pki_enrollment_disabled" in caplog.text


def test_start_enabled_does_not_require_step_binary(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("CERT_SUBJECT", "tts-speaker")
    monkeypatch.setenv("STEP_BINARY", str(tmp_path / "no-such-step"))
    monkeypatch.setenv("STEP_CA_URL", "https://127.0.0.1:1")
    monkeypatch.setenv("STEP_CA_ROOT_FILE", str(tmp_path / "missing-root.pem"))
    monkeypatch.setenv(
        "STEP_CA_PROVISIONER_PASSWORD_FILE",
        str(tmp_path / "pki-agent-tts-speaker-jwk"),
    )
    with pytest.raises((OSError, RuntimeError, ValueError)) as excinfo:
        start("tts-speaker")
    assert "step CLI" not in str(excinfo.value)


def test_start_with_enabled_enrolls_and_stops(tmp_path: Path, caplog: pytest.LogCaptureFixture) -> None:
    nb = datetime.now(UTC) - timedelta(minutes=1)
    cfg = Config(
        mode=MODE_ENABLED,
        subject="tts-speaker",
        sans=("tts-speaker",),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-tts-speaker",
        password_file="/run/secrets/pki-agent-tts-speaker-jwk",
        renew_at_fraction=0.66,
        tick_interval=3600,
        retry_attempts=1,
        retry_backoff=0.001,
    )
    iss = FakeIssuer(not_before=nb, lifetime=timedelta(hours=24))
    caplog.set_level(logging.INFO)
    handle = start_with(cfg, iss)
    assert handle is not None
    try:
        assert Path(cfg.cert_path).is_file()
        assert "pki_enrollment_enabled" in caplog.text
        assert handle._thread.daemon is False
        assert handle._thread.name.startswith("pki-enrollment-")
    finally:
        handle.stop()
        handle.stop()


def test_start_enabled_shared_secret_rejected(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tts-speaker")
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
    with pytest.raises(ValueError):
        start("tts-speaker")
