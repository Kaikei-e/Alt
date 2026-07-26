"""Tests for configuration module."""

from news_creator.config.config import NewsCreatorConfig


def test_config_loads_defaults(monkeypatch):
    """Test that config loads with default values."""
    monkeypatch.delenv("LLM_SERVICE_URL", raising=False)
    monkeypatch.delenv("LLM_MODEL", raising=False)

    config = NewsCreatorConfig()

    assert config.llm_service_url == "http://localhost:11435"
    assert config.model_name == "gemma4-e4b-q4km"


def test_config_loads_from_environment(monkeypatch):
    """Test that config loads values from environment variables."""
    monkeypatch.setenv("LLM_SERVICE_URL", "http://custom-llm:8080")
    monkeypatch.setenv("LLM_MODEL", "custom-model:7b")
    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "120")
    monkeypatch.setenv("LLM_TEMPERATURE", "0.7")

    config = NewsCreatorConfig()

    assert config.llm_service_url == "http://custom-llm:8080"
    assert config.model_name == "custom-model:7b"
    assert config.llm_timeout_seconds == 120
    assert config.llm_temperature == 0.7


def test_config_handles_invalid_numeric_values(monkeypatch):
    """Test that config handles invalid numeric values gracefully."""
    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "invalid")
    monkeypatch.setenv("LLM_TEMPERATURE", "not_a_float")

    config = NewsCreatorConfig()

    # Should fall back to defaults
    assert config.llm_timeout_seconds == 300
    assert config.llm_temperature == 0.7  # Gemma4 default


def test_config_auth_settings(monkeypatch):
    """Authentication is now established at the TLS transport layer; the
    config retains only the auth service URL for forward-compatible refs."""
    monkeypatch.setenv("AUTH_SERVICE_URL", "http://auth:8080")

    config = NewsCreatorConfig()

    assert config.auth_service_url == "http://auth:8080"
    assert config.service_name == "news-creator"


def test_config_llm_options(monkeypatch):
    """Test LLM options configuration."""
    monkeypatch.setenv("LLM_NUM_PREDICT", "1000")
    monkeypatch.setenv("LLM_TOP_P", "0.95")
    monkeypatch.setenv("LLM_REPEAT_PENALTY", "1.1")
    monkeypatch.setenv("LLM_NUM_CTX", "4096")
    monkeypatch.setenv("LLM_STOP_TOKENS", "<end>,<stop>")

    config = NewsCreatorConfig()

    assert config.llm_num_predict == 1000
    assert config.llm_top_p == 0.95
    assert config.llm_repeat_penalty == 1.1
    assert config.llm_num_ctx == 4096
    assert config.llm_stop_tokens == ["<end>", "<stop>"]


def test_config_summary_num_predict(monkeypatch):
    """Test summary-specific num_predict configuration."""
    monkeypatch.setenv("SUMMARY_NUM_PREDICT", "750")

    config = NewsCreatorConfig()

    assert config.summary_num_predict == 750


def test_config_recap_quality_defaults():
    """Recap quality gates should load sane defaults."""

    config = NewsCreatorConfig()

    assert config.recap_min_source_articles_for_llm == 1
    assert config.recap_min_representative_sentences_for_llm == 2
    assert config.recap_ja_ratio_threshold == 0.6
    assert config.recap_summary_repair_attempts == 2


def test_recap_summary_num_predict_default(monkeypatch):
    """recap_summary_num_predict should default to 4000 (separate from summary_num_predict)."""
    monkeypatch.delenv("RECAP_SUMMARY_NUM_PREDICT", raising=False)

    config = NewsCreatorConfig()

    assert config.recap_summary_num_predict == 4000
    assert config.recap_min_avg_bullet_length == 300
    # Must be independent from summary_num_predict (which defaults to 1000)
    assert config.summary_num_predict == 1000


def test_recap_summary_num_predict_env_override(monkeypatch):
    """RECAP_SUMMARY_NUM_PREDICT env should override the default."""
    monkeypatch.setenv("RECAP_SUMMARY_NUM_PREDICT", "3000")

    config = NewsCreatorConfig()

    assert config.recap_summary_num_predict == 3000


def test_concurrency_defaults_to_one_when_envs_missing(monkeypatch):
    """When concurrency envs are missing, default to 1 via OLLAMA_NUM_PARALLEL."""
    # Ensure no concurrency envs are set
    monkeypatch.delenv("OLLAMA_REQUEST_CONCURRENCY", raising=False)
    monkeypatch.delenv("OLLAMA_NUM_PARALLEL", raising=False)

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 1
    # When both envs are missing, we fall back via OLLAMA_NUM_PARALLEL default=1
    assert getattr(config, "_ollama_concurrency_source") == "OLLAMA_NUM_PARALLEL"


