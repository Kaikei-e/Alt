"""Tests for ExpandQueryUsecase."""

import pytest
from unittest.mock import AsyncMock, Mock

from news_creator.domain.models import LLMGenerateResponse
from news_creator.usecase.expand_query_usecase import ExpandQueryUsecase


@pytest.mark.asyncio
async def test_expand_query_success():
    """Test successful query expansion with Japanese and English queries."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="""AI技術の最新動向2025年
AI technology trends 2025
machine learning advances
generative AI development""",
        model="gemma3-4b-8k",
        prompt_eval_count=256,
        eval_count=64,
        total_duration=500_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    expanded_queries, model, elapsed_ms = await usecase.expand_query(
        query="AI技術のトレンド",
        japanese_count=1,
        english_count=3,
    )

    assert len(expanded_queries) == 4
    assert model == "gemma3-4b-8k"
    assert elapsed_ms > 0

    # Verify LLM was called with correct model
    llm_provider.generate.assert_called_once()
    call_args = llm_provider.generate.call_args
    assert call_args.kwargs.get("model") == "gemma3-4b-8k"


@pytest.mark.asyncio
async def test_expand_query_filters_labels():
    """Test that query expansion filters out labels and empty lines."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="""Japanese:
AI技術の最新動向

English:
1. AI technology trends
2. machine learning advances
- generative AI development
• neural network research""",
        model="gemma3-4b-8k",
        prompt_eval_count=256,
        eval_count=64,
        total_duration=500_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    expanded_queries, model, _ = await usecase.expand_query(
        query="AI技術のトレンド",
        japanese_count=1,
        english_count=4,
    )

    # Should filter out "Japanese:", "English:", empty lines, and strip numbering/bullets
    assert "Japanese:" not in expanded_queries
    assert "English:" not in expanded_queries
    assert "" not in expanded_queries
    assert len(expanded_queries) == 5
    assert "AI技術の最新動向" in expanded_queries
    assert "AI technology trends" in expanded_queries
    assert "machine learning advances" in expanded_queries
    assert "generative AI development" in expanded_queries
    assert "neural network research" in expanded_queries


@pytest.mark.asyncio
async def test_expand_query_empty_query_raises_error():
    """Test that empty query raises ValueError."""
    config = Mock()
    llm_provider = AsyncMock()

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    with pytest.raises(ValueError, match="query cannot be empty"):
        await usecase.expand_query(query="", japanese_count=1, english_count=3)

    with pytest.raises(ValueError, match="query cannot be empty"):
        await usecase.expand_query(query="   ", japanese_count=1, english_count=3)


@pytest.mark.asyncio
async def test_expand_query_llm_failure():
    """Test that LLM failure raises RuntimeError."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.side_effect = Exception("LLM connection failed")

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    with pytest.raises(RuntimeError, match="Query expansion failed"):
        await usecase.expand_query(query="test query", japanese_count=1, english_count=3)


@pytest.mark.asyncio
async def test_expand_query_uses_correct_model():
    """Test that query expansion uses gemma3-4b-8k model specifically."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="test query result",
        model="gemma3-4b-8k",
        prompt_eval_count=100,
        eval_count=20,
        total_duration=100_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    await usecase.expand_query(query="test", japanese_count=1, english_count=1)

    # Verify the EXPANSION_MODEL constant is used
    assert usecase.EXPANSION_MODEL == "gemma3-4b-8k"
    call_args = llm_provider.generate.call_args
    assert call_args.kwargs.get("model") == "gemma3-4b-8k"


@pytest.mark.asyncio
async def test_expand_query_low_temperature():
    """Test that query expansion uses low temperature for consistent results."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="test query",
        model="gemma3-4b-8k",
        prompt_eval_count=100,
        eval_count=20,
        total_duration=100_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    await usecase.expand_query(query="test", japanese_count=1, english_count=1)

    call_args = llm_provider.generate.call_args
    options = call_args.kwargs.get("options", {})
    assert options.get("temperature") == 0.3
