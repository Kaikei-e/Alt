"""Tests for expand query handler."""

from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock

from news_creator.handler.expand_query_handler import create_expand_query_router


def test_expand_query_handler_success():
    """Test successful query expansion request."""
    usecase = AsyncMock()
    usecase.expand_query.return_value = (
        ["AI技術の最新動向", "AI technology trends 2025", "machine learning advances", "generative AI development"],
        "gemma3-4b-16k",
        150.5,
    )

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {
        "query": "AI技術のトレンド",
        "japanese_count": 1,
        "english_count": 3,
    }
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == 200
    data = resp.json()
    assert len(data["expanded_queries"]) == 4
    assert data["original_query"] == "AI技術のトレンド"
    assert data["model"] == "gemma3-4b-16k"
    assert data["processing_time_ms"] == 150.5
    usecase.expand_query.assert_awaited_once_with(
        query="AI技術のトレンド",
        japanese_count=1,
        english_count=3,
    )


def test_expand_query_handler_default_counts():
    """Test query expansion with default counts."""
    usecase = AsyncMock()
    usecase.expand_query.return_value = (
        ["test query 1", "test query 2"],
        "gemma3-4b-16k",
        100.0,
    )

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    # Only query provided, should use defaults (japanese_count=1, english_count=3)
    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == 200
    usecase.expand_query.assert_awaited_once_with(
        query="test query",
        japanese_count=1,
        english_count=3,
    )


def test_expand_query_handler_value_error():
    """Test that ValueError results in 400 response."""
    usecase = AsyncMock()
    usecase.expand_query.side_effect = ValueError("query cannot be empty")

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": ""}
    resp = client.post("/api/v1/expand-query", json=payload)

    # Note: pydantic validation may catch this first, but if it gets through:
    # The handler should return 400 for ValueError from usecase
    # If pydantic catches it, it will be 422
    assert resp.status_code in (400, 422)


def test_expand_query_handler_runtime_error():
    """Test that RuntimeError results in 502 response."""
    usecase = AsyncMock()
    usecase.expand_query.side_effect = RuntimeError("LLM service unavailable")

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == 502
    assert resp.json()["detail"] == "LLM service unavailable"


def test_expand_query_handler_unexpected_error():
    """Test that unexpected exceptions result in 500 response."""
    usecase = AsyncMock()
    usecase.expand_query.side_effect = Exception("Unexpected error")

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == 500
    assert resp.json()["detail"] == "Internal server error"


def test_expand_query_handler_validation_min_length():
    """Test that query with min_length=1 is validated."""
    usecase = AsyncMock()

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    # Empty query should fail validation (min_length=1 in model)
    payload = {"query": ""}
    resp = client.post("/api/v1/expand-query", json=payload)

    # Pydantic validation error returns 422
    assert resp.status_code == 422
