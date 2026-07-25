"""Tests for generate handler - HTTP 429 queue full behavior."""

from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, Mock

from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway.hybrid_priority_semaphore import QueueFullError
from news_creator.handler.generate_handler import create_generate_router
from news_creator.port.llm_provider_port import LLMProviderPort


def _make_client(mock_provider):
    app = FastAPI()
    app.include_router(create_generate_router(mock_provider))
    return TestClient(app)


def _make_mock_provider(return_value=None, side_effect=None):
    mock = Mock(spec=LLMProviderPort)
    if side_effect:
        mock.generate = AsyncMock(side_effect=side_effect)
    else:
        mock.generate = AsyncMock(
            return_value=return_value
            or LLMGenerateResponse(
                response="hello",
                model="test-model",
                done=True,
                done_reason="stop",
            )
        )
    return mock


def test_generate_returns_429_when_queue_full():
    """Queue saturation is expected backpressure; must surface as 429, not 500."""
    mock_provider = _make_mock_provider(
        side_effect=QueueFullError("Queue depth 10 >= max 10")
    )
    client = _make_client(mock_provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == 429
    data = response.json()
    assert "queue full" in data["error"]
    assert response.headers.get("Retry-After") == "30"


def test_generate_returns_200_on_success():
    mock_provider = _make_mock_provider()
    client = _make_client(mock_provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == 200
    assert response.json()["response"] == "hello"
