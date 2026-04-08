"""Tests for decomposed configuration classes (Phase 1 refactoring).

Following Python 3.14 best practices:
- Frozen dataclasses for immutable configuration
- Protocol-based structural typing
- Type aliases with the `type` statement
"""

from __future__ import annotations

import pytest


class TestLLMConfig:
    """Tests for LLMConfig dataclass."""

    def test_llm_config_is_frozen_dataclass(self):
        """LLMConfig should be an immutable frozen dataclass."""
        from news_creator.config.llm_config import LLMConfig

        config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="gemma4-e4b-q4km",
            timeout_seconds=300,
        )

        # Should be immutable
        with pytest.raises(AttributeError):
            config.service_url = "http://other:11435"

    def test_llm_config_has_required_fields(self):
        """LLMConfig should have all LLM-related fields."""
        from news_creator.config.llm_config import LLMConfig

        config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="gemma4-e4b-q4km",
            timeout_seconds=300,
            keep_alive="24h",
            keep_alive_8k="24h",
            keep_alive_60k="15m",
            num_ctx=8192,
            num_batch=1024,
            num_predict=1200,
            temperature=0.7,
            top_p=0.85,
            top_k=40,
            repeat_penalty=1.15,
            num_keep=-1,
            stop_tokens=["<turn|>"],
        )

        assert config.service_url == "http://localhost:11435"
        assert config.model_name == "gemma4-e4b-q4km"
        assert config.timeout_seconds == 300
        assert config.keep_alive == "24h"
        assert config.num_ctx == 8192
        assert config.temperature == 0.7
        assert config.stop_tokens == ["<turn|>"]

    def test_llm_config_get_options(self):
        """LLMConfig should provide get_options() method for Ollama."""
        from news_creator.config.llm_config import LLMConfig

        config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="gemma4-e4b-q4km",
            timeout_seconds=300,
            keep_alive="24h",
            keep_alive_8k="24h",
            keep_alive_60k="15m",
            num_ctx=8192,
            num_batch=1024,
            num_predict=1200,
            temperature=0.7,
            top_p=0.85,
            top_k=40,
            repeat_penalty=1.15,
            num_keep=-1,
            stop_tokens=["<turn|>"],
        )

        options = config.get_options()

        assert options["num_ctx"] == 8192
        assert options["num_predict"] == 1200
        assert options["temperature"] == 0.7
        assert options["stop"] == ["<turn|>"]

    def test_llm_config_from_env(self, monkeypatch):
        """LLMConfig.from_env() should load from environment variables."""
        from news_creator.config.llm_config import LLMConfig

        monkeypatch.setenv("LLM_SERVICE_URL", "http://custom:11435")
        monkeypatch.setenv("LLM_MODEL", "custom-model")
        monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "600")

        config = LLMConfig.from_env()

        assert config.service_url == "http://custom:11435"
        assert config.model_name == "custom-model"
        assert config.timeout_seconds == 600


class TestSchedulingConfig:
    """Tests for SchedulingConfig dataclass."""

    def test_scheduling_config_is_frozen_dataclass(self):
        """SchedulingConfig should be an immutable frozen dataclass."""
        from news_creator.config.scheduling_config import SchedulingConfig

        config = SchedulingConfig(
            rt_reserved_slots=1,
            aging_threshold_seconds=60.0,
            aging_boost=0.5,
        )

        with pytest.raises(AttributeError):
            config.rt_reserved_slots = 2

    def test_scheduling_config_has_required_fields(self):
        """SchedulingConfig should have all scheduling-related fields."""
        from news_creator.config.scheduling_config import SchedulingConfig

        config = SchedulingConfig(
            rt_reserved_slots=1,
            aging_threshold_seconds=60.0,
            aging_boost=0.5,
            preemption_enabled=True,
            preemption_wait_threshold_seconds=2.0,
            priority_promotion_threshold_seconds=120.0,
            guaranteed_be_ratio=5,
            max_queue_depth=10,
            rt_mode="fifo",
        )

        assert config.rt_reserved_slots == 1
        assert config.aging_threshold_seconds == 60.0
        assert config.preemption_enabled is True
        assert config.max_queue_depth == 10
        assert config.rt_mode == "fifo"

    def test_scheduling_config_from_env(self, monkeypatch):
        """SchedulingConfig.from_env() should load from environment variables."""
        from news_creator.config.scheduling_config import SchedulingConfig

        monkeypatch.setenv("SCHEDULING_RT_RESERVED_SLOTS", "2")
        monkeypatch.setenv("SCHEDULING_AGING_THRESHOLD_SECONDS", "30.0")
        monkeypatch.setenv("SCHEDULING_PREEMPTION_ENABLED", "false")

        config = SchedulingConfig.from_env()

        assert config.rt_reserved_slots == 2
        assert config.aging_threshold_seconds == 30.0
        assert config.preemption_enabled is False


