"""Tests for AsyncDatabaseManager."""

import pytest

from tag_generator.domain.errors import DatabaseConnectionError


class TestAsyncDatabaseManager:
    def test_pool_not_initialized_raises(self):
        from tag_generator.infra.async_database import AsyncDatabaseManager

        manager = AsyncDatabaseManager()
        with pytest.raises(DatabaseConnectionError, match="not initialized"):
            _ = manager.pool

    def test_build_dsn_from_config(self):
        from unittest.mock import MagicMock

        from tag_generator.infra.async_database import AsyncDatabaseManager

        mock_config = MagicMock()
        mock_config.tag_generator_user = "myuser"
        mock_config.tag_generator_password = "mypass"  # noqa: S105
        mock_config.tag_generator_password_file = None
        mock_config.host = "db.example.com"
        mock_config.port = 5433
        mock_config.name = "mydb"
        mock_config.sslmode = "require"

        manager = AsyncDatabaseManager(config=mock_config)
        dsn = manager._build_dsn()

        assert "myuser" in dsn
        assert "mypass" in dsn
        assert "db.example.com" in dsn
        assert "5433" in dsn
        assert "mydb" in dsn
        assert "sslmode=require" in dsn

    def test_build_dsn_from_env(self, monkeypatch):
        from tag_generator.infra.async_database import AsyncDatabaseManager

        monkeypatch.setenv("DB_TAG_GENERATOR_USER", "envuser")
        monkeypatch.setenv("DB_TAG_GENERATOR_PASSWORD", "envpass")
        monkeypatch.setenv("DB_HOST", "envhost")
        monkeypatch.setenv("DB_PORT", "5434")
        monkeypatch.setenv("DB_NAME", "envdb")

        manager = AsyncDatabaseManager()
        dsn = manager._build_dsn()

        assert "envuser" in dsn
        assert "envpass" in dsn
        assert "envhost" in dsn

    def test_build_dsn_no_password_raises(self, monkeypatch):
        from tag_generator.infra.async_database import AsyncDatabaseManager

        monkeypatch.delenv("DB_TAG_GENERATOR_PASSWORD", raising=False)
        monkeypatch.delenv("DB_TAG_GENERATOR_PASSWORD_FILE", raising=False)

        manager = AsyncDatabaseManager()
        with pytest.raises(DatabaseConnectionError, match="No database password"):
            manager._build_dsn()
