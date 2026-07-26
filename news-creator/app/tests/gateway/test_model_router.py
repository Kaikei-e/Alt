"""Tests for model router - selecting appropriate model bucket based on token count."""

from unittest.mock import Mock

from news_creator.gateway.model_router import ModelRouter


def _create_mock_config(model_60k_enabled: bool = False):
    """Create a mock config for testing."""
    config = Mock()
    config.model_routing_enabled = True
    config.model_name = "gemma4-e4b-q4km"
    config.llm_num_ctx = 8192
    config.llm_num_predict = 1200
    config.model_8k_name = "gemma4-e4b-12k"
    config.model_60k_name = "gemma4-e4b-60k"
    config.token_safety_margin_percent = 10
    config.token_safety_margin_fixed = 512
    config.oom_detection_enabled = False
    config.model_60k_enabled = model_60k_enabled
    return config


class TestModelRouter60KDisabled:
    """Tests for model router when 60K model is disabled (single primary-bucket mode)."""

    def test_small_prompt_uses_primary_bucket_when_60k_disabled(self):
        """Small prompts should use the primary bucket model when 60K is disabled."""
        config = _create_mock_config(model_60k_enabled=False)
        router = ModelRouter(config)

        # Small prompt that fits in the primary bucket
        prompt = "A" * 1000  # ~250 tokens
        model_name, bucket_size = router.select_model(prompt)

        assert model_name == "gemma4-e4b-12k"
        assert bucket_size == 8192

    def test_large_prompt_still_uses_primary_bucket_when_60k_disabled(self):
        """Large prompts should still use the primary bucket model when 60K is disabled."""
        config = _create_mock_config(model_60k_enabled=False)
        router = ModelRouter(config)

        # Large prompt that would normally require 60K (~15K tokens)
        prompt = "A" * 60000
        model_name, bucket_size = router.select_model(prompt)

        # Should still use the primary bucket even though it's too large
        assert model_name == "gemma4-e4b-12k"
        assert bucket_size == 8192

    def test_60k_enabled_uses_60k_for_large_prompts(self):
        """When 60K is enabled, large prompts should use 60K model."""
        config = _create_mock_config(model_60k_enabled=True)
        router = ModelRouter(config)

        # Large prompt that requires 60K
        prompt = "A" * 60000  # ~15K tokens
        model_name, bucket_size = router.select_model(prompt)

        assert model_name == "gemma4-e4b-60k"
        assert bucket_size == 61440

    def test_60k_enabled_uses_primary_bucket_for_small_prompts(self):
        """When 60K is enabled, small prompts should still use the primary bucket model."""
        config = _create_mock_config(model_60k_enabled=True)
        router = ModelRouter(config)

        # Small prompt that fits in the primary bucket
        prompt = "A" * 1000
        model_name, bucket_size = router.select_model(prompt)

        assert model_name == "gemma4-e4b-12k"
        assert bucket_size == 8192

    def test_extremely_large_prompt_uses_primary_bucket_when_60k_disabled(self):
        """Extremely large prompts should use the primary bucket model when 60K is disabled with warning logged."""
        config = _create_mock_config(model_60k_enabled=False)
        router = ModelRouter(config)

        # Extremely large prompt that exceeds even 60K
        prompt = "A" * 250000  # ~62.5K tokens
        model_name, bucket_size = router.select_model(prompt)

        # Should still use the primary bucket (hierarchical summarization should handle this upstream)
        assert model_name == "gemma4-e4b-12k"
        assert bucket_size == 8192


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


class TestModelRouterBucketHysteresis:
    """Tests for the cross-call "sticky bucket" (2x rule) behavior.

    ModelRouter keeps `_current_bucket` on the instance across calls to avoid
    thrashing Ollama model reloads on an 8GB-VRAM box: a downgrade to a
    smaller bucket is only allowed once the smaller bucket is <= half of the
    current one, while an upgrade to a larger bucket is always allowed
    immediately. Every other test in this module builds a fresh ModelRouter
    per call, so this stateful path was previously never exercised.
    """

    def test_upgrade_to_larger_bucket_happens_immediately(self):
        """A router already on the 8K bucket must upgrade to 60K right away."""
        config = _create_mock_config(model_60k_enabled=True)
        router = ModelRouter(config)

        # First call: small prompt -> 8K bucket, and this becomes "current".
        model_name, bucket_size = router.select_model("A" * 1000)
        assert (model_name, bucket_size) == ("gemma4-e4b-12k", 8192)
        assert router.current_bucket == 8192

        # Second call: large prompt -> upgrade to 60K, no 2x rule gating.
        model_name, bucket_size = router.select_model("A" * 60000)
        assert (model_name, bucket_size) == ("gemma4-e4b-60k", 61440)
        assert router.current_bucket == 61440

    def test_downgrade_to_smaller_bucket_switches_when_2x_rule_satisfied(self):
        """A router on 60K must drop back to 8K once demand is small enough.

        60K (61440) is more than 2x the 8K bucket (8192), so a subsequent
        small prompt satisfies the 2x rule and the router switches down
        instead of staying pinned to the larger bucket forever.
        """
        config = _create_mock_config(model_60k_enabled=True)
        router = ModelRouter(config)

        # First call: large prompt -> 60K bucket becomes "current".
        model_name, bucket_size = router.select_model("A" * 60000)
        assert (model_name, bucket_size) == ("gemma4-e4b-60k", 61440)
        assert router.current_bucket == 61440

        # Second call: small prompt -> 2x rule satisfied, downgrades to 8K.
        model_name, bucket_size = router.select_model("A" * 1000)
        assert (model_name, bucket_size) == ("gemma4-e4b-12k", 8192)
        assert router.current_bucket == 8192

    def test_repeated_selection_in_same_bucket_keeps_current_bucket(self):
        """Two consecutive calls needing the same bucket must not thrash."""
        config = _create_mock_config(model_60k_enabled=True)
        router = ModelRouter(config)

        first = router.select_model("A" * 60000)
        second = router.select_model("A" * 60000)

        assert first == second == ("gemma4-e4b-60k", 61440)
        assert router.current_bucket == 61440
