"""Tests for PlanQueryUsecase."""

import json
import pytest
from unittest.mock import AsyncMock, Mock

from news_creator.domain.models import (
    ConversationMessage,
    LLMGenerateResponse,
    PlanQueryRequest,
    QueryPlan,
)
from news_creator.usecase.plan_query_usecase import PlanQueryUsecase


def _make_llm_response(plan: dict) -> LLMGenerateResponse:
    """Helper to create an LLM response with a QueryPlan JSON."""
    return LLMGenerateResponse(
        response=json.dumps(plan, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=256,
        eval_count=64,
        total_duration=500_000_000,
    )


def _make_usecase() -> tuple[PlanQueryUsecase, AsyncMock]:
    config = Mock()
    llm_provider = AsyncMock()
    usecase = PlanQueryUsecase(config=config, llm_provider=llm_provider)
    return usecase, llm_provider


# --- Single-turn queries ---


@pytest.mark.asyncio
async def test_plan_query_causal_single_turn():
    """Causal query produces resolved_query and causal intent."""
    usecase, llm = _make_usecase()
    llm.generate.return_value = _make_llm_response({
        "reasoning": "test reasoning", "resolved_query": "イランの石油危機が発生した背景と直接的原因",
        "search_queries": [
            "イラン 石油危機 原因 2026",
            "Iran oil crisis causes sanctions",
            "原油供給 イラン 制裁 影響",
        ],
        "intent": "causal_explanation",
        "retrieval_policy": "global_only",
        "answer_format": "causal_analysis",
        "should_clarify": False,
        "topic_entities": ["イラン", "石油", "制裁"],
    })

    request = PlanQueryRequest(query="イランの石油危機はなぜ起きた？")
    response = await usecase.plan_query(request)

    assert response.plan.intent == "causal_explanation"
    assert response.plan.retrieval_policy == "global_only"
    assert not response.plan.should_clarify
    assert len(response.plan.search_queries) >= 2
    assert response.original_query == "イランの石油危機はなぜ起きた？"
    assert response.model == "gemma4-e4b-12k"


# --- Follow-up with coreference resolution ---


@pytest.mark.asyncio
async def test_plan_query_follow_up_resolves_coreference():
    """Follow-up with 'それ' should produce a standalone resolved_query."""
    usecase, llm = _make_usecase()
    llm.generate.return_value = _make_llm_response({
        "reasoning": "test reasoning", "resolved_query": "イランの最近の外交的・軍事的動向",
        "search_queries": [
            "イラン 動向 2026",
            "Iran recent developments",
            "イラン 外交 軍事",
        ],
        "intent": "temporal",
        "retrieval_policy": "global_only",
        "answer_format": "summary",
        "should_clarify": False,
        "topic_entities": ["イラン"],
    })

    request = PlanQueryRequest(
        query="では、それに関連するイランの動向は？",
        conversation_history=[
            ConversationMessage(role="user", content="最近の石油危機の真因は？"),
            ConversationMessage(
                role="assistant",
                content="石油危機は複数の産油国の減産方針と地政学的緊張が原因です。",
            ),
        ],
    )
    response = await usecase.plan_query(request)

    # resolved_query should NOT contain 'それ' — it should be self-contained
    assert "それ" not in response.plan.resolved_query
    assert "イラン" in response.plan.resolved_query
    assert not response.plan.should_clarify


# --- Ambiguous follow-up → clarification ---


@pytest.mark.asyncio
async def test_plan_query_ambiguous_requests_clarification():
    """Ambiguous follow-up 'もっと詳しく' should request clarification."""
    usecase, llm = _make_usecase()
    llm.generate.return_value = _make_llm_response({
        "reasoning": "test reasoning", "resolved_query": "",
        "search_queries": [],
        "intent": "general",
        "retrieval_policy": "no_retrieval",
        "answer_format": "detail",
        "should_clarify": True,
        "topic_entities": [],
    })

    request = PlanQueryRequest(
        query="もっと詳しく",
        conversation_history=[
            ConversationMessage(role="user", content="最近のAIチップ開発の動向は？"),
            ConversationMessage(
                role="assistant",
                content="NVIDIAのBlackwellアーキテクチャが発表されました。",
            ),
        ],
    )
    response = await usecase.plan_query(request)

    assert response.plan.should_clarify is True


# --- Article-scoped query ---


@pytest.mark.asyncio
async def test_plan_query_article_scoped():
    """Article-scoped query should set article_only policy."""
    usecase, llm = _make_usecase()
    llm.generate.return_value = _make_llm_response({
        "reasoning": "test reasoning", "resolved_query": "Transformerアーキテクチャのattention機構の技術的詳細",
        "search_queries": [
            "Transformer attention mechanism detail",
            "attention 機構 仕組み",
        ],
        "intent": "topic_deep_dive",
        "retrieval_policy": "article_only",
        "answer_format": "detail",
        "should_clarify": False,
        "topic_entities": ["Transformer", "attention"],
    })

    request = PlanQueryRequest(
        query="attention機構について詳しく",
        article_id="abc-123",
        article_title="Transformerアーキテクチャ解説",
    )
    response = await usecase.plan_query(request)

    assert response.plan.retrieval_policy == "article_only"
    assert response.plan.intent == "topic_deep_dive"


# --- LLM failure → deterministic fallback ---


@pytest.mark.asyncio
async def test_plan_query_llm_failure_returns_fallback():
    """LLM error should produce a safe deterministic fallback plan."""
    usecase, llm = _make_usecase()
    llm.generate.side_effect = RuntimeError("Ollama timeout")

    request = PlanQueryRequest(query="量子コンピュータの実用化")
    response = await usecase.plan_query(request)

    # Fallback: use original query as resolved_query, intent=general
    assert response.plan.resolved_query == "量子コンピュータの実用化"
    assert response.plan.intent == "general"
    assert response.plan.retrieval_policy == "global_only"
    assert not response.plan.should_clarify


# --- Invalid JSON from LLM → deterministic fallback ---


@pytest.mark.asyncio
async def test_plan_query_invalid_json_returns_fallback():
    """Malformed LLM response should produce a safe fallback plan."""
    usecase, llm = _make_usecase()
    llm.generate.return_value = LLMGenerateResponse(
        response="This is not valid JSON at all {broken",
        model="gemma4-e4b-12k",
    )

    request = PlanQueryRequest(query="半導体市場の動向")
    response = await usecase.plan_query(request)

    assert response.plan.resolved_query == "半導体市場の動向"
    assert response.plan.intent == "general"
    assert not response.plan.should_clarify


# --- Prompt includes conversation history ---


@pytest.mark.asyncio
async def test_plan_query_prompt_includes_history():
    """Verify the LLM prompt includes conversation history when provided."""
    usecase, llm = _make_usecase()
    llm.generate.return_value = _make_llm_response({
        "reasoning": "test reasoning", "resolved_query": "EV充電インフラの課題",
        "search_queries": ["EV charging infrastructure challenges"],
        "intent": "general",
        "retrieval_policy": "global_only",
        "answer_format": "summary",
        "should_clarify": False,
        "topic_entities": ["EV", "充電"],
    })

    request = PlanQueryRequest(
        query="具体的な課題は？",
        conversation_history=[
            ConversationMessage(role="user", content="EVの普及状況は？"),
            ConversationMessage(role="assistant", content="充電インフラの整備が鍵です。"),
        ],
    )
    await usecase.plan_query(request)

    # Check that the prompt sent to LLM contains the conversation context
    call_args = llm.generate.call_args
    prompt = call_args.args[0] if call_args.args else call_args.kwargs.get("prompt", "")
    assert "EVの普及状況は？" in prompt
    assert "充電インフラの整備が鍵" in prompt