def test_concurrency_uses_ollama_num_parallel_when_set(monkeypatch):
    """When only OLLAMA_NUM_PARALLEL is set, use it for request concurrency."""
    monkeypatch.delenv("OLLAMA_REQUEST_CONCURRENCY", raising=False)
    monkeypatch.setenv("OLLAMA_NUM_PARALLEL", "2")

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 2
    assert getattr(config, "_ollama_concurrency_source") == "OLLAMA_NUM_PARALLEL"


def test_concurrency_prefers_request_concurrency_over_num_parallel(monkeypatch):
    """OLLAMA_REQUEST_CONCURRENCY should override OLLAMA_NUM_PARALLEL when both are set."""
    monkeypatch.setenv("OLLAMA_REQUEST_CONCURRENCY", "1")
    monkeypatch.setenv("OLLAMA_NUM_PARALLEL", "2")

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 1
    assert getattr(config, "_ollama_concurrency_source") == "OLLAMA_REQUEST_CONCURRENCY"


# ============================================================================
# 12K-only Mode Configuration Tests
# ============================================================================


def test_model_60k_enabled_defaults_to_false(monkeypatch):
    """Test that model_60k_enabled defaults to False for 12K-only operation."""
    monkeypatch.delenv("MODEL_60K_ENABLED", raising=False)

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is False


def test_model_60k_enabled_can_be_set_true(monkeypatch):
    """Test that model_60k_enabled can be enabled via environment variable."""
    monkeypatch.setenv("MODEL_60K_ENABLED", "true")

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is True


def test_model_60k_enabled_case_insensitive(monkeypatch):
    """Test that MODEL_60K_ENABLED is case-insensitive."""
    monkeypatch.setenv("MODEL_60K_ENABLED", "TRUE")

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is True


def test_hierarchical_threshold_chars_default_8000(monkeypatch):
    """Test that hierarchical_threshold_chars defaults to 8000 for primary-bucket mode."""
    monkeypatch.delenv("HIERARCHICAL_THRESHOLD_CHARS", raising=False)

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_chars == 8_000


def test_hierarchical_threshold_clusters_default_5(monkeypatch):
    """Test that hierarchical_threshold_clusters defaults to 5 for primary-bucket mode."""
    monkeypatch.delenv("HIERARCHICAL_THRESHOLD_CLUSTERS", raising=False)

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_clusters == 5


def test_hierarchical_chunk_max_chars_default_6000(monkeypatch):
    """Test that hierarchical_chunk_max_chars defaults to 6000 (~1.5K tokens) for the primary bucket."""
    monkeypatch.delenv("HIERARCHICAL_CHUNK_MAX_CHARS", raising=False)

    config = NewsCreatorConfig()

    assert config.hierarchical_chunk_max_chars == 6_000


def test_hierarchical_thresholds_can_be_customized(monkeypatch):
    """Test that all hierarchical thresholds can be customized via environment."""
    monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CHARS", "20000")
    monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CLUSTERS", "10")
    monkeypatch.setenv("HIERARCHICAL_CHUNK_MAX_CHARS", "10000")

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_chars == 20_000
    assert config.hierarchical_threshold_clusters == 10
    assert config.hierarchical_chunk_max_chars == 10_000


# ============================================================================
# Preemption Configuration Tests
# ============================================================================


def test_preemption_enabled_defaults_to_true(monkeypatch):
    """Test that preemption is enabled by default."""
    monkeypatch.delenv("SCHEDULING_PREEMPTION_ENABLED", raising=False)

    config = NewsCreatorConfig()

    assert config.scheduling_preemption_enabled is True


def test_preemption_can_be_disabled(monkeypatch):
    """Test that preemption can be disabled via environment variable."""
    monkeypatch.setenv("SCHEDULING_PREEMPTION_ENABLED", "false")

    config = NewsCreatorConfig()

    assert config.scheduling_preemption_enabled is False


def test_preemption_wait_threshold_defaults_to_2_seconds(monkeypatch):
    """Test that preemption wait threshold defaults to 2.0 seconds."""
    monkeypatch.delenv("SCHEDULING_PREEMPTION_WAIT_THRESHOLD_SECONDS", raising=False)

    config = NewsCreatorConfig()

    assert config.scheduling_preemption_wait_threshold_seconds == 2.0


def test_preemption_wait_threshold_can_be_customized(monkeypatch):
    """Test that preemption wait threshold can be customized."""
    monkeypatch.setenv("SCHEDULING_PREEMPTION_WAIT_THRESHOLD_SECONDS", "10.0")

    config = NewsCreatorConfig()

    assert config.scheduling_preemption_wait_threshold_seconds == 10.0
