"""Tests for distributed BE dispatch configuration (OpenAI-compatible targets)."""

import dataclasses
import pytest

from news_creator.config.config import NewsCreatorConfig

pytestmark = pytest.mark.usefixtures("dummy_redis_password_file")


def test_remote_target_is_frozen_dataclass_with_key():
    """RemoteTarget is a frozen dataclass exposing base_url, model, num_ctx, and key."""
    from news_creator.config.distributed_be_config import RemoteTarget

    target = RemoteTarget(
        base_url="http://router.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )

    assert dataclasses.is_dataclass(target)
    assert target.base_url == "http://router.example:8080"
    assert target.model == "provider-a/model-a:latest"
    assert target.num_ctx == 32768
    assert target.key == "http://router.example:8080|provider-a/model-a:latest"

    with pytest.raises(dataclasses.FrozenInstanceError):
        target.num_ctx = 16384  # type: ignore[misc]


def test_parse_distributed_be_targets_parses_entries_and_strips_trailing_slash():
    """Valid comma-separated <base_url>|<model>|<num_ctx> entries are parsed."""
    from news_creator.config.distributed_be_config import (
        RemoteTarget,
        parse_distributed_be_targets,
    )

    raw = (
        " http://router.example:8080/ | provider-a/model-a:latest | 32768 , "
        "https://router-b.example:8443///|provider-b/model-b:q4_k_m|16384 "
    )

    targets = parse_distributed_be_targets(raw)

    assert targets == [
        RemoteTarget(
            base_url="http://router.example:8080",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        ),
        RemoteTarget(
            base_url="https://router-b.example:8443",
            model="provider-b/model-b:q4_k_m",
            num_ctx=16384,
        ),
    ]


def test_parse_distributed_be_targets_allows_same_base_url_with_distinct_models():
    """Multiple targets may share a base_url when their model names differ."""
    from news_creator.config.distributed_be_config import (
        RemoteTarget,
        parse_distributed_be_targets,
    )

    raw = (
        "http://router.example:8080|provider-a/model-a:latest|32768,"
        "http://router.example:8080|provider-b/model-b:latest|16384"
    )

    targets = parse_distributed_be_targets(raw)

    assert targets == [
        RemoteTarget(
            base_url="http://router.example:8080",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        ),
        RemoteTarget(
            base_url="http://router.example:8080",
            model="provider-b/model-b:latest",
            num_ctx=16384,
        ),
    ]


def test_parse_distributed_be_targets_rejects_duplicate_base_url_and_model():
    """Duplicate (base_url, model) pairs raise ValueError naming the entry index."""
    from news_creator.config.distributed_be_config import parse_distributed_be_targets

    raw = (
        "http://router.example:8080/|provider-a/model-a:latest|32768,"
        "http://router.example:8080|provider-a/model-a:latest|16384"
    )

    with pytest.raises(ValueError) as exc_info:
        parse_distributed_be_targets(raw)

    msg = str(exc_info.value).lower()
    assert "1" in msg
    assert "duplicate" in msg


@pytest.mark.parametrize(
    "raw,expected_index,expected_problem_fragment",
    [
        ("http://router.example:8080|provider-a/model-a:latest", "0", ""),
        (
            "http://router.example:8080|provider-a/model-a:latest|32768|extra",
            "0",
            "",
        ),
        ("router.example:8080|provider-a/model-a:latest|32768", "0", "base_url"),
        ("ftp://router.example:8080|provider-a/model-a:latest|32768", "0", "base_url"),
        ("http:///no-host|provider-a/model-a:latest|32768", "0", "base_url"),
        ("http://router.example:8080|   |32768", "0", "model"),
        ("http://router.example:8080|provider-a/model-a:latest|abc", "0", "num_ctx"),
        ("http://router.example:8080|provider-a/model-a:latest|512", "0", "1024"),
        (
            "http://router.example:8080|provider-a/model-a:latest|2048,"
            "http://router.example:8080|provider-b/model-b:latest|1023",
            "1",
            "1024",
        ),
        ("http://u:p@gw.example:8080|m|8192", "0", ""),
        ("http://user@gw.example:8080|m|8192", "0", ""),
        ("http://gw.example:8080/v1|m|8192", "0", ""),
        ("http://gw.example:8080/api/v1/|m|8192", "0", ""),
        ("http://gw.example:8080?foo=bar|m|8192", "0", ""),
        ("http://gw.example:8080#frag|m|8192", "0", ""),
        ("http://gw.example:8080/;param=1|m|8192", "0", ""),
        ("http://gw.example :8080|m|8192", "0", ""),
        ("http://gw.example:8080\x00|m|8192", "0", ""),
        ("http://gw.example:8080\x1f|m|8192", "0", ""),
        ("http://gw\x9b.example:8080|m|8192", "0", ""),
        ("http://gw\u200b.example:8080|m|8192", "0", ""),
        (
            "http://gw.example:8080|m-a|8192,http://u:p@gw.example:8080|m-b|8192",
            "1",
            "",
        ),
    ],
)
def test_parse_distributed_be_targets_invalid_entry_names_index_and_problem(
    raw, expected_index, expected_problem_fragment
):
    """Invalid entries raise ValueError naming the entry index and problem."""
    from news_creator.config.distributed_be_config import parse_distributed_be_targets

    with pytest.raises(ValueError) as exc_info:
        parse_distributed_be_targets(raw)

    msg = str(exc_info.value)
    assert expected_index in msg
    if expected_problem_fragment:
        assert expected_problem_fragment in msg


def test_distributed_be_defaults_off_and_removes_legacy_fields(monkeypatch):
    """DISTRIBUTED_BE_ENABLED defaults to false; legacy remote fields are removed."""
    monkeypatch.delenv("DISTRIBUTED_BE_ENABLED", raising=False)
    monkeypatch.delenv("DISTRIBUTED_BE_TARGETS", raising=False)
    monkeypatch.delenv("DISTRIBUTED_BE_REASONING_EFFORT", raising=False)

    config = NewsCreatorConfig()

    assert config.distributed_be_enabled is False
    assert config.distributed_be_targets == []
    assert config.distributed_be_reasoning_effort is None
    assert config.distributed_be_health_interval_seconds == 30
    assert config.distributed_be_timeout_seconds == 300
    assert config.distributed_be_cooldown_seconds == 60
    assert not hasattr(config, "distributed_be_remotes")
    assert not hasattr(config, "distributed_be_remote_model")
    assert not hasattr(config, "distributed_be_model_overrides")


def test_distributed_be_disabled_does_not_require_targets(monkeypatch):
    """When DISTRIBUTED_BE_ENABLED=false, empty targets do not fail config init."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "false")
    monkeypatch.setenv("DISTRIBUTED_BE_TARGETS", "")

    config = NewsCreatorConfig()

    assert config.distributed_be_enabled is False
    assert config.distributed_be_targets == []


def test_distributed_be_enabled_with_empty_targets_raises_value_error(monkeypatch):
    """When DISTRIBUTED_BE_ENABLED=true, empty DISTRIBUTED_BE_TARGETS fails fast."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv("DISTRIBUTED_BE_TARGETS", "   ")

    with pytest.raises(ValueError, match="DISTRIBUTED_BE_TARGETS"):
        NewsCreatorConfig()


def test_distributed_be_enabled_with_invalid_target_raises_value_error(monkeypatch):
    """When DISTRIBUTED_BE_ENABLED=true, an invalid target fails fast at init."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_TARGETS",
        "http://router.example:8080|provider-a/model-a:latest|512",
    )

    with pytest.raises(ValueError) as exc_info:
        NewsCreatorConfig()

    assert "0" in str(exc_info.value)


def test_distributed_be_enabled_parses_targets_and_custom_intervals(monkeypatch):
    """NewsCreatorConfig parses DISTRIBUTED_BE_TARGETS and keeps timing settings."""
    from news_creator.config.distributed_be_config import RemoteTarget

    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_TARGETS",
        "http://router.example:8080/|provider-a/model-a:latest|32768",
    )
    monkeypatch.setenv("DISTRIBUTED_BE_HEALTH_INTERVAL_SECONDS", "15")
    monkeypatch.setenv("DISTRIBUTED_BE_TIMEOUT_SECONDS", "120")
    monkeypatch.setenv("DISTRIBUTED_BE_COOLDOWN_SECONDS", "30")

    config = NewsCreatorConfig()

    assert config.distributed_be_enabled is True
    assert config.distributed_be_targets == [
        RemoteTarget(
            base_url="http://router.example:8080",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        )
    ]
    assert config.distributed_be_health_interval_seconds == 15
    assert config.distributed_be_timeout_seconds == 120
    assert config.distributed_be_cooldown_seconds == 30


def test_distributed_be_reasoning_effort_unset_or_empty_is_none(monkeypatch):
    """Empty or unset DISTRIBUTED_BE_REASONING_EFFORT resolves to None."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_TARGETS",
        "http://router.example:8080|provider-a/model-a:latest|32768",
    )
    monkeypatch.setenv("DISTRIBUTED_BE_REASONING_EFFORT", "   ")

    config = NewsCreatorConfig()

    assert config.distributed_be_reasoning_effort is None


