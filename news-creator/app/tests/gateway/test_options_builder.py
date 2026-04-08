"""Tests for OptionsBuilder (Phase 2 refactoring).

Following Python 3.14 best practices:
- Protocol-based structural typing
- Dataclass for option results
"""

from __future__ import annotations


class TestOptionsBuilder:
    """Tests for OptionsBuilder class."""

    def test_options_builder_builds_base_options_from_config(self, monkeypatch):
        """OptionsBuilder should build base options from LLMConfig."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.gateway.options_builder import OptionsBuilder

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            num_ctx=8192,
            num_batch=1024,
            num_predict=1200,
            temperature=0.7,
            top_p=0.85,
            top_k=40,
            repeat_penalty=1.15,
            num_keep=-1,
            stop_tokens=("<turn|>",),
        )

        builder = OptionsBuilder(llm_config)
        options = builder.build()

        assert options["num_ctx"] == 8192
        assert options["num_predict"] == 1200
        assert options["temperature"] == 0.7
        assert options["top_p"] == 0.85
        assert options["stop"] == ["<turn|>"]

    def test_options_builder_merges_extra_options(self):
        """OptionsBuilder should merge extra options, filtering num_ctx."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.gateway.options_builder import OptionsBuilder

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            temperature=0.7,
        )

        builder = OptionsBuilder(llm_config)
        extra_options = {
            "temperature": 0.9,  # Override
            "num_ctx": 16384,  # Should be filtered out
            "custom_param": "value",  # Should be kept
        }

        options = builder.build(extra_options=extra_options)

        assert options["temperature"] == 0.9  # Overridden
        assert options["num_ctx"] == 8192  # Original from config, not overridden
        assert options["custom_param"] == "value"

    def test_options_builder_applies_num_predict_override(self):
        """OptionsBuilder should apply num_predict override."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.gateway.options_builder import OptionsBuilder

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            num_predict=1200,
        )

        builder = OptionsBuilder(llm_config)
        options = builder.build(num_predict_override=500)

        assert options["num_predict"] == 500

    def test_options_builder_num_predict_override_beats_extra_options(self):
        """num_predict_override should take precedence over extra_options."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.gateway.options_builder import OptionsBuilder

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            num_predict=1200,
        )

        builder = OptionsBuilder(llm_config)
        extra_options = {"num_predict": 800}
        options = builder.build(extra_options=extra_options, num_predict_override=500)

        assert options["num_predict"] == 500  # Override wins

    def test_options_builder_returns_immutable_base_on_no_overrides(self):
        """OptionsBuilder should return consistent options when no overrides."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.gateway.options_builder import OptionsBuilder

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
        )

        builder = OptionsBuilder(llm_config)
        options1 = builder.build()
        options2 = builder.build()

        # Should be equal but separate instances
        assert options1 == options2
        assert options1 is not options2


class TestKeepAliveResolver:
    """Tests for KeepAliveResolver class."""

    def test_keep_alive_resolver_returns_explicit_keep_alive(self):
        """KeepAliveResolver should return explicit keep_alive when provided."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.config.model_routing_config import ModelRoutingConfig
        from news_creator.gateway.options_builder import KeepAliveResolver

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            keep_alive="24h",
            keep_alive_8k="24h",
            keep_alive_60k="15m",
        )
        routing_config = ModelRoutingConfig(
            enabled=True,
            base_name="test-model",
            model_8k_name="model-8k",
            model_60k_name="model-60k",
        )

        resolver = KeepAliveResolver(llm_config, routing_config)
        result = resolver.resolve(model="model-8k", explicit_keep_alive="30m")

        assert result == "30m"  # Explicit wins

    def test_keep_alive_resolver_returns_model_specific_for_8k(self):
        """KeepAliveResolver should return 8K-specific keep_alive."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.config.model_routing_config import ModelRoutingConfig
        from news_creator.gateway.options_builder import KeepAliveResolver

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            keep_alive="24h",
            keep_alive_8k="48h",
            keep_alive_60k="15m",
        )
        routing_config = ModelRoutingConfig(
            enabled=True,
            base_name="test-model",
            model_8k_name="model-8k",
            model_60k_name="model-60k",
        )

        resolver = KeepAliveResolver(llm_config, routing_config)
        result = resolver.resolve(model="model-8k")

        assert result == "48h"

    def test_keep_alive_resolver_returns_model_specific_for_60k(self):
        """KeepAliveResolver should return 60K-specific keep_alive."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.config.model_routing_config import ModelRoutingConfig
        from news_creator.gateway.options_builder import KeepAliveResolver

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            keep_alive="24h",
            keep_alive_8k="48h",
            keep_alive_60k="10m",
        )
        routing_config = ModelRoutingConfig(
            enabled=True,
            base_name="test-model",
            model_8k_name="model-8k",
            model_60k_name="model-60k",
        )

        resolver = KeepAliveResolver(llm_config, routing_config)
        result = resolver.resolve(model="model-60k")

        assert result == "10m"

    def test_keep_alive_resolver_returns_default_for_unknown_model(self):
        """KeepAliveResolver should return default for unknown models."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.config.model_routing_config import ModelRoutingConfig
        from news_creator.gateway.options_builder import KeepAliveResolver

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
            keep_alive="24h",
            keep_alive_8k="48h",
            keep_alive_60k="10m",
        )
        routing_config = ModelRoutingConfig(
            enabled=True,
            base_name="test-model",
            model_8k_name="model-8k",
            model_60k_name="model-60k",
        )

        resolver = KeepAliveResolver(llm_config, routing_config)
        result = resolver.resolve(model="unknown-model")

        assert result == "24h"  # Default


class TestOptionsBuilderProtocol:
    """Tests for OptionsBuilder Protocol compliance."""

    def test_options_builder_implements_protocol(self):
        """OptionsBuilder should implement OptionsBuilderProtocol."""
        from news_creator.config.llm_config import LLMConfig
        from news_creator.gateway.options_builder import (
            OptionsBuilder,
        )

        llm_config = LLMConfig(
            service_url="http://localhost:11435",
            model_name="test-model",
            timeout_seconds=300,
        )

        builder = OptionsBuilder(llm_config)

        # Should be structurally compatible with protocol
        assert hasattr(builder, "build")
        assert callable(builder.build)

        # Static type check would verify Protocol compliance
        # Runtime check: can call the method
        options = builder.build()
        assert isinstance(options, dict)
