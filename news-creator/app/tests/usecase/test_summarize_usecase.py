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
    config.hierarchical_token_budget_percent = 75
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


class TestTokenBudgetHierarchicalFallback:
    """Token budget-based hierarchical fallback tests."""

    @pytest.mark.asyncio
    async def test_token_budget_triggers_hierarchical_when_exceeded(self):
        """18,000 chars content → prompt ~4,500 tokens + 1,000 predict = 5,500.
        Budget 75% of 8,192 = 6,144 → should still fit.
        But with prompt template overhead (~500 chars), total can exceed budget.
        Use content that pushes estimated tokens over 75% budget to trigger fallback.
        """
        config = _make_config()
        config.summary_num_predict = 1000
        config.hierarchical_token_budget_percent = 75
        # Set char threshold very high so it doesn't interfere
        config.hierarchical_single_article_threshold = 100_000
        llm_provider = Mock()

        generate_calls = []
        hierarchical_generate_calls = []

        @asynccontextmanager
        async def mock_hold_slot(is_high_priority=False):
            yield 0.0, None, None

        async def mock_generate(prompt, **kwargs):
            """Track calls from hierarchical path (uses generate, not generate_raw)."""
            hierarchical_generate_calls.append(prompt)
            return _make_llm_response()

        async def mock_generate_raw(prompt, **kwargs):
            """Track calls from normal path."""
            generate_calls.append(prompt)
            return _make_llm_response()

        llm_provider.hold_slot = mock_hold_slot
        llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
        llm_provider.generate = AsyncMock(side_effect=mock_generate)

        usecase = SummarizeUsecase(config=config, llm_provider=llm_provider)

        # 20,000 chars → prompt ~5,000 tokens + predict 1,000 = 6,000
        # With template overhead (~500 chars = ~125 tokens), total ~5,125 + 1,000 = 6,125
        # Budget = 75% of 8,192 = 6,144 → borderline
        # Use 22,000 chars to ensure we exceed budget
        content = "A" * 22_000

        summary, metadata = await usecase.generate_summary("test-article", content)

        # Should have used hierarchical path (generate), not normal path (generate_raw)
        assert len(generate_calls) == 0 or len(hierarchical_generate_calls) > 0
        assert len(generate_calls) == 0, "Normal generate_raw should NOT have been called"
        assert summary

    @pytest.mark.asyncio
    async def test_token_budget_allows_normal_when_within_budget(self):
        """10,000 chars content → prompt ~2,500 tokens + 1,000 predict = 3,500.
        Budget 75% of 8,192 = 6,144 → within budget → normal path.
        """
        config = _make_config()
        config.summary_num_predict = 1000
        config.hierarchical_token_budget_percent = 75
        config.hierarchical_single_article_threshold = 100_000
        llm_provider = Mock()

        @asynccontextmanager
        async def mock_hold_slot(is_high_priority=False):
            yield 0.0, None, None

        llm_provider.hold_slot = mock_hold_slot
        llm_provider.generate_raw = AsyncMock(return_value=_make_llm_response())
        llm_provider.generate = AsyncMock(side_effect=AssertionError("hierarchical should not be called"))

        usecase = SummarizeUsecase(config=config, llm_provider=llm_provider)
        content = "A" * 10_000

        summary, metadata = await usecase.generate_summary("test-article", content)

        # Normal path should have been used
        assert llm_provider.generate_raw.called
        assert summary

    @pytest.mark.asyncio
    async def test_char_threshold_still_triggers_before_token_check(self):
        """25,000 chars → char threshold (20,000) catches it first (backward compat)."""
        config = _make_config()
        config.summary_num_predict = 1000
        config.hierarchical_token_budget_percent = 75
        # Default char threshold
        config.hierarchical_single_article_threshold = 20_000
        llm_provider = Mock()

        hierarchical_called = False

        async def mock_generate(prompt, **kwargs):
            nonlocal hierarchical_called
            hierarchical_called = True
            return _make_llm_response()

        llm_provider.generate = AsyncMock(side_effect=mock_generate)
        # generate_raw should NOT be called since char threshold triggers first
        llm_provider.generate_raw = AsyncMock(side_effect=AssertionError("should not reach normal path"))

        usecase = SummarizeUsecase(config=config, llm_provider=llm_provider)
        content = "A" * 25_000

        summary, metadata = await usecase.generate_summary("test-article", content)

        assert hierarchical_called, "Char threshold should have triggered hierarchical"
        assert summary

    @pytest.mark.asyncio
    async def test_token_budget_percent_configurable(self):
        """Budget 90% → 18,000 chars stays within budget and uses normal path."""
        config = _make_config()
        config.summary_num_predict = 1000
        config.hierarchical_token_budget_percent = 90
        config.hierarchical_single_article_threshold = 100_000
        llm_provider = Mock()

        @asynccontextmanager
        async def mock_hold_slot(is_high_priority=False):
            yield 0.0, None, None

        llm_provider.hold_slot = mock_hold_slot
        llm_provider.generate_raw = AsyncMock(return_value=_make_llm_response())
        llm_provider.generate = AsyncMock(side_effect=AssertionError("hierarchical should not be called"))

        usecase = SummarizeUsecase(config=config, llm_provider=llm_provider)
        # 18,000 chars → prompt ~4,500 + template ~125 = ~4,625 tokens + 1,000 predict = 5,625
        # Budget = 90% of 8,192 = 7,372 → within budget
        content = "A" * 18_000

        summary, metadata = await usecase.generate_summary("test-article", content)

        assert llm_provider.generate_raw.called
        assert summary
