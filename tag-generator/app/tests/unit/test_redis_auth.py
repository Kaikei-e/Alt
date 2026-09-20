"""REDIS_PASSWORD_FILE resolution for the Redis Streams consumer.

Mirrors the Go services' ResolveRedisPassword tests (e.g.
mq-hub/app/config/redis_auth_test.go): unset is the explicit "auth disabled"
mode (logged once), set-but-missing/empty is a startup error, never a silent
unauthenticated connection.
"""

import redis.asyncio as redis

from tag_generator.infra import redis_auth
from tag_generator.stream_consumer import ConsumerConfig, build_redis_client


def _reset_logged_once(monkeypatch) -> None:
    monkeypatch.setattr(redis_auth, "_disabled_logged", False)
    monkeypatch.delenv("REDIS_AUTH", raising=False)


def test_unset_raises_error_naming_both_variables(monkeypatch):
    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)
    _reset_logged_once(monkeypatch)

    try:
        redis_auth.resolve_redis_password()
        raise AssertionError("expected RuntimeError when neither variable is set")
    except RuntimeError as exc:
        assert "REDIS_PASSWORD_FILE" in str(exc)
        assert "REDIS_AUTH" in str(exc)


def test_explicit_disabled_returns_none_and_logs_once(monkeypatch):
    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)
    _reset_logged_once(monkeypatch)
    monkeypatch.setenv("REDIS_AUTH", "disabled")

    assert redis_auth.resolve_redis_password() is None
    assert redis_auth._disabled_logged is True

    # A second call in the same process must not log again -- the module-level
    # guard should still read True, not be reset by resolve_redis_password itself.
    assert redis_auth.resolve_redis_password() is None
    assert redis_auth._disabled_logged is True


def test_explicit_disabled_is_case_insensitive(monkeypatch):
    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)
    _reset_logged_once(monkeypatch)
    monkeypatch.setenv("REDIS_AUTH", "Disabled")

    assert redis_auth.resolve_redis_password() is None


def test_present_file_returns_trimmed_password(tmp_path, monkeypatch):
    pw_file = tmp_path / "redis_password.txt"
    pw_file.write_text("  secret-redis-password \n")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(pw_file))
    _reset_logged_once(monkeypatch)

    assert redis_auth.resolve_redis_password() == "secret-redis-password"


def test_missing_file_fails_fast(tmp_path, monkeypatch):
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(tmp_path / "does-not-exist.txt"))
    _reset_logged_once(monkeypatch)

    try:
        redis_auth.resolve_redis_password()
        raise AssertionError("expected RuntimeError for a missing REDIS_PASSWORD_FILE")
    except RuntimeError as exc:
        assert "REDIS_PASSWORD_FILE" in str(exc)


def test_empty_file_fails_fast(tmp_path, monkeypatch):
    pw_file = tmp_path / "empty_password.txt"
    pw_file.write_text("   \n\t ")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(pw_file))
    _reset_logged_once(monkeypatch)

    try:
        redis_auth.resolve_redis_password()
        raise AssertionError("expected RuntimeError for an empty REDIS_PASSWORD_FILE")
    except RuntimeError as exc:
        assert "empty password" in str(exc)


def test_env_set_to_empty_string_fails_fast(monkeypatch):
    monkeypatch.setenv("REDIS_PASSWORD_FILE", "")
    _reset_logged_once(monkeypatch)

    try:
        redis_auth.resolve_redis_password()
        raise AssertionError("expected RuntimeError for REDIS_PASSWORD_FILE=''")
    except RuntimeError as exc:
        assert "REDIS_PASSWORD_FILE is set but empty" in str(exc)


def test_consumer_config_from_env_carries_the_resolved_password(tmp_path, monkeypatch):
    pw_file = tmp_path / "redis_password.txt"
    pw_file.write_text("from-env-password")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(pw_file))
    _reset_logged_once(monkeypatch)

    config = ConsumerConfig.from_env()

    assert config.redis_password == "from-env-password"


def test_consumer_config_tags_stream_from_env_carries_the_resolved_password(tmp_path, monkeypatch):
    pw_file = tmp_path / "redis_password.txt"
    pw_file.write_text("from-tags-env-password")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(pw_file))
    _reset_logged_once(monkeypatch)

    config = ConsumerConfig.tags_stream_from_env()

    assert config.redis_password == "from-tags-env-password"


def test_build_redis_client_authenticates_with_the_resolved_password(monkeypatch):
    captured: dict[str, object] = {}

    def fake_from_url(url: str, **kwargs: object) -> object:
        captured["url"] = url
        captured.update(kwargs)
        return object()

    monkeypatch.setattr(redis, "from_url", fake_from_url)

    config = ConsumerConfig(enabled=True, redis_password="hunter2")
    build_redis_client(config, socket_timeout_seconds=15.0)

    assert captured["password"] == "hunter2"
    assert captured["url"] == config.redis_url


def test_build_redis_client_passes_none_when_auth_is_disabled(monkeypatch):
    captured: dict[str, object] = {}

    def fake_from_url(url: str, **kwargs: object) -> object:
        captured.update(kwargs)
        return object()

    monkeypatch.setattr(redis, "from_url", fake_from_url)

    config = ConsumerConfig(enabled=True)
    build_redis_client(config, socket_timeout_seconds=15.0)

    assert captured["password"] is None
