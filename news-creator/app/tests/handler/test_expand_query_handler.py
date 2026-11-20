"""Tests for expand query handler."""

from http import HTTPStatus
from unittest.mock import AsyncMock

from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.errors import QueueFullError
from news_creator.handler.expand_query_handler import create_expand_query_router


def test_expand_query_handler_success() -> None:
    """Test successful query expansion request."""
    expected_queries = [
        "AI技術の最新動向",
        "AI technology trends 2025",
        "machine learning advances",
        "generative AI development",
    ]
    expected_processing_time_ms = 150.5
    usecase = AsyncMock()
    usecase.expand_query.return_value = (
        expected_queries,
        "gemma4-e4b-12k",
        expected_processing_time_ms,
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

    assert resp.status_code == HTTPStatus.OK
    data = resp.json()
    assert len(data["expanded_queries"]) == len(expected_queries)
    assert data["original_query"] == "AI技術のトレンド"
    assert data["model"] == "gemma4-e4b-12k"
    assert data["processing_time_ms"] == expected_processing_time_ms
    usecase.expand_query.assert_awaited_once_with(
        query="AI技術のトレンド",
        japanese_count=1,
        english_count=3,
        conversation_history=None,
        priority="high",
    )


def test_expand_query_handler_default_counts() -> None:
    """Test query expansion with default counts."""
    usecase = AsyncMock()
    usecase.expand_query.return_value = (
        ["test query 1", "test query 2"],
        "gemma4-e4b-12k",
        100.0,
    )

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    # Only query provided, should use defaults (japanese_count=1, english_count=3)
    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.OK
    usecase.expand_query.assert_awaited_once_with(
        query="test query",
        japanese_count=1,
        english_count=3,
        conversation_history=None,
        priority="high",
    )


def test_expand_query_handler_with_conversation_history() -> None:
    """Test query expansion with conversation history for coreference resolution."""
    expected_queries = ["expanded query 1", "expanded query 2"]
    usecase = AsyncMock()
    usecase.expand_query.return_value = (
        expected_queries,
        "gemma4-e4b-12k",
        200.0,
    )

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    conversation_history = [
        {"role": "user", "content": "What is machine learning?"},
        {"role": "assistant", "content": "Machine learning is a subset of AI..."},
    ]
    payload = {
        "query": "tell me more about that",
        "japanese_count": 1,
        "english_count": 3,
        "conversation_history": conversation_history,
    }
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.OK
    data = resp.json()
    assert len(data["expanded_queries"]) == len(expected_queries)

    # Verify conversation_history was passed through
    call_kwargs = usecase.expand_query.call_args.kwargs
    assert call_kwargs["conversation_history"] is not None
    assert len(call_kwargs["conversation_history"]) == len(conversation_history)


def test_expand_query_handler_value_error() -> None:
    """ValueError from usecase returns 400 without leaking exception text."""
    usecase = AsyncMock()
    secret = (
        "query cannot be empty at /app/news_creator/usecase/expand_query_usecase.py:40"  # noqa: S105 - test fixture string simulating leaked internal detail
    )
    usecase.expand_query.side_effect = ValueError(secret)

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.BAD_REQUEST
    assert resp.json()["detail"] == "Invalid request"
    assert secret not in resp.text


def test_expand_query_handler_runtime_error() -> None:
    """RuntimeError from usecase returns 502 without leaking exception text."""
    usecase = AsyncMock()
    secret = "LLM service unavailable at http://ollama.internal:11434"  # noqa: S105 - test fixture string simulating leaked internal detail
    usecase.expand_query.side_effect = RuntimeError(secret)

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.BAD_GATEWAY
    assert resp.json()["detail"] == "Upstream service error"
    assert secret not in resp.text


def test_expand_query_handler_queue_full_returns_429() -> None:
    """Queue saturation is expected backpressure; must surface as 429, not 502/500."""
    usecase = AsyncMock()
    usecase.expand_query.side_effect = QueueFullError("Queue depth 10 >= max 10")

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.TOO_MANY_REQUESTS
    assert "queue full" in resp.json()["error"]
    assert resp.headers.get("Retry-After") == "30"


def test_expand_query_handler_unexpected_error() -> None:
    """Test that unexpected exceptions result in 500 response."""
    usecase = AsyncMock()
    usecase.expand_query.side_effect = Exception("Unexpected error")

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.INTERNAL_SERVER_ERROR
    assert resp.json()["detail"] == "Internal server error"


def test_expand_query_handler_validation_min_length() -> None:
    """Test that query with min_length=1 is validated."""
    usecase = AsyncMock()

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    # Empty query should fail validation (min_length=1 in model)
    payload = {"query": ""}
    resp = client.post("/api/v1/expand-query", json=payload)

    # Pydantic validation error returns 422
    assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY


def test_expand_query_handler_with_priority() -> None:
    """Test that priority field is passed through to usecase."""
    usecase = AsyncMock()
    usecase.expand_query.return_value = (
        ["expanded query 1"],
        "gemma4-e4b-12k",
        100.0,
    )

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {
        "query": "test query",
        "priority": "high",
    }
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.OK
    call_kwargs = usecase.expand_query.call_args.kwargs
    assert call_kwargs["priority"] == "high"


def test_expand_query_handler_default_priority() -> None:
    """Test that priority defaults to high when not provided."""
    usecase = AsyncMock()
    usecase.expand_query.return_value = (
        ["expanded query 1"],
        "gemma4-e4b-12k",
        100.0,
    )

    app = FastAPI()
    app.include_router(create_expand_query_router(usecase))
    client = TestClient(app)

    payload = {"query": "test query"}
    resp = client.post("/api/v1/expand-query", json=payload)

    assert resp.status_code == HTTPStatus.OK
    call_kwargs = usecase.expand_query.call_args.kwargs
    assert call_kwargs["priority"] == "high"
