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
