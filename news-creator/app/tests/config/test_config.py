"""Tests for configuration module."""

import os
import pytest
from news_creator.config.config import NewsCreatorConfig


def test_config_loads_defaults():
    """Test that config loads with default values."""
    os.environ.pop("LLM_SERVICE_URL", None)
    os.environ.pop("LLM_MODEL", None)
    os.environ["SERVICE_SECRET"] = "test-secret"

    config = NewsCreatorConfig()

    assert config.llm_service_url == "http://localhost:11435"
    assert config.model_name == "gemma3:4b-it-qat"

    # Cleanup
    os.environ.pop("SERVICE_SECRET", None)


def test_config_loads_from_environment():
    """Test that config loads values from environment variables."""
    os.environ["LLM_SERVICE_URL"] = "http://custom-llm:8080"
    os.environ["LLM_MODEL"] = "custom-model:7b"
    os.environ["LLM_TIMEOUT_SECONDS"] = "120"
    os.environ["LLM_TEMPERATURE"] = "0.7"
    os.environ["SERVICE_SECRET"] = "test-secret"

    config = NewsCreatorConfig()

    assert config.llm_service_url == "http://custom-llm:8080"
    assert config.model_name == "custom-model:7b"
    assert config.llm_timeout_seconds == 120
    assert config.llm_temperature == 0.7

    # Cleanup
    del os.environ["LLM_SERVICE_URL"]
    del os.environ["LLM_MODEL"]
    del os.environ["LLM_TIMEOUT_SECONDS"]
    del os.environ["LLM_TEMPERATURE"]
    os.environ.pop("SERVICE_SECRET", None)


def test_config_handles_invalid_numeric_values():
    """Test that config handles invalid numeric values gracefully."""
    os.environ["LLM_TIMEOUT_SECONDS"] = "invalid"
    os.environ["LLM_TEMPERATURE"] = "not_a_float"
    os.environ["SERVICE_SECRET"] = "test-secret"

    config = NewsCreatorConfig()

    # Should fall back to defaults
    assert config.llm_timeout_seconds == 300
    assert config.llm_temperature == 0.7  # Gemma3 QAT optimized default

    # Cleanup
    del os.environ["LLM_TIMEOUT_SECONDS"]
    del os.environ["LLM_TEMPERATURE"]
    os.environ.pop("SERVICE_SECRET", None)


def test_config_auth_settings():
    """Test authentication configuration."""
    os.environ["AUTH_SERVICE_URL"] = "http://auth:8080"
    os.environ["SERVICE_SECRET"] = "test-secret"

    config = NewsCreatorConfig()

    assert config.auth_service_url == "http://auth:8080"
    assert config.service_secret == "test-secret"
    assert config.service_name == "news-creator"

    # Cleanup
    del os.environ["AUTH_SERVICE_URL"]
    del os.environ["SERVICE_SECRET"]


def test_config_raises_error_when_service_secret_missing():
    """Test that config raises error when SERVICE_SECRET is not set."""
    os.environ.pop("SERVICE_SECRET", None)

    with pytest.raises(ValueError, match="SERVICE_SECRET environment variable is required"):
        NewsCreatorConfig()


def test_config_llm_options():
    """Test LLM options configuration."""
    os.environ["SERVICE_SECRET"] = "test-secret"
    os.environ["LLM_NUM_PREDICT"] = "1000"
    os.environ["LLM_TOP_P"] = "0.95"
    os.environ["LLM_REPEAT_PENALTY"] = "1.1"
    os.environ["LLM_NUM_CTX"] = "4096"
    os.environ["LLM_STOP_TOKENS"] = "<end>,<stop>"

    config = NewsCreatorConfig()

    assert config.llm_num_predict == 1000
    assert config.llm_top_p == 0.95
    assert config.llm_repeat_penalty == 1.1
    assert config.llm_num_ctx == 4096
    assert config.llm_stop_tokens == ["<end>", "<stop>"]

    # Cleanup
    for key in ["SERVICE_SECRET", "LLM_NUM_PREDICT", "LLM_TOP_P",
                "LLM_REPEAT_PENALTY", "LLM_NUM_CTX", "LLM_STOP_TOKENS"]:
        os.environ.pop(key, None)


