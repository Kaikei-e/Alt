"""Unit tests for SummarizeUsecase.generate_summary_stream()."""

from unittest.mock import AsyncMock, Mock

import aiohttp
import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.usecase.summarize_usecase import SummarizeUsecase


def _make_config() -> Mock:
    config = Mock(spec=NewsCreatorConfig)
    config.max_repetition_retries = 2
    config.repetition_threshold = 0.3
    config.summary_num_predict = 500
    config.summary_temperature = 0.5
    config.llm_repeat_penalty = 1.15
    config.llm_num_ctx = 8192
    config.hierarchical_single_article_threshold = 20_000
    config.hierarchical_single_article_chunk_size = 6_000
    config.hierarchical_token_budget_percent = 75
    return config


class TestGenerateSummaryStream:
    """Specification tests for SummarizeUsecase.generate_summary_stream()."""

    @pytest.mark.asyncio
    async def test_happy_path_strips_control_tokens_and_forwards_high_priority(self):
        """Yields sanitized token chunks (stripping Gemma 4 control tokens) and passes priority='high'."""
        raw_chunks = [
            "<|tu",
            "rn>",
            "これはテスト要約",
            "のチャンク<turn|>",
            "です。",
        ]

        async def _stream():
            for idx, text in enumerate(raw_chunks):
                yield LLMGenerateResponse(
                    response=text,
                    model="gemma4-e4b-12k",
                    done=idx == len(raw_chunks) - 1,
                )

        llm_provider = Mock()
        llm_provider.generate = AsyncMock(return_value=_stream())

        usecase = SummarizeUsecase(config=_make_config(), llm_provider=llm_provider)
        emitted = [
            token
            async for token in usecase.generate_summary_stream(
                "art-stream-1", "A" * 200, priority="high"
            )
        ]

        assert "".join(emitted) == "これはテスト要約のチャンクです。"
        llm_provider.generate.assert_awaited_once()
        call_kwargs = llm_provider.generate.call_args.kwargs
        assert call_kwargs["stream"] is True
        assert call_kwargs["priority"] == "high"

    @pytest.mark.asyncio
    async def test_rejects_short_content_under_100_chars(self):
        """Content shorter than 100 chars raises ValueError without calling LLM."""
        llm_provider = Mock()
        llm_provider.generate = AsyncMock()
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=llm_provider)

        with pytest.raises(ValueError, match="Content is too short"):
            async for _ in usecase.generate_summary_stream("art-short", "Short text"):
                pass

        llm_provider.generate.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_stream_ending_without_done_true_raises_truncated_error(self):
        """When stream ends after yielding tokens without a done=True chunk, raises RuntimeError."""

        async def _truncated_stream():
            yield LLMGenerateResponse(
                response="途中までの要約テキストです。",
                model="gemma4-e4b-12k",
                done=False,
            )

        llm_provider = Mock()
        llm_provider.generate = AsyncMock(return_value=_truncated_stream())
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=llm_provider)

        emitted: list[str] = []
        with pytest.raises(
            RuntimeError,
            match="ended before a done=true chunk was received; response is truncated",
        ):
            async for token in usecase.generate_summary_stream(
                "art-truncated", "A" * 200
            ):
                emitted.append(token)

        assert emitted == ["途中までの要約テキストです。"]

    @pytest.mark.asyncio
    async def test_client_connection_error_with_partial_tokens_raises_truncated_error(
        self,
    ):
        """When ClientConnectionError occurs after yielding partial tokens, raises RuntimeError so SSE emits error."""

        async def _interrupted_stream():
            yield LLMGenerateResponse(
                response="途中までの要約テキストです。",
                model="gemma4-e4b-12k",
                done=False,
            )
            raise aiohttp.ClientConnectionError("stream disconnected mid-generation")

        llm_provider = Mock()
        llm_provider.generate = AsyncMock(return_value=_interrupted_stream())
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=llm_provider)

        emitted: list[str] = []
        with pytest.raises(
            RuntimeError,
            match="connection closed after partial data was received; response is truncated",
        ):
            async for token in usecase.generate_summary_stream(
                "art-partial", "A" * 200
            ):
                emitted.append(token)

        assert emitted == ["途中までの要約テキストです。"]

    @pytest.mark.asyncio
    async def test_client_connection_error_with_zero_tokens_raises_runtime_error(self):
        """When ClientConnectionError occurs before any token is yielded, raises RuntimeError."""

        async def _immediate_disconnect_stream():
            if False:
                yield
            raise aiohttp.ClientConnectionError("connection reset before first token")

        llm_provider = Mock()
        llm_provider.generate = AsyncMock(return_value=_immediate_disconnect_stream())
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=llm_provider)

        with pytest.raises(
            RuntimeError,
            match="connection closed before any data was received",
        ):
            async for _ in usecase.generate_summary_stream("art-zero", "A" * 200):
                pass
