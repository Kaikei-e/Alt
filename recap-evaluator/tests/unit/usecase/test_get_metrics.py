"""Tests for GetMetricsUsecase."""

from datetime import datetime, timezone
from unittest.mock import AsyncMock
from uuid import uuid4

import pytest

from recap_evaluator.domain.models import (
    AlertLevel,
    ClusterMetrics,
    GenreEvaluationResult,
    PipelineMetrics,
)
from recap_evaluator.usecase.get_metrics import GetMetricsUsecase
from tests.fixtures.job_data import SAMPLE_JOB


@pytest.fixture
def mock_genre_eval():
    m = AsyncMock()
    m.fetch_latest_evaluation.return_value = GenreEvaluationResult(
        macro_f1=0.82, alert_level=AlertLevel.OK
    )
    return m


@pytest.fixture
def mock_cluster_eval():
    m = AsyncMock()
    m.evaluate_batch.return_value = {
        "technology": ClusterMetrics(silhouette_score=0.35)
    }
    return m


@pytest.fixture
def mock_pipeline_eval():
    m = AsyncMock()
    m.evaluate_batch.return_value = PipelineMetrics(
        success_rate=0.95, alert_level=AlertLevel.OK
    )
    return m


@pytest.fixture
def get_metrics_uc(mock_genre_eval, mock_cluster_eval, mock_pipeline_eval, mock_db):
    return GetMetricsUsecase(
        genre_evaluator=mock_genre_eval,
        cluster_evaluator=mock_cluster_eval,
        pipeline_evaluator=mock_pipeline_eval,
        db=mock_db,
    )


class TestGetMetricsUsecase:
    async def test_get_latest_returns_all_dimensions(
        self, get_metrics_uc, mock_db
    ):
        mock_db.fetch_recent_jobs.return_value = [SAMPLE_JOB]

        result = await get_metrics_uc.get_latest()

        assert "genre_macro_f1" in result
        assert result["genre_macro_f1"] == 0.82
        assert "pipeline_success_rate" in result
        assert "cluster_avg_silhouette" in result

    async def test_get_latest_no_jobs(self, get_metrics_uc, mock_db):
        mock_db.fetch_recent_jobs.return_value = []
        # Also mock genre to return None to test empty path
        get_metrics_uc._genre.fetch_latest_evaluation.return_value = None

        result = await get_metrics_uc.get_latest()

        assert "pipeline_success_rate" not in result

    async def test_get_evaluation_by_id(self, get_metrics_uc, mock_db):
        eval_id = uuid4()
        mock_db.fetch_evaluation_by_id.return_value = {
            "evaluation_id": eval_id,
            "evaluation_type": "full",
        }

        result = await get_metrics_uc.get_evaluation_by_id(eval_id)

        assert result is not None
        assert result["evaluation_id"] == eval_id

    async def test_get_evaluation_history(self, get_metrics_uc, mock_db):
        mock_db.fetch_evaluation_history.return_value = [
            {"evaluation_id": uuid4(), "evaluation_type": "full"}
        ]

        result = await get_metrics_uc.get_evaluation_history(limit=10)

        assert len(result) == 1
        mock_db.fetch_evaluation_history.assert_called_once_with(
            evaluation_type=None, limit=10
        )

    async def test_get_trends_returns_metrics_from_history(
        self, get_metrics_uc, mock_db
    ):
        ts1 = datetime(2025, 1, 1, tzinfo=timezone.utc)
        ts2 = datetime(2025, 1, 2, tzinfo=timezone.utc)
        mock_db.fetch_evaluation_history.return_value = [
            {
                "created_at": ts2,
                "metrics": {
                    "genre": {"macro_f1": 0.85},
                    "pipeline": {"success_rate": 0.96},
                    "summary": {"overall_quality_score": 0.72},
                },
            },
            {
                "created_at": ts1,
                "metrics": {
                    "genre": {"macro_f1": 0.80},
                    "pipeline": {"success_rate": 0.94},
                    "summary": {"overall_quality_score": 0.70},
                },
            },
        ]

        trends = await get_metrics_uc.get_trends(window_days=7)

        assert len(trends) >= 2
        metric_names = {t["metric_name"] for t in trends}
        assert "genre_macro_f1" in metric_names
        assert "pipeline_success_rate" in metric_names

        genre_trend = next(t for t in trends if t["metric_name"] == "genre_macro_f1")
        assert genre_trend["current_value"] == 0.85
        assert len(genre_trend["data_points"]) == 2

    async def test_get_trends_empty_history(self, get_metrics_uc, mock_db):
        mock_db.fetch_evaluation_history.return_value = []

        trends = await get_metrics_uc.get_trends(window_days=30)

        assert trends == []
