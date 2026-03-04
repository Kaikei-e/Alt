"""Tests for health handler."""

from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_evaluator.handler.health_handler import router


@pytest.fixture
def app():
    app = FastAPI()
    app.include_router(router)

    mock_db = MagicMock()
    mock_pool = AsyncMock()
    mock_pool.fetchval.return_value = 1
    mock_db._pool = mock_pool
    app.state.db = mock_db

    return app


@pytest.fixture
def client(app):
    return TestClient(app)


class TestHealthCheck:
    def test_healthy_when_db_ok(self, client):
        resp = client.get("/health")

        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "healthy"
        assert data["service"] == "recap-evaluator"
        assert data["checks"]["db"] == "ok"

    def test_degraded_when_db_unavailable(self, client, app):
        app.state.db._pool.fetchval.side_effect = Exception("connection refused")

        resp = client.get("/health")

        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "degraded"
        assert data["checks"]["db"] == "unavailable"