@pytest.mark.parametrize("effort", ["none", "minimal", "low", "medium", "high"])
def test_distributed_be_reasoning_effort_valid_values(monkeypatch, effort: str):
    """Valid DISTRIBUTED_BE_REASONING_EFFORT values are stored verbatim."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_TARGETS",
        "http://router.example:8080|provider-a/model-a:latest|32768",
    )
    monkeypatch.setenv("DISTRIBUTED_BE_REASONING_EFFORT", effort)

    config = NewsCreatorConfig()

    assert config.distributed_be_reasoning_effort == effort


def test_distributed_be_reasoning_effort_invalid_raises_value_error_when_enabled(
    monkeypatch,
):
    """Invalid DISTRIBUTED_BE_REASONING_EFFORT raises ValueError when DISTRIBUTED_BE_ENABLED=true."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_TARGETS",
        "http://router.example:8080|provider-a/model-a:latest|32768",
    )
    monkeypatch.setenv("DISTRIBUTED_BE_REASONING_EFFORT", "turbo")

    with pytest.raises(ValueError, match="DISTRIBUTED_BE_REASONING_EFFORT"):
        NewsCreatorConfig()


def test_distributed_be_disabled_ignores_malformed_targets_and_reasoning_effort(
    monkeypatch,
):
    """When DISTRIBUTED_BE_ENABLED=false, neither DISTRIBUTED_BE_TARGETS nor DISTRIBUTED_BE_REASONING_EFFORT is parsed even if malformed."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "false")
    monkeypatch.setenv("DISTRIBUTED_BE_TARGETS", "malformed-not-a-valid-target")
    monkeypatch.setenv("DISTRIBUTED_BE_REASONING_EFFORT", "turbo-invalid")

    config = NewsCreatorConfig()

    assert config.distributed_be_enabled is False
    assert config.distributed_be_targets == []
    assert config.distributed_be_reasoning_effort is None


def test_distributed_be_disabled_ignores_populated_targets_and_reasoning_effort(
    monkeypatch,
):
    """When DISTRIBUTED_BE_ENABLED=false, populated targets and effort resolve to [] and None."""
    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "false")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_TARGETS",
        "http://router.example:8080|provider-a/model-a:latest|32768",
    )
    monkeypatch.setenv("DISTRIBUTED_BE_REASONING_EFFORT", "high")

    config = NewsCreatorConfig()

    assert config.distributed_be_enabled is False
    assert config.distributed_be_targets == []
    assert config.distributed_be_reasoning_effort is None


def test_parse_distributed_be_reasoning_effort_has_no_strict_parameter():
    """L5: parse_distributed_be_reasoning_effort takes only raw (no strict parameter)."""
    import inspect

    from news_creator.config.distributed_be_config import (
        parse_distributed_be_reasoning_effort,
    )

    sig = inspect.signature(parse_distributed_be_reasoning_effort)
    assert "strict" not in sig.parameters
    assert parse_distributed_be_reasoning_effort(None) is None
    assert parse_distributed_be_reasoning_effort("   ") is None
    assert parse_distributed_be_reasoning_effort("low") == "low"
    with pytest.raises(ValueError, match="DISTRIBUTED_BE_REASONING_EFFORT"):
        parse_distributed_be_reasoning_effort("turbo")
    with pytest.raises(TypeError):
        parse_distributed_be_reasoning_effort("turbo", strict=False)  # type: ignore[call-arg]


def test_distributed_be_unhealthy_threshold_default_override_and_invalid(monkeypatch):
    """(g) DISTRIBUTED_BE_UNHEALTHY_THRESHOLD defaults to 2, accepts positive int override, and rejects < 1."""
    monkeypatch.delenv("DISTRIBUTED_BE_UNHEALTHY_THRESHOLD", raising=False)
    default_config = NewsCreatorConfig()
    assert default_config.distributed_be_unhealthy_threshold == 2

    monkeypatch.setenv("DISTRIBUTED_BE_UNHEALTHY_THRESHOLD", "4")
    override_config = NewsCreatorConfig()
    assert override_config.distributed_be_unhealthy_threshold == 4

    monkeypatch.setenv("DISTRIBUTED_BE_UNHEALTHY_THRESHOLD", "0")
    with pytest.raises(ValueError, match="DISTRIBUTED_BE_UNHEALTHY_THRESHOLD"):
        NewsCreatorConfig()

    monkeypatch.setenv("DISTRIBUTED_BE_UNHEALTHY_THRESHOLD", "-2")
    with pytest.raises(ValueError, match="DISTRIBUTED_BE_UNHEALTHY_THRESHOLD"):
        NewsCreatorConfig()
