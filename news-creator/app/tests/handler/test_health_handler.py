"""Tests for health check handler."""

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, Mock

from news_creator.handler.health_handler import create_health_router


@pytest.fixture
def mock_ollama_gateway():
    """Create a mock Ollama gateway."""
    mock = Mock()
    mock.list_models = AsyncMock()
    # Add semaphore mock with queue_status
    mock._semaphore = Mock()
    mock._semaphore.queue_status.return_value = {
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


def test_health_check_with_models_loaded(client, mock_ollama_gateway):
    """Test health check returns models when Ollama has models loaded."""
    # Arrange
    mock_ollama_gateway.list_models.return_value = [
        {"name": "gemma3:4b", "size": 1234567890},
        {"name": "llama2:7b", "size": 9876543210},
    ]

    # Act
    response = client.get("/health")

    # Assert
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["service"] == "news-creator"
    assert "models" in data
    assert len(data["models"]) == 2
    assert data["models"][0]["name"] == "gemma3:4b"
    assert data["models"][1]["name"] == "llama2:7b"


def test_health_check_with_no_models_loaded(client, mock_ollama_gateway):
    """Test health check returns empty models when Ollama has no models."""
    # Arrange
    mock_ollama_gateway.list_models.return_value = []

    # Act
    response = client.get("/health")

    # Assert
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["service"] == "news-creator"
    assert "models" in data
    assert len(data["models"]) == 0


def test_health_check_handles_ollama_unavailable(client, mock_ollama_gateway):
    """Test health check handles Ollama service unavailability gracefully."""
    # Arrange
    mock_ollama_gateway.list_models.side_effect = RuntimeError("Ollama service unavailable")

    # Act
    response = client.get("/health")

    # Assert
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["service"] == "news-creator"
    assert "models" in data
    assert len(data["models"]) == 0
    assert "error" in data
    assert "Ollama service unavailable" in data["error"]


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
    mock_ollama_gateway._semaphore.queue_status.return_value = {
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


def test_health_check_handles_network_timeout(client, mock_ollama_gateway):
    """Test health check handles network timeout gracefully."""
    # Arrange
    import asyncio
    mock_ollama_gateway.list_models.side_effect = asyncio.TimeoutError()

    # Act
    response = client.get("/health")

    # Assert
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["service"] == "news-creator"
    assert "models" in data
    assert len(data["models"]) == 0
    assert "error" in data
