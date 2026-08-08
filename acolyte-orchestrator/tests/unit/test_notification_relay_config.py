"""Startup configuration for the relay is fail-fast, never warn-and-limp.

A relay that starts without a target, without an identity to notify, or without
a client certificate cannot forward anything — and the failure would only show
up as a silently growing outbox. So it refuses to boot instead.
"""

from __future__ import annotations

from pathlib import Path
from uuid import UUID

import pytest

from acolyte.config.settings import Settings

_USER = "66666666-6666-6666-6666-666666666666"


@pytest.fixture(autouse=True)
def _base_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("ACOLYTE_DB_DSN", "postgresql://test:test@localhost/test")


def _write_mtls_material(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    for name, env in (
        ("cert.pem", "MTLS_CERT_FILE"),
        ("key.pem", "MTLS_KEY_FILE"),
        ("ca.pem", "MTLS_CA_FILE"),
    ):
        path = tmp_path / name
        path.write_text("x")
        monkeypatch.setenv(env, str(path))
    monkeypatch.setenv("MTLS_ENFORCE", "true")


def test_disabled_is_an_explicit_answer_not_an_error(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("NOTIFICATIONS_ENABLED", "false")
    assert Settings().resolve_notification_relay_config() is None


def test_enabled_without_a_user_refuses_to_start(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("NOTIFICATIONS_ENABLED", "true")
    monkeypatch.setenv("DATAHUB_URL", "https://alt-data-hub:9443")
    _write_mtls_material(tmp_path, monkeypatch)

    with pytest.raises(RuntimeError, match="NOTIFICATION_USER_ID"):
        Settings().resolve_notification_relay_config()


def test_a_malformed_user_id_is_rejected_at_startup(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("NOTIFICATIONS_ENABLED", "true")
    monkeypatch.setenv("NOTIFICATION_USER_ID", "not-a-uuid")
    monkeypatch.setenv("DATAHUB_URL", "https://alt-data-hub:9443")
    _write_mtls_material(tmp_path, monkeypatch)

    with pytest.raises(RuntimeError, match="NOTIFICATION_USER_ID"):
        Settings().resolve_notification_relay_config()


def test_enabled_without_a_target_refuses_to_start(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("NOTIFICATIONS_ENABLED", "true")
    monkeypatch.setenv("NOTIFICATION_USER_ID", _USER)
    monkeypatch.setenv("DATAHUB_URL", "")
    _write_mtls_material(tmp_path, monkeypatch)

    with pytest.raises(RuntimeError, match="DATAHUB_URL"):
        Settings().resolve_notification_relay_config()


def test_enabled_without_mtls_refuses_to_start(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("NOTIFICATIONS_ENABLED", "true")
    monkeypatch.setenv("NOTIFICATION_USER_ID", _USER)
    monkeypatch.setenv("DATAHUB_URL", "https://alt-data-hub:9443")
    _write_mtls_material(tmp_path, monkeypatch)
    monkeypatch.setenv("MTLS_ENFORCE", "false")

    # alt-data-hub always requires and verifies a client certificate, so a
    # relay with MTLS_ENFORCE=false would fail every single handshake.
    with pytest.raises(RuntimeError, match="MTLS_ENFORCE"):
        Settings().resolve_notification_relay_config()


def test_enabled_with_unreadable_client_cert_refuses_to_start(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    monkeypatch.setenv("NOTIFICATIONS_ENABLED", "true")
    monkeypatch.setenv("NOTIFICATION_USER_ID", _USER)
    monkeypatch.setenv("DATAHUB_URL", "https://alt-data-hub:9443")
    _write_mtls_material(tmp_path, monkeypatch)
    monkeypatch.setenv("MTLS_CERT_FILE", str(tmp_path / "missing.pem"))

    with pytest.raises(RuntimeError, match="MTLS_CERT_FILE"):
        Settings().resolve_notification_relay_config()


def test_a_fully_configured_relay_resolves(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("NOTIFICATIONS_ENABLED", "true")
    monkeypatch.setenv("NOTIFICATION_USER_ID", _USER)
    monkeypatch.setenv("DATAHUB_URL", "https://alt-data-hub:9443/")
    monkeypatch.setenv("NOTIFICATION_RELAY_BATCH_SIZE", "50")
    monkeypatch.setenv("NOTIFICATION_RELAY_INTERVAL_SECONDS", "2.5")
    monkeypatch.setenv("NOTIFICATION_TTL_SECONDS", "3600")
    _write_mtls_material(tmp_path, monkeypatch)

    cfg = Settings().resolve_notification_relay_config()

    assert cfg is not None
    assert cfg.user_id == UUID(_USER)
    assert cfg.datahub_url == "https://alt-data-hub:9443"
    assert cfg.batch_size == 50
    assert cfg.interval_seconds == 2.5
    assert cfg.ttl_seconds == 3600
    assert cfg.cert_file == str(tmp_path / "cert.pem")
    assert cfg.key_file == str(tmp_path / "key.pem")
    assert cfg.ca_file == str(tmp_path / "ca.pem")
