"""Branch coverage tests for PKI Manager error logging paths."""

from __future__ import annotations

import logging
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from news_creator.infra.pki.certfile import CertFile
from news_creator.infra.pki.config import MODE_ENABLED, Config
from news_creator.infra.pki.ctx import Ctx
from news_creator.infra.pki.manager import Manager
from tests.infra.pki.test_certfile import new_self_signed_pem

SUBJECT = "news-creator"
CERT_LIFETIME_HOURS = 24
NEAR_EXPIRY_ELAPSED_HOURS = 16


class _FailingRekeyIssuer:
    def issue(self, _ctx: Ctx, _subject: str, _sans: list[str]) -> tuple[bytes, bytes]:
        msg = "issue failed"
        raise RuntimeError(msg)

    def rekey(
        self,
        _ctx: Ctx,
        _cert: bytes,
        _key: bytes,
        _subject: str,
        _sans: list[str],
    ) -> tuple[bytes, bytes]:
        msg = "rekey failed"
        raise RuntimeError(msg)


class _ValidRekeyIssuer:
    def __init__(self, not_before: datetime) -> None:
        self._not_before = not_before

    def issue(self, ctx: Ctx, subject: str, _sans: list[str]) -> tuple[bytes, bytes]:
        ctx.raise_if_cancelled()
        return new_self_signed_pem(
            subject,
            self._not_before,
            self._not_before + timedelta(hours=CERT_LIFETIME_HOURS),
        )

    def rekey(
        self,
        ctx: Ctx,
        _cert: bytes,
        _key: bytes,
        subject: str,
        sans: list[str],
    ) -> tuple[bytes, bytes]:
        return self.issue(ctx, subject, sans)


def _pki_config(tmp_path: Path) -> Config:
    return Config(
        mode=MODE_ENABLED,
        subject=SUBJECT,
        sans=(SUBJECT,),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-news-creator",
        password_file="/run/secrets/pki-agent-news-creator-jwk",  # noqa: S106 - secret path fixture
        renew_at_fraction=0.66,
        retry_attempts=1,
        retry_backoff=0.001,
        tick_interval=3600,
    )


def test_pki_manager_logs_exception_on_issue_write_and_rekey_failures(
    tmp_path: Path,
    caplog: pytest.LogCaptureFixture,
) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    cert, key = new_self_signed_pem(
        SUBJECT, nb, nb + timedelta(hours=CERT_LIFETIME_HOURS)
    )
    files.write(cert, key)

    rekey_fail_mgr = Manager(
        _pki_config(tmp_path),
        _FailingRekeyIssuer(),
        files,
        now=lambda: nb + timedelta(hours=NEAR_EXPIRY_ELAPSED_HOURS),
    )
    with (
        caplog.at_level(logging.ERROR),
        pytest.raises(RuntimeError, match="rekey cert"),
    ):
        rekey_fail_mgr.tick(Ctx())
    assert any("pki_enrollment_failed" in r.message for r in caplog.records)

    class _WriteFailCertFile(CertFile):
        def write(self, cert_pem: bytes, key_pem: bytes) -> None:
            del cert_pem, key_pem
            msg = "disk full"
            raise OSError(msg)

    caplog.clear()
    write_fail_files = _WriteFailCertFile(
        str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem")
    )
    write_fail_mgr = Manager(
        _pki_config(tmp_path),
        _ValidRekeyIssuer(nb),
        write_fail_files,
        now=lambda: nb + timedelta(hours=NEAR_EXPIRY_ELAPSED_HOURS),
    )
    with (
        caplog.at_level(logging.ERROR),
        pytest.raises(RuntimeError, match="write rekeyed cert"),
    ):
        write_fail_mgr.tick(Ctx())
    assert any("pki_enrollment_failed" in r.message for r in caplog.records)


class _UnlistedEnrollError(Exception):
    """Custom exception outside (OSError, RuntimeError, ValueError)."""


def test_enroll_retries_on_unlisted_tick_exception(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    cfg = Config(
        mode=MODE_ENABLED,
        subject=SUBJECT,
        sans=(SUBJECT,),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-news-creator",
        password_file="/run/secrets/pki-agent-news-creator-jwk",  # noqa: S106 - secret path fixture
        renew_at_fraction=0.66,
        retry_attempts=2,
        retry_backoff=0.001,
        tick_interval=3600,
    )
    files = CertFile(cfg.cert_path, cfg.key_path)
    issuer = _ValidRekeyIssuer(nb)
    mgr = Manager(cfg, issuer, files, now=lambda: nb)

    attempts = 0
    original_tick = mgr.tick

    def _flaky_tick(ctx: Ctx) -> object:
        nonlocal attempts
        attempts += 1
        if attempts == 1:
            msg = "transient non-std error"
            raise _UnlistedEnrollError(msg)
        return original_tick(ctx)

    mgr.tick = _flaky_tick  # type: ignore[method-assign]
    mgr.enroll(Ctx())
    expected_attempts = 2
    assert attempts == expected_attempts
