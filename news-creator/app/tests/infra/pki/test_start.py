"""Start: loud disabled log, fail-fast enabled, no step CLI, shared secret rejected."""

from __future__ import annotations

import logging
import socket
import threading
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from prometheus_client import CollectorRegistry

from news_creator.infra.pki.config import MODE_DISABLED, MODE_ENABLED, Config
from news_creator.infra.pki import start as start_module
from news_creator.infra.pki.ctx import Ctx
from news_creator.infra.pki.manager import NopObserver
from news_creator.infra.pki.start import start, start_with, start_with_observer
from tests.infra.pki.test_manager import FakeIssuer


def test_start_disabled_logs(
    monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_DISABLED)
    caplog.set_level(logging.INFO)
    handle = start("news-creator")
    assert handle is None
    assert "pki_enrollment_disabled" in caplog.text


def test_start_enabled_does_not_require_step_binary(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    monkeypatch.setenv("STEP_BINARY", str(tmp_path / "no-such-step"))
    monkeypatch.setenv("STEP_CA_URL", "https://127.0.0.1:1")
    monkeypatch.setenv("STEP_CA_ROOT_FILE", str(tmp_path / "missing-root.pem"))
    monkeypatch.setenv(
        "STEP_CA_PROVISIONER_PASSWORD_FILE",
        str(tmp_path / "pki-agent-news-creator-jwk"),
    )
    with pytest.raises((OSError, RuntimeError, ValueError)) as excinfo:
        start("news-creator")
    assert "step CLI" not in str(excinfo.value)


def test_start_with_enabled_enrolls_and_stops(
    tmp_path: Path, caplog: pytest.LogCaptureFixture
) -> None:
    nb = datetime.now(UTC) - timedelta(minutes=1)
    cfg = Config(
        mode=MODE_ENABLED,
        subject="news-creator",
        sans=("news-creator",),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-news-creator",
        password_file="/run/secrets/pki-agent-news-creator-jwk",
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
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    monkeypatch.setenv(
        "STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password"
    )
    with pytest.raises(ValueError):
        start("news-creator")


def test_ops_bind_failure_does_not_strand_renewal_thread(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    created: list[Ctx] = []

    class RecordingCtx(Ctx):
        def __init__(self, *, timeout: float | None = None) -> None:
            super().__init__(timeout=timeout)
            created.append(self)

    monkeypatch.setattr(start_module, "Ctx", RecordingCtx)
    cfg = Config(
        mode=MODE_ENABLED,
        subject="news-creator",
        sans=("news-creator",),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-news-creator",
        password_file="/run/secrets/pki-agent-news-creator-jwk",
        renew_at_fraction=0.66,
        tick_interval=3600,
        retry_attempts=1,
        retry_backoff=0.001,
    )
    thread_name = f"pki-enrollment-{cfg.subject}"
    with socket.create_server(("127.0.0.1", 0)) as taken:
        monkeypatch.setenv("OPS_LISTEN", f"127.0.0.1:{taken.getsockname()[1]}")
        try:
            with pytest.raises(OSError):
                start_with_observer(
                    cfg, FakeIssuer(), NopObserver(), registry=CollectorRegistry()
                )
            stranded = [t.name for t in threading.enumerate() if t.name == thread_name]
            assert stranded == []
        finally:
            for ctx in created:
                ctx.cancel()
            for t in threading.enumerate():
                if t.name == thread_name:
                    t.join(timeout=5)