class TestHierarchicalConfig:
    """Tests for HierarchicalConfig dataclass."""

    def test_hierarchical_config_is_frozen_dataclass(self):
        """HierarchicalConfig should be an immutable frozen dataclass."""
        from news_creator.config.hierarchical_config import HierarchicalConfig

        config = HierarchicalConfig(
            threshold_chars=8000,
            threshold_clusters=5,
            chunk_max_chars=6000,
        )

        with pytest.raises(AttributeError):
            config.threshold_chars = 10000

    def test_hierarchical_config_has_required_fields(self):
        """HierarchicalConfig should have all hierarchical summarization fields."""
        from news_creator.config.hierarchical_config import HierarchicalConfig

        config = HierarchicalConfig(
            threshold_chars=8000,
            threshold_clusters=5,
            chunk_max_chars=6000,
            chunk_overlap_ratio=0.15,
            recursive_reduce_max_chars=6000,
            recursive_reduce_max_depth=3,
            single_article_threshold=20000,
            single_article_chunk_size=6000,
            token_budget_percent=75,
        )

        assert config.threshold_chars == 8000
        assert config.threshold_clusters == 5
        assert config.chunk_overlap_ratio == 0.15
        assert config.recursive_reduce_max_depth == 3

    def test_hierarchical_config_from_env(self, monkeypatch):
        """HierarchicalConfig.from_env() should load from environment variables."""
        from news_creator.config.hierarchical_config import HierarchicalConfig

        monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CHARS", "10000")
        monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CLUSTERS", "10")

        config = HierarchicalConfig.from_env()

        assert config.threshold_chars == 10000
        assert config.threshold_clusters == 10


class TestModelRoutingConfig:
    """Tests for ModelRoutingConfig dataclass."""

    def test_model_routing_config_is_frozen_dataclass(self):
        """ModelRoutingConfig should be an immutable frozen dataclass."""
        from news_creator.config.model_routing_config import ModelRoutingConfig

        config = ModelRoutingConfig(
            enabled=True,
            base_name="gemma4-e4b-q4km",
            model_8k_name="gemma4-e4b-q4km",
            model_60k_name="gemma4-e4b-60k",
        )

        with pytest.raises(AttributeError):
            config.enabled = False

    def test_model_routing_config_has_required_fields(self):
        """ModelRoutingConfig should have all model routing fields."""
        from news_creator.config.model_routing_config import ModelRoutingConfig

        config = ModelRoutingConfig(
            enabled=True,
            base_name="gemma4-e4b-q4km",
            model_8k_name="gemma4-e4b-q4km",
            model_60k_name="gemma4-e4b-60k",
            model_60k_enabled=False,
            token_safety_margin_percent=10,
            token_safety_margin_fixed=512,
            oom_detection_enabled=True,
            warmup_enabled=True,
            warmup_keep_alive_minutes=30,
        )

        assert config.enabled is True
        assert config.base_name == "gemma4-e4b-q4km"
        assert config.model_60k_enabled is False

    def test_model_routing_config_is_base_model_name(self):
        """ModelRoutingConfig should provide is_base_model_name() method."""
        from news_creator.config.model_routing_config import ModelRoutingConfig

        config = ModelRoutingConfig(
            enabled=True,
            base_name="gemma4-e4b-q4km",
            model_8k_name="gemma4-e4b-q4km",
            model_60k_name="gemma4-e4b-60k",
        )

        assert config.is_base_model_name("gemma4-e4b-q4km") is True
        assert config.is_base_model_name("other-model") is False

    def test_model_routing_config_is_bucket_model_name(self):
        """ModelRoutingConfig should provide is_bucket_model_name() method."""
        from news_creator.config.model_routing_config import ModelRoutingConfig

        config = ModelRoutingConfig(
            enabled=True,
            base_name="gemma4-e4b-q4km",
            model_8k_name="gemma4-e4b-q4km",
            model_60k_name="gemma4-e4b-60k",
        )

        assert config.is_bucket_model_name("gemma4-e4b-q4km") is True
        assert config.is_bucket_model_name("gemma4-e4b-60k") is True
        assert config.is_bucket_model_name("other-model") is False

    def test_model_routing_config_from_env(self, monkeypatch):
        """ModelRoutingConfig.from_env() should load from environment variables."""
        from news_creator.config.model_routing_config import ModelRoutingConfig

        monkeypatch.setenv("MODEL_ROUTING_ENABLED", "false")
        monkeypatch.setenv("MODEL_BASE_NAME", "custom-base")

        config = ModelRoutingConfig.from_env()

        assert config.enabled is False
        assert config.base_name == "custom-base"


