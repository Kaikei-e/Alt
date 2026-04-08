"""Tests for PayloadBuilder (Phase 3 refactoring).

Following Python 3.14 best practices:
- Dataclass for payload representation
- Protocol for structural typing
"""

from __future__ import annotations

import pytest


class TestPayloadBuilder:
    """Tests for PayloadBuilder class."""

    def test_payload_builder_creates_basic_payload(self):
        """PayloadBuilder should create basic Ollama API payload."""
        from news_creator.gateway.payload_builder import PayloadBuilder, GeneratePayload

        builder = PayloadBuilder()
        payload = builder.build(
            prompt="Test prompt",
            model="test-model",
            options={"temperature": 0.7},
            keep_alive="24h",
        )

        assert isinstance(payload, GeneratePayload)
        assert payload.model == "test-model"
        assert payload.prompt == "Test prompt"
        assert payload.options == {"temperature": 0.7}
        assert payload.keep_alive == "24h"
        assert payload.raw is True  # Default for Gemma 4 compatibility
        assert payload.stream is False  # Default

    def test_payload_builder_strips_prompt_whitespace(self):
        """PayloadBuilder should strip whitespace from prompt."""
        from news_creator.gateway.payload_builder import PayloadBuilder

        builder = PayloadBuilder()
        payload = builder.build(
            prompt="  Test prompt with whitespace  \n",
            model="test-model",
            options={},
            keep_alive="24h",
        )

        assert payload.prompt == "Test prompt with whitespace"

    def test_payload_builder_handles_streaming(self):
        """PayloadBuilder should set stream flag correctly."""
        from news_creator.gateway.payload_builder import PayloadBuilder

        builder = PayloadBuilder()
        payload = builder.build(
            prompt="Test",
            model="test-model",
            options={},
            keep_alive="24h",
            stream=True,
        )

        assert payload.stream is True

    def test_payload_builder_includes_format_when_provided(self):
        """PayloadBuilder should include format parameter when provided."""
        from news_creator.gateway.payload_builder import PayloadBuilder

        builder = PayloadBuilder()

        # String format
        payload = builder.build(
            prompt="Test",
            model="test-model",
            options={},
            keep_alive="24h",
            format="json",
        )
        assert payload.format == "json"

        # Dict format (structured output schema)
        schema = {"type": "object", "properties": {"name": {"type": "string"}}}
        payload2 = builder.build(
            prompt="Test",
            model="test-model",
            options={},
            keep_alive="24h",
            format=schema,
        )
        assert payload2.format == schema

    def test_payload_builder_format_is_none_by_default(self):
        """PayloadBuilder should have format=None when not provided."""
        from news_creator.gateway.payload_builder import PayloadBuilder

        builder = PayloadBuilder()
        payload = builder.build(
            prompt="Test",
            model="test-model",
            options={},
            keep_alive="24h",
        )

        assert payload.format is None

    def test_payload_builder_raw_defaults_to_true(self):
        """PayloadBuilder should default raw=True for Gemma 4 compatibility."""
        from news_creator.gateway.payload_builder import PayloadBuilder

        builder = PayloadBuilder()
        payload = builder.build(
            prompt="Test",
            model="test-model",
            options={},
            keep_alive="24h",
        )

        assert payload.raw is True

    def test_payload_builder_allows_raw_override(self):
        """PayloadBuilder should allow raw to be overridden."""
        from news_creator.gateway.payload_builder import PayloadBuilder

        builder = PayloadBuilder()
        payload = builder.build(
            prompt="Test",
            model="test-model",
            options={},
            keep_alive="24h",
            raw=False,
        )

        assert payload.raw is False


class TestGeneratePayload:
    """Tests for GeneratePayload dataclass."""

    def test_generate_payload_to_dict(self):
        """GeneratePayload should convert to dict for API call."""
        from news_creator.gateway.payload_builder import GeneratePayload

        payload = GeneratePayload(
            model="test-model",
            prompt="Test prompt",
            options={"temperature": 0.7},
            keep_alive="24h",
            stream=False,
            raw=True,
            format=None,
        )

        result = payload.to_dict()

        assert result["model"] == "test-model"
        assert result["prompt"] == "Test prompt"
        assert result["options"] == {"temperature": 0.7}
        assert result["keep_alive"] == "24h"
        assert result["stream"] is False
        assert result["raw"] is True
        assert "format" not in result  # Should be excluded when None

    def test_generate_payload_to_dict_includes_format_when_set(self):
        """GeneratePayload.to_dict() should include format when set."""
        from news_creator.gateway.payload_builder import GeneratePayload

        payload = GeneratePayload(
            model="test-model",
            prompt="Test",
            options={},
            keep_alive="24h",
            stream=False,
            raw=True,
            format="json",
        )

        result = payload.to_dict()

        assert result["format"] == "json"

    def test_generate_payload_is_frozen(self):
        """GeneratePayload should be immutable."""
        from news_creator.gateway.payload_builder import GeneratePayload

        payload = GeneratePayload(
            model="test-model",
            prompt="Test",
            options={},
            keep_alive="24h",
            stream=False,
            raw=True,
        )

        with pytest.raises(AttributeError):
            payload.model = "other-model"


class TestPayloadBuilderProtocol:
    """Tests for PayloadBuilder Protocol compliance."""

    def test_payload_builder_implements_protocol(self):
        """PayloadBuilder should implement PayloadBuilderProtocol."""
        from news_creator.gateway.payload_builder import (
            PayloadBuilder,
            PayloadBuilderProtocol,
        )

        builder = PayloadBuilder()

        # Should be structurally compatible with protocol
        assert hasattr(builder, "build")
        assert callable(builder.build)

        # Runtime check: can call the method
        payload = builder.build(
            prompt="Test",
            model="model",
            options={},
            keep_alive="24h",
        )
        assert payload is not None
