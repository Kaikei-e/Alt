"""Unit tests for settings."""

from __future__ import annotations

import pytest

from acolyte.config.settings import Settings

DEFAULT_SERVICE_PORT = 8090
DEFAULT_DB_POOL_MIN_SIZE = 2
DEFAULT_DB_POOL_MAX_SIZE = 10
DEFAULT_INBOUND_TLS_PORT = 9443
OVERRIDDEN_SERVICE_PORT = 9999


def test_settings_default_values() -> None:
    """Settings should have sensible defaults."""
    s = Settings()
    assert s.port == DEFAULT_SERVICE_PORT
    assert s.log_level == "info"
    assert s.db_pool_min_size == DEFAULT_DB_POOL_MIN_SIZE
    assert s.db_pool_max_size == DEFAULT_DB_POOL_MAX_SIZE
    assert s.worker_id == "acolyte-1"
    assert s.peer_identity_strict is False
    assert s.inbound_tls_enabled is False
    assert s.inbound_tls_port == DEFAULT_INBOUND_TLS_PORT


def test_peer_identity_strict_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    monkeypatch.setenv("ACOLYTE_DB_DSN", "postgresql://test:test@localhost/test")
    s = Settings()
    assert s.peer_identity_strict is True


def test_resolve_db_dsn_reads_password_via_path(tmp_path, monkeypatch: pytest.MonkeyPatch) -> None:  # noqa: ANN001
    pw_file = tmp_path / "db_password"
    pw_file.write_text("s3cret\n")
    monkeypatch.setenv("ACOLYTE_DB_DSN", "postgresql://user:PLACEHOLDER@localhost:5432/acolyte")
    s = Settings(acolyte_db_password_file=str(pw_file))
    dsn = s.resolve_db_dsn()
    assert "s3cret" in dsn
    assert "PLACEHOLDER" not in dsn


def test_settings_from_env(monkeypatch: object) -> None:
    """Settings should load from environment variables."""
    mp = pytest.importorskip("pytest").MonkeyPatch()
    mp.setenv("PORT", str(OVERRIDDEN_SERVICE_PORT))
    mp.setenv("ACOLYTE_DB_DSN", "postgresql://test:test@localhost/test")
    mp.setenv("NEWS_CREATOR_URL", "http://localhost:11434")

    s = Settings()
    assert s.port == OVERRIDDEN_SERVICE_PORT
    assert s.acolyte_db_dsn == "postgresql://test:test@localhost/test"
    assert s.news_creator_url == "http://localhost:11434"
    mp.undo()
