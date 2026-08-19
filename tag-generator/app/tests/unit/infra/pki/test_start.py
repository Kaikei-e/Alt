"""start_enrollment enabled/disabled logs and fail-fast."""

from __future__ import annotations

from collections.abc import Sequence
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from tag_generator.infra.pki.config import MODE_DISABLED, MODE_ENABLED, EnrollmentConfig, SharedRootSecretError
from tag_generator.infra.pki.start import start_enrollment, start_enrollment_with
from tests.unit.infra.pki.helpers import self_signed_pem


class _Log:
    def __init__(self) -> None:
        self.events: list[str] = []

    def info(self, event: str, **_kwargs: object) -> None:
        self.events.append(event)

    def error(self, event: str, **_kwargs: object) -> None:
        self.events.append(event)


class _FakeIssuer:
    def __init__(self) -> None:
        self.calls = 0

    async def issue(self, subject: str, sans: Sequence[str]) -> tuple[bytes, bytes]:
        self.calls += 1
        _ = sans
        now = datetime.now(tz=UTC)
        return self_signed_pem(subject, now - timedelta(minutes=1), now + timedelta(hours=24))


@pytest.mark.asyncio
async def test_start_disabled_logs(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_DISABLED)
    log = _Log()
    handle = await start_enrollment("tag-generator", logger=log)
    assert handle is None
    assert "pki_enrollment_disabled" in log.events


@pytest.mark.asyncio
async def test_start_enabled_requires_inbound_tls_true(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    monkeypatch.delenv("INBOUND_TLS_ENABLED", raising=False)
    with pytest.raises(RuntimeError, match="INBOUND_TLS_ENABLED"):
        await start_enrollment("tag-generator")


@pytest.mark.asyncio
async def test_start_enabled_rejects_inbound_tls_false(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    monkeypatch.setenv("INBOUND_TLS_ENABLED", "false")
    with pytest.raises(RuntimeError, match="INBOUND_TLS_ENABLED"):
        await start_enrollment("tag-generator")


@pytest.mark.asyncio
async def test_start_enabled_does_not_require_step_binary(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_TLS_ENABLED", "true")
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    monkeypatch.setenv("STEP_BINARY", str(tmp_path / "no-such-step"))
    monkeypatch.setenv("STEP_CA_URL", "https://127.0.0.1:1")
    monkeypatch.setenv("STEP_CA_ROOT_FILE", str(tmp_path / "missing-root.pem"))
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", str(tmp_path / "pki-agent-tag-generator-jwk"))
    with pytest.raises(Exception, match="pki:") as exc_info:
        await start_enrollment("tag-generator")
    assert "step CLI" not in str(exc_info.value)


@pytest.mark.asyncio
async def test_start_with_enabled_enrolls_and_stops(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("INBOUND_TLS_ENABLED", "true")
    cfg = EnrollmentConfig(
        mode=MODE_ENABLED,
        subject="tag-generator",
        sans=("tag-generator",),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-tag-generator",
        password_file="/run/secrets/pki-agent-tag-generator-jwk",
        renew_at_fraction=0.66,
        tick_interval_seconds=3600,
        retry_backoff_seconds=0.001,
        retry_attempts=1,
    )
    log = _Log()
    handle = await start_enrollment_with(cfg, _FakeIssuer(), logger=log)
    assert handle is not None
    try:
        assert Path(cfg.cert_path).is_file()  # noqa: ASYNC240
        assert "pki_enrollment_enabled" in log.events
    finally:
        await handle.aclose()


@pytest.mark.asyncio
async def test_start_enabled_shared_secret_rejected(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
    with pytest.raises(SharedRootSecretError):
        await start_enrollment("tag-generator")
