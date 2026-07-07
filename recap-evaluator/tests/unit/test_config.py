"""Tests for Settings — RECAP_DB_PASSWORD_FILE (Docker secret) support.

recap-evaluator previously required a single pre-built RECAP_DB_DSN, which
forced compose/recap.yaml to interpolate the DB password directly into a
plaintext env var (visible via `docker inspect`). Settings must also accept
discrete RECAP_DB_HOST/PORT/USER/NAME + RECAP_DB_PASSWORD_FILE and build the
DSN internally, matching the pattern already used by recap-worker and
recap-db-migrator in the same compose file.
"""

import pytest
from pydantic import ValidationError

from recap_evaluator.config import Settings


class TestSettingsRecapDbDsn:
    def test_recap_db_dsn_env_var_passthrough(self, monkeypatch):
        """An explicitly provided RECAP_DB_DSN (tests/local dev) wins as-is."""
        monkeypatch.delenv("RECAP_DB_PASSWORD", raising=False)
        monkeypatch.delenv("RECAP_DB_PASSWORD_FILE", raising=False)
        monkeypatch.setenv("RECAP_DB_DSN", "postgres://explicit:dsn@host:5432/db")

        settings = Settings(_env_file=None)

        assert settings.recap_db_dsn == "postgres://explicit:dsn@host:5432/db"

    def test_builds_dsn_from_password_file(self, tmp_path, monkeypatch):
        """RECAP_DB_PASSWORD_FILE (Docker secret path) is read and combined
        with host/port/user/name into a DSN — password never touches the
        environment directly."""
        password_file = tmp_path / "recap_db_password"
        password_file.write_text("s3cr3t\n")

        monkeypatch.delenv("RECAP_DB_DSN", raising=False)
        monkeypatch.delenv("RECAP_DB_PASSWORD", raising=False)
        monkeypatch.setenv("RECAP_DB_PASSWORD_FILE", str(password_file))
        monkeypatch.setenv("RECAP_DB_HOST", "recap-db")
        monkeypatch.setenv("RECAP_DB_PORT", "5432")
        monkeypatch.setenv("RECAP_DB_USER", "recap_user")
        monkeypatch.setenv("RECAP_DB_NAME", "recap")

        settings = Settings(_env_file=None)

        assert settings.recap_db_dsn == (
            "postgres://recap_user:s3cr3t@recap-db:5432/recap"
        )

    def test_raises_when_no_dsn_and_no_password(self, monkeypatch):
        """Missing both RECAP_DB_DSN and a password source must fail fast at
        startup (CLAUDE.md rule 9) rather than silently building a DSN with
        no credentials."""
        monkeypatch.delenv("RECAP_DB_DSN", raising=False)
        monkeypatch.delenv("RECAP_DB_PASSWORD", raising=False)
        monkeypatch.delenv("RECAP_DB_PASSWORD_FILE", raising=False)

        with pytest.raises(ValidationError, match="RECAP_DB_DSN"):
            Settings(_env_file=None)
