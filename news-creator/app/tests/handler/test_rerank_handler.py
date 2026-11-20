"""Tests for rerank handler."""

from http import HTTPStatus
from unittest.mock import AsyncMock, Mock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

import news_creator.handler.rerank_handler as rerank_handler_mod
from news_creator.handler.rerank_handler import create_rerank_router
from news_creator.usecase.rerank_usecase import RerankUsecase


class TestRerankHandler:
    """Test suite for rerank handler."""

    @pytest.fixture
    def mock_usecase(self) -> AsyncMock:
        """Create a mock RerankUsecase."""
        mock = AsyncMock(spec=RerankUsecase)
        mock.model_name = "test-model"
        mock.rerank.return_value = (
            [(1, 0.95), (0, 0.85), (2, 0.75)],
            "test-model",
            123.45,
        )
        return mock

    @pytest.fixture
    def client(self, mock_usecase: AsyncMock) -> TestClient:
        """Create test client with mock usecase."""
        app = FastAPI()
        app.include_router(create_rerank_router(mock_usecase))
        return TestClient(app)

    def test_rerank_success(self, client: TestClient, mock_usecase: AsyncMock) -> None:
        """Test successful rerank request."""
        expected_results = [(1, 0.95), (0, 0.85), (2, 0.75)]
        expected_model = "test-model"
        expected_time_ms = 123.45
        mock_usecase.rerank.return_value = (
            expected_results,
            expected_model,
            expected_time_ms,
        )

        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1", "candidate 2", "candidate 3"],
            },
        )

        assert response.status_code == HTTPStatus.OK
        data = response.json()

        assert len(data["results"]) == len(expected_results)
        for i, (exp_idx, exp_score) in enumerate(expected_results):
            assert data["results"][i]["index"] == exp_idx
            assert data["results"][i]["score"] == exp_score
        assert data["model"] == expected_model
        assert data["processing_time_ms"] == expected_time_ms

        mock_usecase.rerank.assert_called_once_with(
            query="test query",
            candidates=["candidate 1", "candidate 2", "candidate 3"],
            top_k=None,
        )

    def test_rerank_with_top_k(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """Test rerank request with top_k limit."""
        top_k = 2
        mock_usecase.rerank.return_value = ([(1, 0.95), (0, 0.85)], "test-model", 100.0)

        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1", "candidate 2", "candidate 3"],
                "top_k": top_k,
            },
        )

        assert response.status_code == HTTPStatus.OK
        data = response.json()

        assert len(data["results"]) == top_k

        mock_usecase.rerank.assert_called_once_with(
            query="test query",
            candidates=["candidate 1", "candidate 2", "candidate 3"],
            top_k=top_k,
        )

    def test_rerank_empty_query(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """Test rerank request with empty query returns 400."""
        response = client.post(
            "/v1/rerank", json={"query": "", "candidates": ["candidate 1"]}
        )

        # FastAPI validation should catch empty query
        assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY

    def test_rerank_empty_candidates(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """Test rerank request with empty candidates returns 422."""
        response = client.post(
            "/v1/rerank", json={"query": "test query", "candidates": []}
        )

        assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY

    def test_rerank_usecase_value_error(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """ValueError from usecase returns 400 without leaking exception text."""
        secret = "Invalid input at /app/news_creator/usecase/rerank_usecase.py:12"  # noqa: S105 - test fixture string simulating leaked internal detail
        mock_usecase.rerank.side_effect = ValueError(secret)

        response = client.post(
            "/v1/rerank", json={"query": "test query", "candidates": ["candidate 1"]}
        )

        assert response.status_code == HTTPStatus.BAD_REQUEST
        assert response.json()["detail"] == "Invalid request"
        assert secret not in response.text

    def test_rerank_usecase_runtime_error(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """RuntimeError from usecase returns 502 without leaking exception text."""
        secret = "Model failed at http://ollama.internal:11434"  # noqa: S105 - test fixture string simulating leaked internal detail
        mock_usecase.rerank.side_effect = RuntimeError(secret)

        response = client.post(
            "/v1/rerank", json={"query": "test query", "candidates": ["candidate 1"]}
        )

        assert response.status_code == HTTPStatus.BAD_GATEWAY
        assert response.json()["detail"] == "Upstream service error"
        assert secret not in response.text

    def test_rerank_unexpected_error(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """Test rerank request when usecase raises unexpected error."""
        mock_usecase.rerank.side_effect = Exception("Unexpected error")

        response = client.post(
            "/v1/rerank", json={"query": "test query", "candidates": ["candidate 1"]}
        )

        assert response.status_code == HTTPStatus.INTERNAL_SERVER_ERROR
        assert "Internal server error" in response.json()["detail"]

    def test_rerank_with_allowed_default_model(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """G7-02: Rerank request with the allowlisted default model succeeds."""
        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1", "candidate 2", "candidate 3"],
                "model": "BAAI/bge-reranker-v2-m3",
            },
        )

        assert response.status_code == HTTPStatus.OK
        mock_usecase.rerank.assert_called_once_with(
            query="test query",
            candidates=["candidate 1", "candidate 2", "candidate 3"],
            top_k=None,
        )

    def test_rerank_rejects_unallowlisted_custom_model(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """G7-02: Arbitrary model names must be rejected with 422 to prevent Hub SSRF / singleton clobbering."""
        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1"],
                "model": "custom-model",
            },
        )

        assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY
        mock_usecase.rerank.assert_not_called()

    def test_rerank_rejects_oversized_query(
        self, client: TestClient, mock_usecase: AsyncMock
    ) -> None:
        """G7-02: Query exceeding 4096 chars must be rejected with 422."""
        response = client.post(
            "/v1/rerank",
            json={
                "query": "q" * 4097,
                "candidates": ["candidate 1"],
            },
        )

        assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY
        mock_usecase.rerank.assert_not_called()

    def test_rerank_unsupported_model_clears_context(
        self,
        client: TestClient,
        mock_usecase: AsyncMock,
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        """Unsupported model check inside handler must still clear logging context via finally."""
        clear_spy = Mock()
        monkeypatch.setattr(rerank_handler_mod, "clear_context", clear_spy)
        monkeypatch.setattr(RerankUsecase, "ALLOWED_MODELS", frozenset())

        response = client.post(
            "/v1/rerank",
            json={
                "query": "test query",
                "candidates": ["candidate 1"],
                "model": "BAAI/bge-reranker-v2-m3",
            },
        )

        assert response.status_code == HTTPStatus.BAD_REQUEST
        mock_usecase.rerank.assert_not_called()
        clear_spy.assert_called_once()
