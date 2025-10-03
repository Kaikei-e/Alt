"""Tests for configuration module."""

import os
import pytest
from news_creator.config.config import NewsCreatorConfig


def test_config_loads_defaults():
    """Test that config loads with default values."""
    os.environ.pop("LLM_SERVICE_URL", None)
    os.environ.pop("LLM_MODEL", None)

    config = NewsCreatorConfig()

    assert config.llm_service_url == "http://localhost:11435"
    assert config.model_name == "gemma3:4b"


def test_config_loads_from_environment():
    """Test that config loads values from environment variables."""
    os.environ["LLM_SERVICE_URL"] = "http://custom-llm:8080"
    os.environ["LLM_MODEL"] = "custom-model:7b"
    os.environ["LLM_TIMEOUT_SECONDS"] = "120"
    os.environ["LLM_TEMPERATURE"] = "0.7"

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


def test_config_handles_invalid_numeric_values():
    """Test that config handles invalid numeric values gracefully."""
    os.environ["LLM_TIMEOUT_SECONDS"] = "invalid"
    os.environ["LLM_TEMPERATURE"] = "not_a_float"

    config = NewsCreatorConfig()

    # Should fall back to defaults
    assert config.llm_timeout_seconds == 180
    assert config.llm_temperature == 0.2

    # Cleanup
    del os.environ["LLM_TIMEOUT_SECONDS"]
    del os.environ["LLM_TEMPERATURE"]


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
