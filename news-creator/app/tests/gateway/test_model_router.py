"""Tests for model router - selecting appropriate model bucket based on token count."""

import pytest
from unittest.mock import Mock

from news_creator.gateway.model_router import ModelRouter


def _create_mock_config(model_60k_enabled: bool = False):
    """Create a mock config for testing."""
    config = Mock()
    config.model_routing_enabled = True
    config.model_name = "gemma3:4b-it-qat"
    config.llm_num_ctx = 12288
    config.llm_num_predict = 1200
    config.model_12k_name = "gemma3-4b-12k"
    config.model_60k_name = "gemma3-4b-60k"
    config.token_safety_margin_percent = 10
    config.token_safety_margin_fixed = 512
    config.oom_detection_enabled = False
    config.model_60k_enabled = model_60k_enabled
    return config


class TestModelRouter60KDisabled:
    """Tests for model router when 60K model is disabled (12K-only mode)."""

    def test_small_prompt_uses_12k_when_60k_disabled(self):
        """Small prompts should use 12K model when 60K is disabled."""
        config = _create_mock_config(model_60k_enabled=False)
        router = ModelRouter(config)

        # Small prompt that would fit in 12K
        prompt = "A" * 1000  # ~250 tokens
        model_name, bucket_size = router.select_model(prompt)

        assert model_name == "gemma3-4b-12k"
        assert bucket_size == 12288

    def test_large_prompt_still_uses_12k_when_60k_disabled(self):
        """Large prompts should still use 12K model when 60K is disabled (hierarchical summarization handles this)."""
        config = _create_mock_config(model_60k_enabled=False)
        router = ModelRouter(config)

        # Large prompt that would normally require 60K (~15K tokens)
        prompt = "A" * 60000
        model_name, bucket_size = router.select_model(prompt)

        # Should still use 12K even though it's too large
        assert model_name == "gemma3-4b-12k"
        assert bucket_size == 12288

    def test_60k_enabled_uses_60k_for_large_prompts(self):
        """When 60K is enabled, large prompts should use 60K model."""
        config = _create_mock_config(model_60k_enabled=True)
        router = ModelRouter(config)

        # Large prompt that requires 60K
        prompt = "A" * 60000  # ~15K tokens
        model_name, bucket_size = router.select_model(prompt)

        assert model_name == "gemma3-4b-60k"
        assert bucket_size == 61440

    def test_60k_enabled_uses_12k_for_small_prompts(self):
        """When 60K is enabled, small prompts should still use 12K model."""
        config = _create_mock_config(model_60k_enabled=True)
        router = ModelRouter(config)

        # Small prompt that fits in 12K
        prompt = "A" * 1000
        model_name, bucket_size = router.select_model(prompt)

        assert model_name == "gemma3-4b-12k"
        assert bucket_size == 12288

    def test_extremely_large_prompt_uses_12k_when_60k_disabled(self):
        """Extremely large prompts should use 12K model when 60K is disabled with warning logged."""
        config = _create_mock_config(model_60k_enabled=False)
        router = ModelRouter(config)

        # Extremely large prompt that exceeds even 60K
        prompt = "A" * 250000  # ~62.5K tokens
        model_name, bucket_size = router.select_model(prompt)

        # Should still use 12K (hierarchical summarization should handle this upstream)
        assert model_name == "gemma3-4b-12k"
        assert bucket_size == 12288


class TestModelRouterRoutingDisabled:
    """Tests for model router when routing is disabled."""

    def test_routing_disabled_uses_default_model(self):
        """When routing is disabled, should use default model."""
        config = _create_mock_config(model_60k_enabled=False)
        config.model_routing_enabled = False
        router = ModelRouter(config)

        prompt = "A" * 60000
        model_name, bucket_size = router.select_model(prompt)

        assert model_name == config.model_name
        assert bucket_size == config.llm_num_ctx
