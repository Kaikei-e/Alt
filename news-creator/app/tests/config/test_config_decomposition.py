"""Tests for decomposed configuration classes (Phase 1 refactoring).

Following Python 3.14 best practices:
- Frozen dataclasses for immutable configuration
- Protocol-based structural typing
- Type aliases with the `type` statement
"""

from __future__ import annotations

import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.config.hierarchical_config import HierarchicalConfig
from news_creator.config.llm_config import LLMConfig
from news_creator.config.model_routing_config import ModelRoutingConfig
from news_creator.config.scheduling_config import SchedulingConfig

pytestmark = pytest.mark.usefixtures("dummy_redis_password_file")


class TestLLMConfig:
    """Tests for LLMConfig dataclass."""

    def test_llm_config_is_frozen_dataclass(self) -> None:
        """LLMConfig should be an immutable frozen dataclass."""
        config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="gemma4-e4b-q4km",
            timeout_seconds=300,
        )

        # Should be immutable
        with pytest.raises(
            AttributeError, match="cannot assign to field 'service_url'"
        ):
            config.service_url = "http://other:11435"

    def test_llm_config_has_required_fields(self) -> None:
        """LLMConfig should have all LLM-related fields."""
        timeout_seconds = 300
        num_ctx = 8192
        temperature = 0.7
        config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="gemma4-e4b-q4km",
            timeout_seconds=timeout_seconds,
            keep_alive="24h",
            keep_alive_8k="24h",
            keep_alive_60k="15m",
            num_ctx=num_ctx,
            num_batch=1024,
            num_predict=1200,
            temperature=temperature,
            top_p=0.85,
            top_k=40,
            repeat_penalty=1.15,
            num_keep=-1,
            stop_tokens=["<turn|>"],
        )

        assert config.service_url == "http://localhost:11435"
        assert config.model_name == "gemma4-e4b-q4km"
        assert config.timeout_seconds == timeout_seconds
        assert config.keep_alive == "24h"
        assert config.num_ctx == num_ctx
        assert config.temperature == temperature
        assert config.stop_tokens == ["<turn|>"]

    def test_llm_config_get_options(self) -> None:
        """LLMConfig should provide get_options() method for Ollama."""
        num_ctx = 8192
        num_predict = 1200
        temperature = 0.7
        config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="gemma4-e4b-q4km",
            timeout_seconds=300,
            keep_alive="24h",
            keep_alive_8k="24h",
            keep_alive_60k="15m",
            num_ctx=num_ctx,
            num_batch=1024,
            num_predict=num_predict,
            temperature=temperature,
            top_p=0.85,
            top_k=40,
            repeat_penalty=1.15,
            num_keep=-1,
            stop_tokens=["<turn|>"],
        )

        options = config.get_options()

        assert options["num_ctx"] == num_ctx
        assert options["num_predict"] == num_predict
        assert options["temperature"] == temperature
        assert options["stop"] == ["<turn|>"]

    def test_llm_config_get_options_pins_gpu_layers(self) -> None:
        """get_options() must carry num_gpu so Ollama forwards --n-gpu-layers.

        Ollama only passes -ngl to llama-server when the option is present in
        the request; a Modelfile PARAMETER alone does not reach the runner
        (measured on Ollama 0.32.1). Without the flag llama.cpp auto-fits the
        layer count to whatever VRAM happens to be free and degrades to a
        CPU/GPU split, and gemma4 aborts at load in that configuration.
        """
        default_num_gpu = 99
        config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="gemma4-e4b-q4km",
        )

        assert config.get_options()["num_gpu"] == default_num_gpu

    def test_llm_config_from_env_reads_num_gpu(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """LLM_NUM_GPU overrides the pinned layer count."""
        expected_num_gpu = 43
        monkeypatch.setenv("LLM_NUM_GPU", str(expected_num_gpu))

        assert LLMConfig.from_env().num_gpu == expected_num_gpu

    def test_llm_config_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """LLMConfig.from_env() should load from environment variables."""
        expected_timeout_seconds = 600
        monkeypatch.setenv("LLM_SERVICE_URL", "http://custom:11435")
        monkeypatch.setenv("LLM_MODEL", "custom-model")
        monkeypatch.setenv("LLM_TIMEOUT_SECONDS", str(expected_timeout_seconds))

        config = LLMConfig.from_env()

        assert config.service_url == "http://custom:11435"
        assert config.model_name == "custom-model"
        assert config.timeout_seconds == expected_timeout_seconds

    def test_llm_config_from_env_invalid_int_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """LLMConfig.from_env() must raise ValueError when an int env var is invalid."""
        monkeypatch.setenv("LLM_NUM_CTX", "12k")

        with pytest.raises(ValueError, match="LLM_NUM_CTX"):
            LLMConfig.from_env()

    def test_llm_config_from_env_invalid_float_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """LLMConfig.from_env() must raise ValueError when a float env var is invalid."""
        monkeypatch.setenv("LLM_TEMPERATURE", "hot")

        with pytest.raises(ValueError, match="LLM_TEMPERATURE"):
            LLMConfig.from_env()


class TestSchedulingConfig:
    """Tests for SchedulingConfig dataclass."""

    def test_scheduling_config_is_frozen_dataclass(self) -> None:
        """SchedulingConfig should be an immutable frozen dataclass."""
        config = SchedulingConfig(
            rt_reserved_slots=1,
            aging_threshold_seconds=60.0,
            aging_boost=0.5,
        )

        with pytest.raises(
            AttributeError, match="cannot assign to field 'rt_reserved_slots'"
        ):
            config.rt_reserved_slots = 2

    def test_scheduling_config_has_required_fields(self) -> None:
        """SchedulingConfig should have all scheduling-related fields."""
        aging_threshold_seconds = 60.0
        max_queue_depth = 10
        config = SchedulingConfig(
            rt_reserved_slots=1,
            aging_threshold_seconds=aging_threshold_seconds,
            aging_boost=0.5,
            preemption_enabled=True,
            preemption_wait_threshold_seconds=2.0,
            priority_promotion_threshold_seconds=120.0,
            guaranteed_be_ratio=5,
            max_queue_depth=max_queue_depth,
            rt_mode="fifo",
        )

        assert config.rt_reserved_slots == 1
        assert config.aging_threshold_seconds == aging_threshold_seconds
        assert config.preemption_enabled is True
        assert config.max_queue_depth == max_queue_depth
        assert config.rt_mode == "fifo"

    def test_scheduling_config_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """SchedulingConfig.from_env() should load from environment variables."""
        expected_rt_slots = 2
        expected_aging_threshold = 30.0
        monkeypatch.setenv("SCHEDULING_RT_RESERVED_SLOTS", str(expected_rt_slots))
        monkeypatch.setenv(
            "SCHEDULING_AGING_THRESHOLD_SECONDS", str(expected_aging_threshold)
        )
        monkeypatch.setenv("SCHEDULING_PREEMPTION_ENABLED", "false")

        config = SchedulingConfig.from_env()

        assert config.rt_reserved_slots == expected_rt_slots
        assert config.aging_threshold_seconds == expected_aging_threshold
        assert config.preemption_enabled is False

    def test_scheduling_config_aging_sweep_interval_releases_default(self) -> None:
        """aging_sweep_interval_releases should default to 8 releases.

        Bounds how often the O(N) _apply_aging() sweep runs on release() --
        without this, a burst of releases (e.g. hierarchical job fan-out)
        rebuilds the whole BE queue on every single release.
        """
        default_sweep_interval = 8
        config = SchedulingConfig()

        assert config.aging_sweep_interval_releases == default_sweep_interval

    def test_scheduling_config_from_env_aging_sweep_interval_releases(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """SCHEDULING_AGING_SWEEP_INTERVAL_RELEASES must be read from env."""
        expected_sweep_interval = 16
        monkeypatch.setenv(
            "SCHEDULING_AGING_SWEEP_INTERVAL_RELEASES", str(expected_sweep_interval)
        )

        config = SchedulingConfig.from_env()

        assert config.aging_sweep_interval_releases == expected_sweep_interval

    def test_scheduling_config_from_env_invalid_int_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """SchedulingConfig.from_env() must raise ValueError when an int env var is invalid."""
        monkeypatch.setenv("OLLAMA_NUM_PARALLEL", "invalid")

        with pytest.raises(ValueError, match="OLLAMA_NUM_PARALLEL"):
            SchedulingConfig.from_env()

    def test_scheduling_config_from_env_invalid_float_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """SchedulingConfig.from_env() must raise ValueError when a float env var is invalid."""
        monkeypatch.setenv("SCHEDULING_AGING_THRESHOLD_SECONDS", "60s")

        with pytest.raises(ValueError, match="SCHEDULING_AGING_THRESHOLD_SECONDS"):
            SchedulingConfig.from_env()


class TestHierarchicalConfig:
    """Tests for HierarchicalConfig dataclass."""

    def test_hierarchical_config_is_frozen_dataclass(self) -> None:
        """HierarchicalConfig should be an immutable frozen dataclass."""
        config = HierarchicalConfig(
            threshold_chars=8000,
            threshold_clusters=5,
            chunk_max_chars=6000,
        )

        with pytest.raises(
            AttributeError, match="cannot assign to field 'threshold_chars'"
        ):
            config.threshold_chars = 10000

    def test_hierarchical_config_has_required_fields(self) -> None:
        """HierarchicalConfig should have all hierarchical summarization fields."""
        threshold_chars = 8000
        threshold_clusters = 5
        chunk_overlap_ratio = 0.15
        recursive_reduce_max_depth = 3
        config = HierarchicalConfig(
            threshold_chars=threshold_chars,
            threshold_clusters=threshold_clusters,
            chunk_max_chars=6000,
            chunk_overlap_ratio=chunk_overlap_ratio,
            recursive_reduce_max_chars=6000,
            recursive_reduce_max_depth=recursive_reduce_max_depth,
            single_article_threshold=20000,
            single_article_chunk_size=6000,
            token_budget_percent=75,
        )

        assert config.threshold_chars == threshold_chars
        assert config.threshold_clusters == threshold_clusters
        assert config.chunk_overlap_ratio == chunk_overlap_ratio
        assert config.recursive_reduce_max_depth == recursive_reduce_max_depth

    def test_hierarchical_config_from_env(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """HierarchicalConfig.from_env() should load from environment variables."""
        expected_threshold_chars = 10000
        expected_threshold_clusters = 10
        monkeypatch.setenv(
            "HIERARCHICAL_THRESHOLD_CHARS", str(expected_threshold_chars)
        )
        monkeypatch.setenv(
            "HIERARCHICAL_THRESHOLD_CLUSTERS", str(expected_threshold_clusters)
        )

        config = HierarchicalConfig.from_env()

        assert config.threshold_chars == expected_threshold_chars
        assert config.threshold_clusters == expected_threshold_clusters

    def test_hierarchical_config_from_env_invalid_int_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """HierarchicalConfig.from_env() must raise ValueError when an int env var is invalid."""
        monkeypatch.setenv("HIERARCHICAL_THRESHOLD_CHARS", "8k")

        with pytest.raises(ValueError, match="HIERARCHICAL_THRESHOLD_CHARS"):
            HierarchicalConfig.from_env()

    def test_hierarchical_config_from_env_invalid_float_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """HierarchicalConfig.from_env() must raise ValueError when a float env var is invalid."""
        monkeypatch.setenv("HIERARCHICAL_CHUNK_OVERLAP_RATIO", "fifteen_percent")

        with pytest.raises(ValueError, match="HIERARCHICAL_CHUNK_OVERLAP_RATIO"):
            HierarchicalConfig.from_env()


class TestModelRoutingConfig:
    """Tests for ModelRoutingConfig dataclass."""

    def test_model_routing_config_is_frozen_dataclass(self) -> None:
        """ModelRoutingConfig should be an immutable frozen dataclass."""
        config = ModelRoutingConfig(
            enabled=True,
            base_name="gemma4-e4b-q4km",
            model_8k_name="gemma4-e4b-q4km",
            model_60k_name="gemma4-e4b-60k",
        )

        with pytest.raises(AttributeError, match="cannot assign to field 'enabled'"):
            config.enabled = False

    def test_model_routing_config_has_required_fields(self) -> None:
        """ModelRoutingConfig should have all model routing fields."""
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

    def test_model_routing_config_is_base_model_name(self) -> None:
        """ModelRoutingConfig should provide is_base_model_name() method."""
        config = ModelRoutingConfig(
            enabled=True,
            base_name="gemma4-e4b-q4km",
            model_8k_name="gemma4-e4b-q4km",
            model_60k_name="gemma4-e4b-60k",
        )

        assert config.is_base_model_name("gemma4-e4b-q4km") is True
        assert config.is_base_model_name("other-model") is False

    def test_model_routing_config_is_bucket_model_name(self) -> None:
        """ModelRoutingConfig should provide is_bucket_model_name() method."""
        config = ModelRoutingConfig(
            enabled=True,
            base_name="gemma4-e4b-q4km",
            model_8k_name="gemma4-e4b-q4km",
            model_60k_name="gemma4-e4b-60k",
        )

        assert config.is_bucket_model_name("gemma4-e4b-q4km") is True
        assert config.is_bucket_model_name("gemma4-e4b-60k") is True
        assert config.is_bucket_model_name("other-model") is False

    def test_model_routing_config_from_env(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """ModelRoutingConfig.from_env() should load from environment variables."""
        monkeypatch.setenv("MODEL_ROUTING_ENABLED", "false")
        monkeypatch.setenv("MODEL_BASE_NAME", "custom-base")

        config = ModelRoutingConfig.from_env()

        assert config.enabled is False
        assert config.base_name == "custom-base"

    def test_model_routing_config_from_env_invalid_int_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """ModelRoutingConfig.from_env() must raise ValueError when an int env var is invalid."""
        monkeypatch.setenv("TOKEN_SAFETY_MARGIN_PERCENT", "ten")

        with pytest.raises(ValueError, match="TOKEN_SAFETY_MARGIN_PERCENT"):
            ModelRoutingConfig.from_env()


class TestNewsCreatorConfigComposition:
    """Tests for NewsCreatorConfig composition with new config classes."""

    def test_news_creator_config_composes_sub_configs(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig should compose LLMConfig, SchedulingConfig, etc."""
        config = NewsCreatorConfig()

        # Should have composed sub-configs
        assert hasattr(config, "llm")
        assert hasattr(config, "scheduling")
        assert hasattr(config, "hierarchical")
        assert hasattr(config, "model_routing")

    def test_news_creator_config_backward_compatible_llm_fields(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig should maintain backward compatible LLM fields."""
        monkeypatch.setenv("LLM_SERVICE_URL", "http://test:11435")

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.llm_service_url == "http://test:11435"
        # New-style access should also work
        assert config.llm.service_url == "http://test:11435"

    def test_news_creator_config_backward_compatible_scheduling_fields(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig should maintain backward compatible scheduling fields."""
        expected_rt_slots = 2
        monkeypatch.setenv("SCHEDULING_RT_RESERVED_SLOTS", str(expected_rt_slots))

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.scheduling_rt_reserved_slots == expected_rt_slots
        # New-style access should also work
        assert config.scheduling.rt_reserved_slots == expected_rt_slots

    def test_news_creator_config_backward_compatible_aging_sweep_interval(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig should expose aging_sweep_interval_releases."""
        expected_sweep_interval = 12
        monkeypatch.setenv(
            "SCHEDULING_AGING_SWEEP_INTERVAL_RELEASES", str(expected_sweep_interval)
        )

        config = NewsCreatorConfig()

        assert (
            config.scheduling_aging_sweep_interval_releases == expected_sweep_interval
        )
        assert (
            config.scheduling.aging_sweep_interval_releases == expected_sweep_interval
        )

    def test_news_creator_config_backward_compatible_hierarchical_fields(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig should maintain backward compatible hierarchical fields."""
        expected_threshold_chars = 10000
        monkeypatch.setenv(
            "HIERARCHICAL_THRESHOLD_CHARS", str(expected_threshold_chars)
        )

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.hierarchical_threshold_chars == expected_threshold_chars
        # New-style access should also work
        assert config.hierarchical.threshold_chars == expected_threshold_chars

    def test_news_creator_config_backward_compatible_model_routing_fields(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig should maintain backward compatible model routing fields."""
        monkeypatch.setenv("MODEL_ROUTING_ENABLED", "false")

        config = NewsCreatorConfig()

        # Old-style access should still work
        assert config.model_routing_enabled is False
        # New-style access should also work
        assert config.model_routing.enabled is False

    def test_news_creator_config_get_llm_options_still_works(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig.get_llm_options() should still work after refactoring."""
        expected_num_ctx = 4096
        expected_temperature = 0.5
        monkeypatch.setenv("LLM_NUM_CTX", str(expected_num_ctx))
        monkeypatch.setenv("LLM_TEMPERATURE", str(expected_temperature))

        config = NewsCreatorConfig()
        options = config.get_llm_options()

        assert options["num_ctx"] == expected_num_ctx
        assert options["temperature"] == expected_temperature

    def test_news_creator_config_is_base_model_name_still_works(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig.is_base_model_name() should still work after refactoring."""
        monkeypatch.setenv("MODEL_BASE_NAME", "test-base")

        config = NewsCreatorConfig()

        assert config.is_base_model_name("test-base") is True
        assert config.is_base_model_name("other") is False

    def test_news_creator_config_is_bucket_model_name_still_works(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig.is_bucket_model_name() should still work after refactoring."""
        monkeypatch.setenv("MODEL_8K_NAME", "model-8k")
        monkeypatch.setenv("MODEL_60K_NAME", "model-60k")

        config = NewsCreatorConfig()

        assert config.is_bucket_model_name("model-8k") is True
        assert config.is_bucket_model_name("model-60k") is True
        assert config.is_bucket_model_name("other") is False

    def test_news_creator_config_get_keep_alive_for_model_still_works(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """NewsCreatorConfig.get_keep_alive_for_model() should still work after refactoring."""
        monkeypatch.setenv("MODEL_8K_NAME", "model-8k")
        monkeypatch.setenv("LLM_KEEP_ALIVE_8K", "48h")

        config = NewsCreatorConfig()

        assert config.get_keep_alive_for_model("model-8k") == "48h"
