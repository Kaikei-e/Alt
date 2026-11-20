"""Tests for the event-loop lag probe threshold configuration."""

import pytest

from news_creator.config.config import NewsCreatorConfig

pytestmark = pytest.mark.usefixtures("dummy_redis_password_file")


DEFAULT_EVENT_LOOP_LAG_WARN_MS = 500


def test_event_loop_lag_warn_ms_default(monkeypatch: pytest.MonkeyPatch) -> None:
    """EVENT_LOOP_LAG_WARN_MS defaults to 500."""
    monkeypatch.delenv("EVENT_LOOP_LAG_WARN_MS", raising=False)

    config = NewsCreatorConfig()

    assert config.event_loop_lag_warn_ms == DEFAULT_EVENT_LOOP_LAG_WARN_MS


def test_event_loop_lag_warn_ms_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """EVENT_LOOP_LAG_WARN_MS is read from the environment."""
    expected_warn_ms = 250
    monkeypatch.setenv("EVENT_LOOP_LAG_WARN_MS", str(expected_warn_ms))

    config = NewsCreatorConfig()

    assert config.event_loop_lag_warn_ms == expected_warn_ms
