"""Tests for summarize handler - HTTP 429 queue full behavior."""

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, Mock

from news_creator.gateway.hybrid_priority_semaphore import QueueFullError


def _make_client(mock_usecase):
    """Create a fresh test client with a fresh router (avoid module-level router reuse)."""
    from news_creator.handler.summarize_handler import create_summarize_router
    # Reload module to get a fresh router each time
    import importlib
    import news_creator.handler.summarize_handler as mod
    importlib.reload(mod)

    app = FastAPI()
    router = mod.create_summarize_router(mock_usecase)
    app.include_router(router)
    return TestClient(app)


def _make_mock_usecase(return_value=None, side_effect=None):
    """Create a mock SummarizeUsecase."""
    from news_creator.usecase.summarize_usecase import SummarizeUsecase
    mock = Mock(spec=SummarizeUsecase)
    if side_effect:
        mock.generate_summary = AsyncMock(side_effect=side_effect)
    else:
        mock.generate_summary = AsyncMock(return_value=return_value or (
            "テスト要約",
            {"model": "test-model", "prompt_tokens": 100, "completion_tokens": 50, "total_duration_ms": 1000.0},
        ))
    return mock


def test_summarize_returns_429_when_queue_full():
    """Test that summarize endpoint returns HTTP 429 when queue is full."""
    mock_usecase = _make_mock_usecase(
        side_effect=QueueFullError("Queue depth 20 >= max 20")
    )
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == 429
    data = response.json()
    assert "queue full" in data["error"]
    assert response.headers.get("Retry-After") == "30"


def test_summarize_returns_200_on_success():
    """Test that summarize endpoint returns 200 on success."""
    mock_usecase = _make_mock_usecase()
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == 200
    data = response.json()
    assert data["success"] is True
    assert data["summary"] == "テスト要約"
