"""Tests for evaluation handler."""

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_evaluator.domain.models import (
    AlertLevel,
    EvaluationRun,
    EvaluationType,
    GenreEvaluationResult,
    PipelineMetrics,
    SummaryMetrics,
)
from recap_evaluator.handler.evaluation_handler import router


@pytest.fixture
def app():
    app = FastAPI()
    app.include_router(router)

    # Mock usecases on app.state
    app.state.run_evaluation = AsyncMock()
    app.state.get_metrics = AsyncMock()
    app.state.genre_evaluator = AsyncMock()
    app.state.cluster_evaluator = AsyncMock()
    app.state.summary_evaluator = AsyncMock()
    app.state.db = AsyncMock()

    return app


@pytest.fixture
def client(app):
    return TestClient(app)


class TestRunFullEvaluation:
    def test_returns_evaluation_run(self, client, app):
        eval_id = uuid4()
        app.state.run_evaluation.execute.return_value = EvaluationRun(
            evaluation_id=eval_id,
            evaluation_type=EvaluationType.FULL,
            job_ids=[uuid4()],
            created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
            window_days=7,
            overall_alert_level=AlertLevel.OK,
        )

        resp = client.post("/api/v1/evaluations/run", json={"window_days": 7})

        assert resp.status_code == 200
        data = resp.json()
        assert data["evaluation_type"] == "full"
        assert data["overall_alert_level"] == "ok"

    def test_returns_404_when_no_jobs(self, client, app):
        app.state.run_evaluation.execute.return_value = EvaluationRun(
            evaluation_id=uuid4(),
            evaluation_type=EvaluationType.FULL,
            job_ids=[],
            created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
            window_days=7,
        )

        resp = client.post("/api/v1/evaluations/run", json={"window_days": 7})

        assert resp.status_code == 404


class TestGetEvaluation:
    def test_returns_evaluation_by_id(self, client, app):
        eval_id = uuid4()
        app.state.get_metrics.get_evaluation_by_id.return_value = {
            "evaluation_id": eval_id,
            "evaluation_type": "full",
            "job_ids": [uuid4()],
            "created_at": datetime(2025, 1, 1, tzinfo=timezone.utc),
            "metrics": {"window_days": 7, "overall_alert_level": "ok"},
        }

        resp = client.get(f"/api/v1/evaluations/{eval_id}")

        assert resp.status_code == 200

    def test_returns_404_when_not_found(self, client, app):
        app.state.get_metrics.get_evaluation_by_id.return_value = None

        resp = client.get(f"/api/v1/evaluations/{uuid4()}")

        assert resp.status_code == 404


class TestListEvaluations:
    def test_returns_empty_list(self, client, app):
        app.state.get_metrics.get_evaluation_history.return_value = []

        resp = client.get("/api/v1/evaluations")

        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] == 0
        assert data["evaluations"] == []
