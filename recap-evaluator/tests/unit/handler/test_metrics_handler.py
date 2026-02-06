"""Tests for metrics handler."""

from datetime import datetime, timezone
from unittest.mock import AsyncMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_evaluator.handler.metrics_handler import router


@pytest.fixture
def app():
    app = FastAPI()
    app.include_router(router)
    app.state.get_metrics = AsyncMock()
    return app


@pytest.fixture
def client(app):
    return TestClient(app)


class TestGetLatestMetrics:
    def test_returns_metrics(self, client, app):
        app.state.get_metrics.get_latest.return_value = {
            "genre_macro_f1": 0.82,
            "genre_alert_level": "ok",
            "pipeline_success_rate": 0.95,
            "pipeline_alert_level": "ok",
            "cluster_avg_silhouette": 0.35,
            "cluster_alert_level": "ok",
            "last_evaluation_at": datetime(2025, 1, 1, tzinfo=timezone.utc),
        }

        resp = client.get("/api/v1/metrics/latest")

        assert resp.status_code == 200
        data = resp.json()
        assert data["genre_macro_f1"] == 0.82
        assert data["pipeline_success_rate"] == 0.95

    def test_returns_null_when_no_data(self, client, app):
        app.state.get_metrics.get_latest.return_value = {}

        resp = client.get("/api/v1/metrics/latest")

        assert resp.status_code == 200
        data = resp.json()
        assert data["genre_macro_f1"] is None


class TestGetMetricsTrends:
    def test_returns_empty_trends(self, client):
        resp = client.get("/api/v1/metrics/trends")

        assert resp.status_code == 200
        data = resp.json()
        assert data["trends"] == []
        assert data["window_days"] == 30
