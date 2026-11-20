"""Tests for metrics handler."""

from datetime import UTC, datetime
from http import HTTPStatus
from unittest.mock import AsyncMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_evaluator.handler.metrics_handler import router

EXPECTED_MACRO_F1 = 0.82
EXPECTED_SUCCESS_RATE = 0.95
DEFAULT_WINDOW_DAYS = 30
EXPECTED_TREND_POINTS = 2
CUSTOM_WINDOW_DAYS = 7


@pytest.fixture
def app() -> FastAPI:
    app = FastAPI()
    app.include_router(router)
    app.state.get_metrics = AsyncMock()
    return app


@pytest.fixture
def client(app: FastAPI) -> TestClient:
    return TestClient(app)


class TestGetLatestMetrics:
    def test_returns_metrics(self, client: TestClient, app: FastAPI) -> None:
        app.state.get_metrics.get_latest.return_value = {
            "genre_macro_f1": 0.82,
            "genre_alert_level": "ok",
            "pipeline_success_rate": 0.95,
            "pipeline_alert_level": "ok",
            "cluster_avg_silhouette": 0.35,
            "cluster_alert_level": "ok",
            "last_evaluation_at": datetime(2025, 1, 1, tzinfo=UTC),
        }

        resp = client.get("/api/v1/metrics/latest")

        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert data["genre_macro_f1"] == EXPECTED_MACRO_F1
        assert data["pipeline_success_rate"] == EXPECTED_SUCCESS_RATE

    def test_returns_null_when_no_data(self, client: TestClient, app: FastAPI) -> None:
        app.state.get_metrics.get_latest.return_value = {}

        resp = client.get("/api/v1/metrics/latest")

        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert data["genre_macro_f1"] is None


class TestGetMetricsTrends:
    def test_returns_empty_trends(self, client: TestClient, app: FastAPI) -> None:
        app.state.get_metrics.get_trends.return_value = []

        resp = client.get("/api/v1/metrics/trends")

        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert data["trends"] == []
        assert data["window_days"] == DEFAULT_WINDOW_DAYS

    def test_returns_trends_from_history(self, client: TestClient, app: FastAPI) -> None:
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

        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert len(data["trends"]) == 1
        assert data["trends"][0]["metric_name"] == "genre_macro_f1"
        assert data["trends"][0]["current_value"] == EXPECTED_MACRO_F1
        assert len(data["trends"][0]["data_points"]) == EXPECTED_TREND_POINTS
        assert data["window_days"] == CUSTOM_WINDOW_DAYS

    @pytest.mark.parametrize("invalid_days", [0, -1, 91, 1000])
    def test_rejects_out_of_range_window_days(self, client: TestClient, invalid_days: int) -> None:
        resp = client.get(f"/api/v1/metrics/trends?window_days={invalid_days}")
        assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY
