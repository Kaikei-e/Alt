"""Tests for ExpandQueryUsecase."""

import pytest
from unittest.mock import AsyncMock, Mock

from news_creator.domain.models import LLMGenerateResponse
from news_creator.usecase.expand_query_usecase import (
    ExpandQueryUsecase,
    EXPAND_QUERY_PROMPT_TEMPLATE,
    EXPAND_QUERY_WITH_HISTORY_TEMPLATE,
)


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
        model="gemma4-e4b-12k",
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
    assert model == "gemma4-e4b-12k"
    assert elapsed_ms is not None and elapsed_ms > 0

    # Verify LLM was called with correct model
    llm_provider.generate.assert_called_once()
    call_args = llm_provider.generate.call_args
    assert call_args.kwargs.get("model") == "gemma4-e4b-12k"


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
        model="gemma4-e4b-12k",
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
    """Test that query expansion uses gemma4-e4b-12k model specifically."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="test query result",
        model="gemma4-e4b-12k",
        prompt_eval_count=100,
        eval_count=20,
        total_duration=100_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    await usecase.expand_query(query="test", japanese_count=1, english_count=1)

    # Verify the EXPANSION_MODEL constant is used
    assert usecase.EXPANSION_MODEL == "gemma4-e4b-12k"
    call_args = llm_provider.generate.call_args
    assert call_args.kwargs.get("model") == "gemma4-e4b-12k"


@pytest.mark.asyncio
async def test_expand_query_low_temperature():
    """Test that query expansion uses low temperature for consistent results."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="test query",
        model="gemma4-e4b-12k",
        prompt_eval_count=100,
        eval_count=20,
        total_duration=100_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    await usecase.expand_query(query="test", japanese_count=1, english_count=1)

    call_args = llm_provider.generate.call_args
    options = call_args.kwargs.get("options", {})
    assert options.get("temperature") == 0.0


@pytest.mark.asyncio
async def test_expand_query_passes_priority_to_llm():
    """Test that query expansion passes priority to LLM provider."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="test query result",
        model="gemma4-e4b-12k",
        prompt_eval_count=100,
        eval_count=20,
        total_duration=100_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    await usecase.expand_query(
        query="test",
        japanese_count=1,
        english_count=1,
        priority="high",
    )

    call_args = llm_provider.generate.call_args
    assert call_args.kwargs.get("priority") == "high"


@pytest.mark.asyncio
async def test_expand_query_default_priority_is_low():
    """Test that query expansion defaults to low priority when not specified."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="test query result",
        model="gemma4-e4b-12k",
        prompt_eval_count=100,
        eval_count=20,
        total_duration=100_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    await usecase.expand_query(query="test", japanese_count=1, english_count=1)

    call_args = llm_provider.generate.call_args
    assert call_args.kwargs.get("priority") == "low"


# --- Phase 1 RED tests: prompt hardening + output validation ---

ECHOABLE_META_PHRASES = [
    "Output Japanese queries first",
    "Output ONLY the generated queries",
    "Do not add numbering, bullets, labels, or explanations",
    "one per line",
    "Japanese queries first, then English queries",
]


class TestPromptHasNoEchoableMeta:
    """Prompt templates must not contain meta-instructions that small models echo."""

    def test_single_turn_template_has_no_echoable_meta_lines(self):
        for phrase in ECHOABLE_META_PHRASES:
            assert phrase not in EXPAND_QUERY_PROMPT_TEMPLATE, (
                f"Template contains echoable meta: '{phrase}'"
            )

    def test_multi_turn_template_has_no_echoable_meta_lines(self):
        for phrase in ECHOABLE_META_PHRASES:
            assert phrase not in EXPAND_QUERY_WITH_HISTORY_TEMPLATE, (
                f"History template contains echoable meta: '{phrase}'"
            )


