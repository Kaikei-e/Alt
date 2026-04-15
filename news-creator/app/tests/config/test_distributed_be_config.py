"""Tests for distributed BE dispatch configuration."""

from news_creator.config.config import NewsCreatorConfig


def test_distributed_be_defaults_off(monkeypatch):
    """DISTRIBUTED_BE_ENABLED defaults to false."""
    monkeypatch.delenv("DISTRIBUTED_BE_ENABLED", raising=False)
    monkeypatch.delenv("DISTRIBUTED_BE_REMOTES", raising=False)

    config = NewsCreatorConfig()

    assert config.distributed_be_enabled is False
    assert config.distributed_be_remotes == []
    assert config.distributed_be_health_interval_seconds == 30
    assert config.distributed_be_timeout_seconds == 300
    assert config.distributed_be_cooldown_seconds == 60
    assert config.distributed_be_remote_model == "gemma4-e4b-q4km"


def test_distributed_be_parses_remotes(monkeypatch):
    """CSV remote URLs are parsed into a list."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_REMOTES",
        "http://remote-a:11434,http://remote-b:11434",
    )

    config = NewsCreatorConfig()

    assert config.distributed_be_enabled is True
    assert config.distributed_be_remotes == [
        "http://remote-a:11434",
        "http://remote-b:11434",
    ]


def test_distributed_be_strips_whitespace_and_trailing_slash(monkeypatch):
    """URLs are cleaned: whitespace trimmed, trailing slashes removed."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_REMOTES",
        " http://host1:11434/ , http://host2:11434 ",
    )

    config = NewsCreatorConfig()

    assert config.distributed_be_remotes == [
        "http://host1:11434",
        "http://host2:11434",
    ]


def test_distributed_be_ignores_empty_elements(monkeypatch):
    """Empty elements in CSV are silently ignored."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_REMOTES",
        "http://host1:11434,,http://host2:11434,",
    )

    config = NewsCreatorConfig()

    assert config.distributed_be_remotes == [
        "http://host1:11434",
        "http://host2:11434",
    ]


def test_distributed_be_deduplicates_urls(monkeypatch):
    """Duplicate URLs are removed, keeping first occurrence."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_REMOTES",
        "http://host1:11434,http://host1:11434,http://host2:11434",
    )

    config = NewsCreatorConfig()

    assert config.distributed_be_remotes == [
        "http://host1:11434",
        "http://host2:11434",
    ]


def test_distributed_be_rejects_schemeless_urls(monkeypatch):
    """URLs without http:// or https:// are ignored."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_REMOTES",
        "remote-a:11434,http://host2:11434",
    )

    config = NewsCreatorConfig()

    assert config.distributed_be_remotes == ["http://host2:11434"]


def test_distributed_be_enabled_empty_remotes_warns(monkeypatch, caplog):
    """enabled=true with empty remotes logs a warning."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv("DISTRIBUTED_BE_REMOTES", "")

    import logging

    with caplog.at_level(logging.WARNING):
        config = NewsCreatorConfig()

    assert config.distributed_be_enabled is True
    assert config.distributed_be_remotes == []
    assert "effectively disabled" in caplog.text


def test_distributed_be_custom_intervals(monkeypatch):
    """Custom health interval, timeout, and cooldown are respected."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv("DISTRIBUTED_BE_REMOTES", "http://host:11434")
    monkeypatch.setenv("DISTRIBUTED_BE_HEALTH_INTERVAL_SECONDS", "15")
    monkeypatch.setenv("DISTRIBUTED_BE_TIMEOUT_SECONDS", "120")
    monkeypatch.setenv("DISTRIBUTED_BE_COOLDOWN_SECONDS", "30")

    config = NewsCreatorConfig()

    assert config.distributed_be_health_interval_seconds == 15
    assert config.distributed_be_timeout_seconds == 120
    assert config.distributed_be_cooldown_seconds == 30


def test_distributed_be_custom_remote_model(monkeypatch):
    """Remote model override is respected."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv("DISTRIBUTED_BE_REMOTES", "http://host:11434")
    monkeypatch.setenv("DISTRIBUTED_BE_REMOTE_MODEL", "gemma4-e4b-q4km")

    config = NewsCreatorConfig()

    assert config.distributed_be_remote_model == "gemma4-e4b-q4km"


def test_distributed_be_model_overrides(monkeypatch):
    """Per-remote model overrides are parsed."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_REMOTES", "http://host-a:11434,http://host-b:11434"
    )
    monkeypatch.setenv(
        "DISTRIBUTED_BE_MODEL_OVERRIDES",
        "http://host-b:11434=gemma4-e4b-rag",
    )

    config = NewsCreatorConfig()

    assert config.distributed_be_model_overrides == {
        "http://host-b:11434": "gemma4-e4b-rag",
    }


def test_distributed_be_model_overrides_empty_by_default(monkeypatch):
    """No overrides when env is empty."""
    monkeypatch.delenv("DISTRIBUTED_BE_MODEL_OVERRIDES", raising=False)

    config = NewsCreatorConfig()

    assert config.distributed_be_model_overrides == {}
