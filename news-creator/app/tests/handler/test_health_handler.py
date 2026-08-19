"""Cheap /health must not fan out; /health/deep owns Ollama reachability."""

from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, Mock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from httpx import ASGITransport, AsyncClient

from news_creator.handler.health_handler import create_health_router


@pytest.fixture
def mock_ollama_gateway():
    """Create a mock Ollama gateway (LLMProviderPort)."""
    mock = Mock()
    mock.list_models = AsyncMock()
    mock.queue_status.return_value = {
        "rt_queue": 0,
        "be_queue": 0,
        "total_slots": 2,
        "available_slots": 2,
        "accepting": True,
        "max_queue_depth": 20,
    }
    return mock


@pytest.fixture
def client(mock_ollama_gateway):
    """Create a test client with the health router."""
    app = FastAPI()
    app.include_router(create_health_router(mock_ollama_gateway))
    return TestClient(app)


def test_cheap_health_returns_constant_liveness(client, mock_ollama_gateway):
    mock_ollama_gateway.list_models.side_effect = AssertionError(
        "list_models must not run"
    )

    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "healthy", "service": "news-creator"}
    mock_ollama_gateway.list_models.assert_not_called()


def test_cheap_health_never_calls_list_models_even_when_models_exist(
    client, mock_ollama_gateway
):
    mock_ollama_gateway.list_models.return_value = [
        {"name": "gemma4-e4b-q4km", "size": 1234567890},
    ]

    response = client.get("/health")

    assert response.status_code == 200
    assert "models" not in response.json()
    assert "error" not in response.json()
    mock_ollama_gateway.list_models.assert_not_called()


@pytest.mark.asyncio
async def test_cheap_health_is_timing_independent_of_hung_upstream(
    mock_ollama_gateway,
) -> None:
    async def hang() -> list[object]:
        await asyncio.Event().wait()
        return []

    mock_ollama_gateway.list_models.side_effect = hang
    app = FastAPI()
    app.include_router(create_health_router(mock_ollama_gateway))
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        try:
            response = await asyncio.wait_for(ac.get("/health"), timeout=0.4)
        except TimeoutError:
            pytest.fail("cheap /health waited on upstream I/O")
    assert response.status_code == 200
    assert response.json() == {"status": "healthy", "service": "news-creator"}
    mock_ollama_gateway.list_models.assert_not_called()


def test_queue_status_returns_correct_state(client, mock_ollama_gateway):
    """Test that /queue/status returns correct queue state."""
    response = client.get("/queue/status")

    assert response.status_code == 200
    data = response.json()
    assert data["rt_queue"] == 0
    assert data["be_queue"] == 0
    assert data["total_slots"] == 2
    assert data["available_slots"] == 2
    assert data["accepting"] is True
    assert data["max_queue_depth"] == 20


def test_queue_status_with_saturated_queue(client, mock_ollama_gateway):
    """Test queue status when queue is saturated."""
    mock_ollama_gateway.queue_status.return_value = {
        "rt_queue": 10,
        "be_queue": 10,
        "total_slots": 2,
        "available_slots": 0,
        "accepting": False,
        "max_queue_depth": 20,
    }

    response = client.get("/queue/status")
    assert response.status_code == 200
    data = response.json()
    assert data["accepting"] is False
    assert data["rt_queue"] == 10
    assert data["be_queue"] == 10


def test_deep_health_fails_when_ollama_unavailable(client, mock_ollama_gateway):
    """Deep health must not silent-200 a critical Ollama outage."""
    secret = "Ollama service unavailable at http://internal:11434/secret-path"
    mock_ollama_gateway.list_models.side_effect = RuntimeError(secret)

    response = client.get("/health/deep")

    assert response.status_code == 503
    data = response.json()
    assert data["status"] == "fail"
    assert data["service"] == "news-creator"
    assert data["checks"][0]["name"] == "ollama"
    assert data["checks"][0]["critical"] is True
    assert data["checks"][0]["status"] == "fail"
    assert secret not in response.text
    assert "11434" not in response.text
    assert "Traceback" not in response.text


def test_deep_health_fails_when_no_models(client, mock_ollama_gateway):
    mock_ollama_gateway.list_models.return_value = []

    response = client.get("/health/deep")

    assert response.status_code == 503
    data = response.json()
    assert data["status"] == "fail"
    assert data["checks"][0]["name"] == "ollama"


def test_deep_health_passes_when_ollama_has_models(client, mock_ollama_gateway):
    mock_ollama_gateway.list_models.return_value = [{"name": "gemma4-e4b-q4km"}]

    response = client.get("/health/deep")

    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "pass"
    assert data["checks"][0]["status"] == "pass"


def test_cheap_health_stays_200_when_deep_fails(client, mock_ollama_gateway):
    mock_ollama_gateway.list_models.side_effect = RuntimeError("ollama down")

    cheap = client.get("/health")
    deep = client.get("/health/deep")

    assert cheap.status_code == 200
    assert cheap.json()["status"] == "healthy"
    mock_ollama_gateway.list_models.assert_called()
    assert deep.status_code == 503
    assert deep.json()["status"] == "fail"


@pytest.mark.asyncio
async def test_deep_health_singleflight_under_concurrent_requests(
    mock_ollama_gateway,
) -> None:
    started = asyncio.Event()
    release = asyncio.Event()
    calls = 0

    async def slow_list() -> list[dict[str, str]]:
        nonlocal calls
        calls += 1
        started.set()
        await release.wait()
        return [{"name": "gemma4-e4b-q4km"}]

    mock_ollama_gateway.list_models.side_effect = slow_list
    app = FastAPI()
    app.include_router(create_health_router(mock_ollama_gateway))
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        first = asyncio.create_task(ac.get("/health/deep"))
        await started.wait()
        rest = [asyncio.create_task(ac.get("/health/deep")) for _ in range(7)]
        release.set()
        responses = [await first, *(await asyncio.gather(*rest))]
        cached = await ac.get("/health/deep")
    assert calls == 1
    assert all(r.status_code == 200 for r in responses)
    assert all(r.json()["status"] == "pass" for r in responses)
    assert cached.status_code == 200
    assert cached.json()["cached"] is True
