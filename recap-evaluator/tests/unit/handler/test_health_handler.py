"""Tests for health handler."""

from http import HTTPStatus
from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_evaluator.handler.health_handler import router


@pytest.fixture
def app() -> FastAPI:
    app = FastAPI()
    app.include_router(router)

    mock_db = MagicMock()
    mock_db.health_check = AsyncMock(return_value=True)
    app.state.db = mock_db

    return app


@pytest.fixture
def client(app: FastAPI) -> TestClient:
    return TestClient(app)


class TestHealthCheck:
    def test_healthy_when_db_ok(self, client: TestClient) -> None:
        resp = client.get("/health")

        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert data["status"] == "healthy"
        assert data["service"] == "recap-evaluator"
        assert data["checks"]["db"] == "ok"

    def test_degraded_when_db_unavailable(self, client: TestClient, app: FastAPI) -> None:
        app.state.db.health_check.side_effect = Exception("connection refused")

        resp = client.get("/health")

        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert data["status"] == "degraded"
        assert data["checks"]["db"] == "unavailable"
