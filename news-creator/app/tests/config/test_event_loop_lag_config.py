"""Tests for the event-loop lag probe threshold configuration."""

import pytest

from news_creator.config.config import NewsCreatorConfig

pytestmark = pytest.mark.usefixtures("dummy_redis_password_file")


def test_event_loop_lag_warn_ms_default(monkeypatch):
    """EVENT_LOOP_LAG_WARN_MS defaults to 500."""
    monkeypatch.delenv("EVENT_LOOP_LAG_WARN_MS", raising=False)

    config = NewsCreatorConfig()

    assert config.event_loop_lag_warn_ms == 500


def test_event_loop_lag_warn_ms_from_env(monkeypatch):
    """EVENT_LOOP_LAG_WARN_MS is read from the environment."""
    monkeypatch.setenv("EVENT_LOOP_LAG_WARN_MS", "250")

    config = NewsCreatorConfig()

    assert config.event_loop_lag_warn_ms == 250
