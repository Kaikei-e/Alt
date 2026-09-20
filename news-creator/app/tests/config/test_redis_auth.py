"""REDIS_PASSWORD_FILE resolution for the news-creator cache gateway.

Mirrors the Go services' ResolveRedisPassword tests (e.g.
mq-hub/app/config/redis_auth_test.go) and tag-generator's own copy: unset is
the explicit "auth disabled" mode (logged once), set-but-missing/empty is a
startup error, never a silent unauthenticated connection.
"""

import pytest

from news_creator.config import redis_auth


@pytest.fixture(autouse=True)
def _reset_logged_once(monkeypatch):
    monkeypatch.setattr(redis_auth, "_disabled_logged", False)
    monkeypatch.delenv("REDIS_AUTH", raising=False)


def test_unset_raises_error_naming_both_variables(monkeypatch):
    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)

    with pytest.raises(RuntimeError) as exc_info:
        redis_auth.resolve_redis_password()
    assert "REDIS_PASSWORD_FILE" in str(exc_info.value)
    assert "REDIS_AUTH" in str(exc_info.value)


def test_explicit_disabled_returns_none_and_logs_once(monkeypatch):
    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)
    monkeypatch.setenv("REDIS_AUTH", "disabled")

    assert redis_auth.resolve_redis_password() is None
    assert redis_auth._disabled_logged is True

    # A second call in the same process must not need to log again.
    assert redis_auth.resolve_redis_password() is None


def test_explicit_disabled_is_case_insensitive(monkeypatch):
    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)
    monkeypatch.setenv("REDIS_AUTH", "Disabled")

    assert redis_auth.resolve_redis_password() is None


def test_present_file_returns_trimmed_password(tmp_path, monkeypatch):
    pw_file = tmp_path / "redis_password.txt"
    pw_file.write_text("  secret-redis-password \n")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(pw_file))

    assert redis_auth.resolve_redis_password() == "secret-redis-password"


def test_missing_file_fails_fast(tmp_path, monkeypatch):
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(tmp_path / "does-not-exist.txt"))

    with pytest.raises(RuntimeError, match="REDIS_PASSWORD_FILE"):
        redis_auth.resolve_redis_password()


def test_empty_file_fails_fast(tmp_path, monkeypatch):
    pw_file = tmp_path / "empty_password.txt"
    pw_file.write_text("   \n\t ")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(pw_file))

    with pytest.raises(RuntimeError, match="empty password"):
        redis_auth.resolve_redis_password()


def test_env_set_to_empty_string_fails_fast(monkeypatch):
    monkeypatch.setenv("REDIS_PASSWORD_FILE", "")

    with pytest.raises(RuntimeError, match="REDIS_PASSWORD_FILE is set but empty"):
        redis_auth.resolve_redis_password()