class TestNewsCreatorConfigComposition:
    """Tests for NewsCreatorConfig composition with new config classes."""

    def test_news_creator_config_composes_sub_configs(self, monkeypatch):
        """NewsCreatorConfig should compose LLMConfig, SchedulingConfig, etc."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")

        config = NewsCreatorConfig()

        # Should have composed sub-configs
        assert hasattr(config, "llm")
        assert hasattr(config, "scheduling")
        assert hasattr(config, "hierarchical")
        assert hasattr(config, "model_routing")

    def test_news_creator_config_backward_compatible_llm_fields(self, monkeypatch):
        """NewsCreatorConfig should maintain backward compatible LLM fields."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("LLM_SERVICE_URL", "http://test:11435")

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.llm_service_url == "http://test:11435"
        # New-style access should also work
        assert config.llm.service_url == "http://test:11435"

    def test_news_creator_config_backward_compatible_scheduling_fields(self, monkeypatch):
        """NewsCreatorConfig should maintain backward compatible scheduling fields."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("SCHEDULING_RT_RESERVED_SLOTS", "2")

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.scheduling_rt_reserved_slots == 2
        # New-style access should also work
        assert config.scheduling.rt_reserved_slots == 2

    def test_news_creator_config_backward_compatible_hierarchical_fields(self, monkeypatch):
        """NewsCreatorConfig should maintain backward compatible hierarchical fields."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CHARS", "10000")

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.hierarchical_threshold_chars == 10000
        # New-style access should also work
        assert config.hierarchical.threshold_chars == 10000

    def test_news_creator_config_backward_compatible_model_routing_fields(self, monkeypatch):
        """NewsCreatorConfig should maintain backward compatible model routing fields."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("MODEL_ROUTING_ENABLED", "false")

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.model_routing_enabled is False
        # New-style access should also work
        assert config.model_routing.enabled is False

    def test_news_creator_config_get_llm_options_still_works(self, monkeypatch):
        """NewsCreatorConfig.get_llm_options() should still work after refactoring."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("LLM_NUM_CTX", "4096")
        monkeypatch.setenv("LLM_TEMPERATURE", "0.5")

        config = NewsCreatorConfig()
        options = config.get_llm_options()

        assert options["num_ctx"] == 4096
        assert options["temperature"] == 0.5

    def test_news_creator_config_is_base_model_name_still_works(self, monkeypatch):
        """NewsCreatorConfig.is_base_model_name() should still work after refactoring."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("MODEL_BASE_NAME", "test-base")

        config = NewsCreatorConfig()

        assert config.is_base_model_name("test-base") is True
        assert config.is_base_model_name("other") is False

    def test_news_creator_config_is_bucket_model_name_still_works(self, monkeypatch):
        """NewsCreatorConfig.is_bucket_model_name() should still work after refactoring."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("MODEL_8K_NAME", "model-8k")
        monkeypatch.setenv("MODEL_60K_NAME", "model-60k")

        config = NewsCreatorConfig()

        assert config.is_bucket_model_name("model-8k") is True
        assert config.is_bucket_model_name("model-60k") is True
        assert config.is_bucket_model_name("other") is False

    def test_news_creator_config_get_keep_alive_for_model_still_works(self, monkeypatch):
        """NewsCreatorConfig.get_keep_alive_for_model() should still work after refactoring."""
        from news_creator.config.config import NewsCreatorConfig

        monkeypatch.setenv("SERVICE_SECRET", "test-secret")
        monkeypatch.setenv("MODEL_8K_NAME", "model-8k")
        monkeypatch.setenv("LLM_KEEP_ALIVE_8K", "48h")

        config = NewsCreatorConfig()

        assert config.get_keep_alive_for_model("model-8k") == "48h"
