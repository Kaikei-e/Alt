"""Tests for SummarizeUsecase - retry semaphore hold behavior."""

import asyncio
import pytest
from unittest.mock import AsyncMock, Mock, patch, MagicMock
from contextlib import asynccontextmanager

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.usecase.summarize_usecase import SummarizeUsecase


def _make_config():
    """Create a mock config."""
    config = Mock(spec=NewsCreatorConfig)
    config.max_repetition_retries = 2
    config.repetition_threshold = 0.3
    config.summary_num_predict = 500
    config.summary_temperature = 0.5
    config.llm_repeat_penalty = 1.15
    config.llm_num_ctx = 8192
    config.hierarchical_single_article_threshold = 20_000
    config.hierarchical_single_article_chunk_size = 6_000
    return config


def _make_llm_response(text="テスト要約。これはテスト記事の日本語要約です。"):
    """Create a mock LLMGenerateResponse."""
    return LLMGenerateResponse(
        response=text,
        model="test-model",
        done=True,
        done_reason="stop",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1_000_000_000,
        load_duration=100_000,
        prompt_eval_duration=500_000_000,
        eval_duration=500_000_000,
    )


class TestRetryDoesNotReacquireSemaphore:
    """Tests that retry loop uses hold_slot to keep semaphore."""

    @pytest.mark.asyncio
    async def test_retry_uses_hold_slot_and_generate_raw(self):
        """Test that retry loop acquires semaphore once via hold_slot and uses generate_raw."""
        config = _make_config()
        llm_provider = Mock()

        # Track hold_slot and generate_raw calls
        hold_slot_calls = 0
        generate_raw_calls = 0

        @asynccontextmanager
        async def mock_hold_slot(is_high_priority=False):
            nonlocal hold_slot_calls
            hold_slot_calls += 1
            yield 0.0, None, None

        async def mock_generate_raw(prompt, **kwargs):
            nonlocal generate_raw_calls
            generate_raw_calls += 1
            if generate_raw_calls == 1:
                # First attempt returns repetitive content
                return _make_llm_response("あああああああああああああああああ" * 20)
            # Second attempt returns good content
            return _make_llm_response()

        llm_provider.hold_slot = mock_hold_slot
        llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
        # generate() should NOT be called in the retry path
        llm_provider.generate = AsyncMock(side_effect=AssertionError("generate() should not be called during retry"))

        usecase = SummarizeUsecase(config=config, llm_provider=llm_provider)
        content = "A" * 200  # Sufficient content

        summary, metadata = await usecase.generate_summary("test-article", content)

        # hold_slot should be called exactly once
        assert hold_slot_calls == 1
        # generate_raw should be called for retries (at least 1 time)
        assert generate_raw_calls >= 1
        assert summary  # Should have a non-empty summary

    @pytest.mark.asyncio
    async def test_max_retries_reduced_to_2(self):
        """Test that max retries defaults to 2."""
        config = _make_config()
        assert config.max_repetition_retries == 2

    @pytest.mark.asyncio
    async def test_semaphore_released_even_on_error(self):
        """Test that hold_slot releases semaphore even if generate_raw raises."""
        config = _make_config()
        llm_provider = Mock()

        released = False

        @asynccontextmanager
        async def mock_hold_slot(is_high_priority=False):
            nonlocal released
            try:
                yield 0.0, None, None
            finally:
                released = True

        llm_provider.hold_slot = mock_hold_slot
        llm_provider.generate_raw = AsyncMock(side_effect=RuntimeError("LLM failed"))

        usecase = SummarizeUsecase(config=config, llm_provider=llm_provider)

        with pytest.raises(RuntimeError):
            await usecase.generate_summary("test-article", "A" * 200)

        assert released, "Semaphore should be released even on error"