@pytest.mark.asyncio
async def test_expand_query_filters_instruction_echo():
    """When LLM echoes prompt instructions, the result must be empty."""
    config = Mock()
    llm_provider = AsyncMock()
    # Simulate the exact failure observed in production logs
    echo_line = "Japanese queries and English queries must be translated to each other."
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="\n".join([echo_line] * 8),
        model="gemma4-e4b-12k",
        prompt_eval_count=256,
        eval_count=64,
        total_duration=500_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    expanded_queries, _, _ = await usecase.expand_query(
        query="イランの石油危機はなぜ起きた？",
        japanese_count=1,
        english_count=3,
    )

    assert expanded_queries == [], (
        f"Instruction echo should be filtered out, got: {expanded_queries}"
    )


@pytest.mark.asyncio
async def test_expand_query_deduplicates_preserving_order():
    """Duplicate queries must be removed while preserving first-occurrence order."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="イランの石油危機 原因\nIran oil crisis causes\nIran oil crisis causes\nイランの石油危機 原因\noil price surge reasons",
        model="gemma4-e4b-12k",
        prompt_eval_count=256,
        eval_count=64,
        total_duration=500_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    expanded_queries, _, _ = await usecase.expand_query(
        query="イランの石油危機はなぜ起きた？",
        japanese_count=1,
        english_count=3,
    )

    # Dedup should keep first occurrence, preserve order
    assert expanded_queries == [
        "イランの石油危機 原因",
        "Iran oil crisis causes",
        "oil price surge reasons",
    ]


@pytest.mark.asyncio
async def test_expand_query_rejects_wrapped_labels_and_preamble():
    """Preamble lines like 'Here are the queries:' must be stripped."""
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="Here are the generated queries:\n\nイランの石油危機 原因\nIran oil crisis causes\nOil price surge reasons",
        model="gemma4-e4b-12k",
        prompt_eval_count=256,
        eval_count=64,
        total_duration=500_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    expanded_queries, _, _ = await usecase.expand_query(
        query="イランの石油危機はなぜ起きた？",
        japanese_count=1,
        english_count=3,
    )

    # Preamble "Here are the generated queries:" must be excluded
    assert "Here are the generated queries:" not in expanded_queries
    assert len(expanded_queries) == 3


class TestExpandQueryWithHistoryTemplateNoAIChipContamination:
    """The multi-turn few-shot example must not contain domain-specific content
    that Gemma 4 (12B) copies verbatim instead of learning the pattern."""

    def test_expand_query_with_history_template_does_not_contain_ai_chip_content(self):
        """Template must not contain AI-chip-market examples that cause contamination."""
        assert "AIチップ" not in EXPAND_QUERY_WITH_HISTORY_TEMPLATE, (
            "Template contains 'AIチップ' which causes few-shot example contamination"
        )
        assert "NVIDIA" not in EXPAND_QUERY_WITH_HISTORY_TEMPLATE, (
            "Template contains 'NVIDIA' which causes few-shot example contamination"
        )
        assert "AMD Intel" not in EXPAND_QUERY_WITH_HISTORY_TEMPLATE, (
            "Template contains 'AMD Intel' which causes few-shot example contamination"
        )

    def test_expand_query_with_history_template_has_neutral_examples(self):
        """Template must use domain-neutral examples (weather + smartphone)."""
        assert "天気" in EXPAND_QUERY_WITH_HISTORY_TEMPLATE, (
            "Template should contain weather (天気) as a neutral example"
        )
        assert "スマートフォン" in EXPAND_QUERY_WITH_HISTORY_TEMPLATE, (
            "Template should contain smartphone (スマートフォン) as a neutral example"
        )

    def test_expand_query_with_history_template_has_anti_copy_rule(self):
        """Template must explicitly tell the model not to copy example topics."""
        assert "SAME TOPIC as the input" in EXPAND_QUERY_WITH_HISTORY_TEMPLATE, (
            "Template must contain anti-contamination instruction 'SAME TOPIC as the input'"
        )
