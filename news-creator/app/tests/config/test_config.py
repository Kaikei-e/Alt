"""Tests for configuration module."""

import pytest

from news_creator.config.config import NewsCreatorConfig

pytestmark = pytest.mark.usefixtures("dummy_redis_password_file")


DEFAULT_MIN_SENTENCES = 2
DEFAULT_JA_RATIO_THRESHOLD = 0.6
DEFAULT_REPAIR_ATTEMPTS = 2
DEFAULT_RECAP_SUMMARY_NUM_PREDICT = 4000
DEFAULT_RECAP_MIN_AVG_BULLET_LENGTH = 300
DEFAULT_SUMMARY_NUM_PREDICT = 1000
DEFAULT_RECAP_CARD_MAX_NEW_TOKENS = 700
DEFAULT_HIERARCHICAL_THRESHOLD_CHARS = 8_000
DEFAULT_HIERARCHICAL_THRESHOLD_CLUSTERS = 5
DEFAULT_HIERARCHICAL_CHUNK_MAX_CHARS = 6_000
DEFAULT_PREEMPTION_WAIT_THRESHOLD_SECONDS = 2.0


def test_config_loads_defaults(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that config loads with default values."""
    monkeypatch.delenv("LLM_SERVICE_URL", raising=False)
    monkeypatch.delenv("LLM_MODEL", raising=False)

    config = NewsCreatorConfig()

    assert config.llm_service_url == "http://localhost:11435"
    assert config.model_name == "gemma4-e4b-q4km"


def test_config_loads_from_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that config loads values from environment variables."""
    expected_timeout_seconds = 120
    expected_temperature = 0.7
    monkeypatch.setenv("LLM_SERVICE_URL", "http://custom-llm:8080")
    monkeypatch.setenv("LLM_MODEL", "custom-model:7b")
    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", str(expected_timeout_seconds))
    monkeypatch.setenv("LLM_TEMPERATURE", str(expected_temperature))

    config = NewsCreatorConfig()

    assert config.llm_service_url == "http://custom-llm:8080"
    assert config.model_name == "custom-model:7b"
    assert config.llm_timeout_seconds == expected_timeout_seconds
    assert config.llm_temperature == expected_temperature


def test_config_handles_invalid_numeric_values(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that config fails fast on invalid numeric values."""
    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "invalid")
    with pytest.raises(ValueError, match="LLM_TIMEOUT_SECONDS"):
        NewsCreatorConfig()

    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "300")
    monkeypatch.setenv("LLM_TEMPERATURE", "not_a_float")
    with pytest.raises(ValueError, match="LLM_TEMPERATURE"):
        NewsCreatorConfig()


def test_config_auth_settings(monkeypatch: pytest.MonkeyPatch) -> None:
    """Authentication is now established at the TLS transport layer; the
    config retains only the auth service URL for forward-compatible refs."""
    monkeypatch.setenv("AUTH_SERVICE_URL", "http://auth:8080")

    config = NewsCreatorConfig()

    assert config.auth_service_url == "http://auth:8080"
    assert config.service_name == "news-creator"


def test_config_llm_options(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test LLM options configuration."""
    expected_num_predict = 1000
    expected_top_p = 0.95
    expected_repeat_penalty = 1.1
    expected_num_ctx = 4096
    monkeypatch.setenv("LLM_NUM_PREDICT", str(expected_num_predict))
    monkeypatch.setenv("LLM_TOP_P", str(expected_top_p))
    monkeypatch.setenv("LLM_REPEAT_PENALTY", str(expected_repeat_penalty))
    monkeypatch.setenv("LLM_NUM_CTX", str(expected_num_ctx))
    monkeypatch.setenv("LLM_STOP_TOKENS", "<end>,<stop>")

    config = NewsCreatorConfig()

    assert config.llm_num_predict == expected_num_predict
    assert config.llm_top_p == expected_top_p
    assert config.llm_repeat_penalty == expected_repeat_penalty
    assert config.llm_num_ctx == expected_num_ctx
    assert config.llm_stop_tokens == ["<end>", "<stop>"]


def test_config_summary_num_predict(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test summary-specific num_predict configuration."""
    expected_summary_num_predict = 750
    monkeypatch.setenv("SUMMARY_NUM_PREDICT", str(expected_summary_num_predict))

    config = NewsCreatorConfig()

    assert config.summary_num_predict == expected_summary_num_predict


def test_config_recap_quality_defaults() -> None:
    """Recap quality gates should load sane defaults."""

    config = NewsCreatorConfig()

    assert config.recap_min_source_articles_for_llm == 1
    assert config.recap_min_representative_sentences_for_llm == DEFAULT_MIN_SENTENCES
    assert config.recap_ja_ratio_threshold == DEFAULT_JA_RATIO_THRESHOLD
    assert config.recap_summary_repair_attempts == DEFAULT_REPAIR_ATTEMPTS


def test_recap_summary_num_predict_default(monkeypatch: pytest.MonkeyPatch) -> None:
    """recap_summary_num_predict should default to 4000 (separate from summary_num_predict)."""
    monkeypatch.delenv("RECAP_SUMMARY_NUM_PREDICT", raising=False)

    config = NewsCreatorConfig()

    assert config.recap_summary_num_predict == DEFAULT_RECAP_SUMMARY_NUM_PREDICT
    assert config.recap_min_avg_bullet_length == DEFAULT_RECAP_MIN_AVG_BULLET_LENGTH
    # Must be independent from summary_num_predict (which defaults to 1000)
    assert config.summary_num_predict == DEFAULT_SUMMARY_NUM_PREDICT


def test_recap_summary_num_predict_env_override(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """RECAP_SUMMARY_NUM_PREDICT env should override the default."""
    expected_num_predict = 3000
    monkeypatch.setenv("RECAP_SUMMARY_NUM_PREDICT", str(expected_num_predict))

    config = NewsCreatorConfig()

    assert config.recap_summary_num_predict == expected_num_predict


def test_recap_card_max_new_tokens_default(monkeypatch: pytest.MonkeyPatch) -> None:
    """recap_card_max_new_tokens should default to 700."""
    monkeypatch.delenv("RECAP_CARD_MAX_NEW_TOKENS", raising=False)

    config = NewsCreatorConfig()

    assert config.llm.recap_card_max_new_tokens == DEFAULT_RECAP_CARD_MAX_NEW_TOKENS


def test_recap_card_max_new_tokens_env_override(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """RECAP_CARD_MAX_NEW_TOKENS env should override the default."""
    expected_max_new_tokens = 850
    monkeypatch.setenv("RECAP_CARD_MAX_NEW_TOKENS", str(expected_max_new_tokens))

    config = NewsCreatorConfig()

    assert config.llm.recap_card_max_new_tokens == expected_max_new_tokens


def test_concurrency_defaults_to_one_when_envs_missing(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """When concurrency envs are missing, default to 1 via OLLAMA_NUM_PARALLEL."""
    # Ensure no concurrency envs are set
    monkeypatch.delenv("OLLAMA_REQUEST_CONCURRENCY", raising=False)
    monkeypatch.delenv("OLLAMA_NUM_PARALLEL", raising=False)

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 1
    # When both envs are missing, we fall back via OLLAMA_NUM_PARALLEL default=1
    assert config._ollama_concurrency_source == "OLLAMA_NUM_PARALLEL"


def test_concurrency_uses_ollama_num_parallel_when_set(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """When only OLLAMA_NUM_PARALLEL is set, use it for request concurrency."""
    expected_concurrency = 2
    monkeypatch.delenv("OLLAMA_REQUEST_CONCURRENCY", raising=False)
    monkeypatch.setenv("OLLAMA_NUM_PARALLEL", str(expected_concurrency))

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == expected_concurrency
    assert config._ollama_concurrency_source == "OLLAMA_NUM_PARALLEL"


def test_concurrency_prefers_request_concurrency_over_num_parallel(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """OLLAMA_REQUEST_CONCURRENCY should override OLLAMA_NUM_PARALLEL when both are set."""
    monkeypatch.setenv("OLLAMA_REQUEST_CONCURRENCY", "1")
    monkeypatch.setenv("OLLAMA_NUM_PARALLEL", "2")

    config = NewsCreatorConfig()

    assert config.ollama_request_concurrency == 1
    assert config._ollama_concurrency_source == "OLLAMA_REQUEST_CONCURRENCY"


# ============================================================================
# 12K-only Mode Configuration Tests
# ============================================================================


def test_model_60k_enabled_defaults_to_false(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that model_60k_enabled defaults to False for 12K-only operation."""
    monkeypatch.delenv("MODEL_60K_ENABLED", raising=False)

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is False


def test_model_60k_enabled_can_be_set_true(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that model_60k_enabled can be enabled via environment variable."""
    monkeypatch.setenv("MODEL_60K_ENABLED", "true")

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is True


def test_model_60k_enabled_case_insensitive(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that MODEL_60K_ENABLED is case-insensitive."""
    monkeypatch.setenv("MODEL_60K_ENABLED", "TRUE")

    config = NewsCreatorConfig()

    assert config.model_60k_enabled is True


def test_hierarchical_threshold_chars_default_8000(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Test that hierarchical_threshold_chars defaults to 8000 for primary-bucket mode."""
    monkeypatch.delenv("HIERARCHICAL_THRESHOLD_CHARS", raising=False)

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_chars == DEFAULT_HIERARCHICAL_THRESHOLD_CHARS


def test_hierarchical_threshold_clusters_default_5(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Test that hierarchical_threshold_clusters defaults to 5 for primary-bucket mode."""
    monkeypatch.delenv("HIERARCHICAL_THRESHOLD_CLUSTERS", raising=False)

    config = NewsCreatorConfig()

    assert (
        config.hierarchical_threshold_clusters
        == DEFAULT_HIERARCHICAL_THRESHOLD_CLUSTERS
    )


def test_hierarchical_chunk_max_chars_default_6000(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Test that hierarchical_chunk_max_chars defaults to 6000 (~1.5K tokens) for the primary bucket."""
    monkeypatch.delenv("HIERARCHICAL_CHUNK_MAX_CHARS", raising=False)

    config = NewsCreatorConfig()

    assert config.hierarchical_chunk_max_chars == DEFAULT_HIERARCHICAL_CHUNK_MAX_CHARS


def test_hierarchical_thresholds_can_be_customized(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Test that all hierarchical thresholds can be customized via environment."""
    expected_threshold_chars = 20_000
    expected_threshold_clusters = 10
    expected_chunk_max_chars = 10_000
    monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CHARS", str(expected_threshold_chars))
    monkeypatch.setenv(
        "HIERARCHICAL_THRESHOLD_CLUSTERS", str(expected_threshold_clusters)
    )
    monkeypatch.setenv("HIERARCHICAL_CHUNK_MAX_CHARS", str(expected_chunk_max_chars))

    config = NewsCreatorConfig()

    assert config.hierarchical_threshold_chars == expected_threshold_chars
    assert config.hierarchical_threshold_clusters == expected_threshold_clusters
    assert config.hierarchical_chunk_max_chars == expected_chunk_max_chars


# ============================================================================
# Preemption Configuration Tests
# ============================================================================


def test_preemption_enabled_defaults_to_true(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that preemption is enabled by default."""
    monkeypatch.delenv("SCHEDULING_PREEMPTION_ENABLED", raising=False)

    config = NewsCreatorConfig()

    assert config.scheduling_preemption_enabled is True


def test_preemption_can_be_disabled(monkeypatch: pytest.MonkeyPatch) -> None:
    """Test that preemption can be disabled via environment variable."""
    monkeypatch.setenv("SCHEDULING_PREEMPTION_ENABLED", "false")

    config = NewsCreatorConfig()

    assert config.scheduling_preemption_enabled is False


def test_preemption_wait_threshold_defaults_to_2_seconds(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Test that preemption wait threshold defaults to 2.0 seconds."""
    monkeypatch.delenv("SCHEDULING_PREEMPTION_WAIT_THRESHOLD_SECONDS", raising=False)

    config = NewsCreatorConfig()

    assert (
        config.scheduling_preemption_wait_threshold_seconds
        == DEFAULT_PREEMPTION_WAIT_THRESHOLD_SECONDS
    )


def test_preemption_wait_threshold_can_be_customized(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Test that preemption wait threshold can be customized."""
    expected_wait_threshold = 10.0
    monkeypatch.setenv(
        "SCHEDULING_PREEMPTION_WAIT_THRESHOLD_SECONDS", str(expected_wait_threshold)
    )

    config = NewsCreatorConfig()

    assert (
        config.scheduling_preemption_wait_threshold_seconds == expected_wait_threshold
    )


def test_llm_options_do_not_discard_the_end_of_the_prompt(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """num_keep must never ask Ollama to keep the whole prompt head.

    Ollama resolves a negative num_keep to len(tokens) (llm/llama_server.go),
    so on context overflow it keeps the head and discards the tail. The tail is
    where the user's question and the generation cue live, so the model is left
    answering a question it can no longer see, with the remaining budget spent:
    measured on a 22800-rune prompt, eval_count came back as 1 token with
    done_reason "length". Truncation has to eat the middle, never the question.
    """
    monkeypatch.delenv("LLM_NUM_KEEP", raising=False)

    config = NewsCreatorConfig()

    assert config.llm.num_keep >= 0
    assert config.llm.get_options()["num_keep"] >= 0
