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
    def test_returns_empty_trends(self, client, app):
        app.state.get_metrics.get_trends.return_value = []

        resp = client.get("/api/v1/metrics/trends")

        assert resp.status_code == 200
        data = resp.json()
        assert data["trends"] == []
        assert data["window_days"] == 30

    def test_returns_trends_from_history(self, client, app):
        app.state.get_metrics.get_trends.return_value = [
            {
                "metric_name": "genre_macro_f1",
                "data_points": [
                    {"timestamp": "2025-01-01T00:00:00+00:00", "value": 0.80},
                    {"timestamp": "2025-01-02T00:00:00+00:00", "value": 0.82},
                ],
                "current_value": 0.82,
                "change_7d": 0.025,
                "change_30d": None,
            }
        ]

        resp = client.get("/api/v1/metrics/trends?window_days=7")

        assert resp.status_code == 200
        data = resp.json()
        assert len(data["trends"]) == 1
        assert data["trends"][0]["metric_name"] == "genre_macro_f1"
        assert data["trends"][0]["current_value"] == 0.82
        assert len(data["trends"][0]["data_points"]) == 2
        assert data["window_days"] == 7