def test_config_summary_num_predict():
    """Test summary-specific num_predict configuration."""
    os.environ["SERVICE_SECRET"] = "test-secret"
    os.environ["SUMMARY_NUM_PREDICT"] = "750"

    config = NewsCreatorConfig()

    assert config.summary_num_predict == 750

    # Cleanup
    del os.environ["SERVICE_SECRET"]
    del os.environ["SUMMARY_NUM_PREDICT"]


def test_concurrency_defaults_to_one_when_envs_missing(monkeypatch):
    """When concurrency envs are missing, default to 1 via OLLAMA_NUM_PARALLEL."""
    # Ensure no concurrency envs are set
    monkeypatch.delenv("OLLAMA_REQUEST_CONCURRENCY", raising=False)
    monkeypatch.delenv("OLLAMA_NUM_PARALLEL", raising=False)
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 1
    # When both envs are missing, we fall back via OLLAMA_NUM_PARALLEL default=1
    assert getattr(config, "_ollama_concurrency_source") == "OLLAMA_NUM_PARALLEL"


def test_concurrency_uses_ollama_num_parallel_when_set(monkeypatch):
    """When only OLLAMA_NUM_PARALLEL is set, use it for request concurrency."""
    monkeypatch.delenv("OLLAMA_REQUEST_CONCURRENCY", raising=False)
    monkeypatch.setenv("OLLAMA_NUM_PARALLEL", "2")
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 2
    assert getattr(config, "_ollama_concurrency_source") == "OLLAMA_NUM_PARALLEL"


def test_concurrency_prefers_request_concurrency_over_num_parallel(monkeypatch):
    """OLLAMA_REQUEST_CONCURRENCY should override OLLAMA_NUM_PARALLEL when both are set."""
    monkeypatch.setenv("OLLAMA_REQUEST_CONCURRENCY", "1")
    monkeypatch.setenv("OLLAMA_NUM_PARALLEL", "2")
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 1
    assert getattr(config, "_ollama_concurrency_source") == "OLLAMA_REQUEST_CONCURRENCY"


# ============================================================================
# 12K-only Mode Configuration Tests
# ============================================================================


def test_model_60k_enabled_defaults_to_false(monkeypatch):
    """Test that model_60k_enabled defaults to False for 12K-only operation."""
    monkeypatch.delenv("MODEL_60K_ENABLED", raising=False)
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is False


def test_model_60k_enabled_can_be_set_true(monkeypatch):
    """Test that model_60k_enabled can be enabled via environment variable."""
    monkeypatch.setenv("MODEL_60K_ENABLED", "true")
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is True


def test_model_60k_enabled_case_insensitive(monkeypatch):
    """Test that MODEL_60K_ENABLED is case-insensitive."""
    monkeypatch.setenv("MODEL_60K_ENABLED", "TRUE")
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is True


def test_hierarchical_threshold_chars_default_8000(monkeypatch):
    """Test that hierarchical_threshold_chars defaults to 8000 for 8K-only mode."""
    monkeypatch.delenv("HIERARCHICAL_THRESHOLD_CHARS", raising=False)
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_chars == 8_000


def test_hierarchical_threshold_clusters_default_5(monkeypatch):
    """Test that hierarchical_threshold_clusters defaults to 5 for 8K-only mode."""
    monkeypatch.delenv("HIERARCHICAL_THRESHOLD_CLUSTERS", raising=False)
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_clusters == 5


def test_hierarchical_chunk_max_chars_default_6000(monkeypatch):
    """Test that hierarchical_chunk_max_chars defaults to 6000 (~1.5K tokens) for 8K context."""
    monkeypatch.delenv("HIERARCHICAL_CHUNK_MAX_CHARS", raising=False)
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.hierarchical_chunk_max_chars == 6_000


def test_hierarchical_thresholds_can_be_customized(monkeypatch):
    """Test that all hierarchical thresholds can be customized via environment."""
    monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CHARS", "20000")
    monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CLUSTERS", "10")
    monkeypatch.setenv("HIERARCHICAL_CHUNK_MAX_CHARS", "10000")
    monkeypatch.setenv("SERVICE_SECRET", "test-secret")

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_chars == 20_000
    assert config.hierarchical_threshold_clusters == 10
    assert config.hierarchical_chunk_max_chars == 10_000
