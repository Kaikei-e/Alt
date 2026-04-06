"""Tests for plan_query_handler."""

import pytest
from unittest.mock import AsyncMock
from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.models import PlanQueryResponse, QueryPlan
from news_creator.handler.plan_query_handler import create_plan_query_router


def _make_app(mock_usecase: AsyncMock) -> TestClient:
    app = FastAPI()
    app.include_router(create_plan_query_router(mock_usecase))
    return TestClient(app)


def _successful_response() -> PlanQueryResponse:
    return PlanQueryResponse(
        plan=QueryPlan(
            reasoning="test reasoning",
            resolved_query="テスト検索クエリ",
            search_queries=["テスト 検索", "test search query"],
            intent="general",
            retrieval_policy="global_only",
            answer_format="summary",
            should_clarify=False,
            topic_entities=["テスト"],
        ),
        original_query="テストクエリ",
        model="gemma4-e4b-12k",
        processing_time_ms=150.0,
    )


def test_plan_query_success():
    """POST /api/v1/plan-query returns 200 with valid PlanQueryResponse."""
    mock_usecase = AsyncMock()
    mock_usecase.plan_query.return_value = _successful_response()
    client = _make_app(mock_usecase)

    resp = client.post("/api/v1/plan-query", json={"query": "テストクエリ"})

    assert resp.status_code == 200
    body = resp.json()
    assert body["plan"]["intent"] == "general"
    assert body["plan"]["resolved_query"] == "テスト検索クエリ"
    assert body["original_query"] == "テストクエリ"
    assert body["model"] == "gemma4-e4b-12k"


def test_plan_query_with_history():
    """POST /api/v1/plan-query with conversation history passes it to usecase."""
    mock_usecase = AsyncMock()
    mock_usecase.plan_query.return_value = _successful_response()
    client = _make_app(mock_usecase)

    resp = client.post("/api/v1/plan-query", json={
        "query": "それについて詳しく",
        "conversation_history": [
            {"role": "user", "content": "AIの最新動向は？"},
            {"role": "assistant", "content": "LLMの進化が著しいです。"},
        ],
    })

    assert resp.status_code == 200
    call_args = mock_usecase.plan_query.call_args[0][0]
    assert len(call_args.conversation_history) == 2


def test_plan_query_empty_query_returns_422():
    """POST /api/v1/plan-query with empty query returns 422 (Pydantic validation)."""
    mock_usecase = AsyncMock()
    client = _make_app(mock_usecase)

    resp = client.post("/api/v1/plan-query", json={"query": ""})

    assert resp.status_code == 422


def test_plan_query_llm_error_returns_502():
    """POST /api/v1/plan-query returns 502 when LLM fails with RuntimeError."""
    mock_usecase = AsyncMock()
    mock_usecase.plan_query.side_effect = RuntimeError("Ollama connection refused")
    client = _make_app(mock_usecase)

    resp = client.post("/api/v1/plan-query", json={"query": "テスト"})

    assert resp.status_code == 502


def test_plan_query_unexpected_error_returns_500():
    """POST /api/v1/plan-query returns 500 on unexpected errors."""
    mock_usecase = AsyncMock()
    mock_usecase.plan_query.side_effect = Exception("unexpected")
    client = _make_app(mock_usecase)

    resp = client.post("/api/v1/plan-query", json={"query": "テスト"})

    assert resp.status_code == 500
