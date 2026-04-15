"""Unit tests for settings."""

from __future__ import annotations

import os
import tempfile


def test_settings_default_values() -> None:
    """Settings should have sensible defaults."""
    from acolyte.config.settings import Settings

    s = Settings()
    assert s.port == 8090
    assert s.log_level == "info"
    assert s.db_pool_min_size == 2
    assert s.db_pool_max_size == 10
    assert s.worker_id == "acolyte-1"


def test_settings_from_env(monkeypatch: object) -> None:
    """Settings should load from environment variables."""
    import pytest

    mp = pytest.importorskip("pytest").MonkeyPatch()
    mp.setenv("PORT", "9999")
    mp.setenv("ACOLYTE_DB_DSN", "postgresql://test:test@localhost/test")
    mp.setenv("NEWS_CREATOR_URL", "http://localhost:11434")

    from acolyte.config.settings import Settings

    s = Settings()
    assert s.port == 9999
    assert s.acolyte_db_dsn == "postgresql://test:test@localhost/test"
    assert s.news_creator_url == "http://localhost:11434"
    mp.undo()


