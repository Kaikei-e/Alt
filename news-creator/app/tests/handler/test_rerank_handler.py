"""Tests for rerank handler."""

import pytest
from unittest.mock import AsyncMock, patch
from fastapi.testclient import TestClient
from fastapi import FastAPI

from news_creator.handler.rerank_handler import create_rerank_router
from news_creator.usecase.rerank_usecase import RerankUsecase


class TestRerankHandler:
    """Test suite for rerank handler."""

    @pytest.fixture
    def mock_usecase(self):
        """Create a mock RerankUsecase."""
        mock = AsyncMock(spec=RerankUsecase)
        mock.model_name = "test-model"
        mock.rerank.return_value = (
            [(1, 0.95), (0, 0.85), (2, 0.75)],
            "test-model",
            123.45
        )
        return mock

    @pytest.fixture
    def client(self, mock_usecase):
        """Create test client with mock usecase."""
        app = FastAPI()
        app.include_router(create_rerank_router(mock_usecase))
        return TestClient(app)

    def test_rerank_success(self, client, mock_usecase):
        """Test successful rerank request."""
        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1", "candidate 2", "candidate 3"]
            }
        )

        assert response.status_code == 200
        data = response.json()

        assert len(data["results"]) == 3
        assert data["results"][0]["index"] == 1
        assert data["results"][0]["score"] == 0.95
        assert data["results"][1]["index"] == 0
        assert data["results"][1]["score"] == 0.85
        assert data["results"][2]["index"] == 2
        assert data["results"][2]["score"] == 0.75
        assert data["model"] == "test-model"
        assert data["processing_time_ms"] == 123.45

        mock_usecase.rerank.assert_called_once_with(
            query="test query",
            candidates=["candidate 1", "candidate 2", "candidate 3"],
            top_k=None
        )

    def test_rerank_with_top_k(self, client, mock_usecase):
        """Test rerank request with top_k limit."""
        mock_usecase.rerank.return_value = (
            [(1, 0.95), (0, 0.85)],
            "test-model",
            100.0
        )

        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1", "candidate 2", "candidate 3"],
                "top_k": 2
            }
        )

        assert response.status_code == 200
        data = response.json()

        assert len(data["results"]) == 2

        mock_usecase.rerank.assert_called_once_with(
            query="test query",
            candidates=["candidate 1", "candidate 2", "candidate 3"],
            top_k=2
        )

    def test_rerank_empty_query(self, client, mock_usecase):
        """Test rerank request with empty query returns 400."""
        response = client.post(
            "/v1/rerank",
            json={
                "query": "",
                "candidates": ["candidate 1"]
            }
        )

        # FastAPI validation should catch empty query
        assert response.status_code == 422

    def test_rerank_empty_candidates(self, client, mock_usecase):
        """Test rerank request with empty candidates returns 422."""
        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": []
            }
        )

        assert response.status_code == 422

    def test_rerank_usecase_value_error(self, client, mock_usecase):
        """Test rerank request when usecase raises ValueError."""
        mock_usecase.rerank.side_effect = ValueError("Invalid input")

        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1"]
            }
        )

        assert response.status_code == 400
        assert "Invalid input" in response.json()["detail"]

    def test_rerank_usecase_runtime_error(self, client, mock_usecase):
        """Test rerank request when usecase raises RuntimeError."""
        mock_usecase.rerank.side_effect = RuntimeError("Model failed")

        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1"]
            }
        )

        assert response.status_code == 502
        assert "Model failed" in response.json()["detail"]

    def test_rerank_unexpected_error(self, client, mock_usecase):
        """Test rerank request when usecase raises unexpected error."""
        mock_usecase.rerank.side_effect = Exception("Unexpected error")

        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1"]
            }
        )

        assert response.status_code == 500
        assert "Internal server error" in response.json()["detail"]

    def test_rerank_with_custom_model(self, client, mock_usecase):
        """Test rerank request with custom model creates new usecase."""
        # When custom model is specified, a new usecase is created
        with patch("news_creator.handler.rerank_handler.RerankUsecase") as MockUsecase:
            mock_instance = AsyncMock()
            mock_instance.rerank.return_value = ([(0, 0.9)], "custom-model", 50.0)
            MockUsecase.return_value = mock_instance

            response = client.post(
                "/v1/rerank",
                json={
                    "query": "test query",
                    "candidates": ["candidate 1"],
                    "model": "custom-model"
                }
            )

            assert response.status_code == 200
            MockUsecase.assert_called_once_with(model_name="custom-model")
