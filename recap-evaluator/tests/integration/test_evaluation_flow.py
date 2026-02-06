"""Integration tests for full evaluation flow: handler → usecase → mock gateway."""

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_evaluator.config import AlertThresholds, EvaluatorWeights, Settings
from recap_evaluator.domain.models import AlertLevel
from recap_evaluator.evaluator.cluster_evaluator import ClusterEvaluator
from recap_evaluator.evaluator.genre_evaluator import GenreEvaluator
from recap_evaluator.evaluator.pipeline_evaluator import PipelineEvaluator
from recap_evaluator.evaluator.summary_evaluator import SummaryEvaluator
from recap_evaluator.handler.evaluation_handler import router as evaluation_router
from recap_evaluator.handler.health_handler import router as health_router
from recap_evaluator.handler.metrics_handler import router as metrics_router
from recap_evaluator.usecase.get_metrics import GetMetricsUsecase
from recap_evaluator.usecase.run_evaluation import RunEvaluationUsecase
from tests.fixtures.evaluation_data import SAMPLE_GENRE_API_RESPONSE
from tests.fixtures.job_data import (
    SAMPLE_ARTICLE,
    SAMPLE_CLUSTER,
    SAMPLE_JOB,
    SAMPLE_JOB_ID,
    SAMPLE_OUTPUT,
    SAMPLE_PREPROCESS_METRICS,
    SAMPLE_STAGE_LOGS,
    SAMPLE_SUBWORKER_RUN,
)


@pytest.fixture
def integration_app():
    """Build full app with mock gateways but real usecases/evaluators."""
    settings = Settings(
        recap_db_dsn="postgres://test:test@localhost/test",
        ollama_concurrency=2,
        geval_sample_size=5,
        evaluation_thread_pool_size=2,
        enable_scheduler=False,
    )
    thresholds = AlertThresholds()
    weights = EvaluatorWeights()

    # Mock gateways
    mock_db = AsyncMock()
    mock_db.fetch_recent_jobs.return_value = [SAMPLE_JOB]
    mock_db.fetch_job_articles.return_value = [SAMPLE_ARTICLE]
    mock_db.fetch_outputs.return_value = [SAMPLE_OUTPUT]
    mock_db.fetch_stage_logs.return_value = SAMPLE_STAGE_LOGS
    mock_db.fetch_stage_logs_batch.return_value = {SAMPLE_JOB_ID: SAMPLE_STAGE_LOGS}
    mock_db.fetch_preprocess_metrics.return_value = SAMPLE_PREPROCESS_METRICS
    mock_db.fetch_preprocess_metrics_batch.return_value = {
        SAMPLE_JOB_ID: SAMPLE_PREPROCESS_METRICS
    }
    mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
    mock_db.fetch_clusters_for_run.return_value = [
        {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10}
        for i in range(5)
    ]
    mock_db.fetch_evaluation_history.return_value = []
    mock_db.fetch_evaluation_by_id.return_value = None

    mock_recap_worker = AsyncMock()
    mock_recap_worker.fetch_latest_genre_evaluation.return_value = (
        SAMPLE_GENRE_API_RESPONSE
    )

    mock_ollama = AsyncMock()
    batch_result = AsyncMock()
    batch_result.avg_coherence = 4.0
    batch_result.avg_consistency = 4.0
    batch_result.avg_fluency = 4.5
    batch_result.avg_relevance = 3.8
    batch_result.avg_overall = 4.075
    batch_result.success_count = 1
    mock_ollama.evaluate_batch.return_value = batch_result

    # Real evaluators with mock dependencies
    genre_eval = GenreEvaluator(mock_recap_worker, mock_db, thresholds)
    cluster_eval = ClusterEvaluator(mock_db, thresholds)

    # Use mocked sub-evaluators for summary to avoid model loading
    mock_rouge = MagicMock()
    mock_rouge.compute_batch.return_value = {
        "rouge_1_f1": 0.45, "rouge_2_f1": 0.22, "rouge_l_f1": 0.38
    }
    mock_bertscore = MagicMock()
    mock_bertscore.evaluate_batch.return_value = {
        "mean_precision": 0.72, "mean_recall": 0.68, "mean_f1": 0.70
    }
    mock_faith = MagicMock()
    fr = MagicMock()
    fr.faithfulness_score = 0.75
    fr.hallucination_score = 0.25
    mock_faith.detect_batch.return_value = [fr]

    summary_eval = SummaryEvaluator(
        mock_ollama, mock_db, settings, thresholds, weights,
        rouge=mock_rouge, bertscore=mock_bertscore, faithfulness=mock_faith,
    )
    pipeline_eval = PipelineEvaluator(mock_db, thresholds)

    # Real usecases
    run_eval_uc = RunEvaluationUsecase(
        genre_eval, cluster_eval, summary_eval, pipeline_eval, mock_db
    )
    get_metrics_uc = GetMetricsUsecase(
        genre_eval, cluster_eval, pipeline_eval, mock_db
    )

    app = FastAPI()
    app.include_router(health_router)
    app.include_router(evaluation_router)
    app.include_router(metrics_router)

    app.state.run_evaluation = run_eval_uc
    app.state.get_metrics = get_metrics_uc
    app.state.genre_evaluator = genre_eval
    app.state.cluster_evaluator = cluster_eval
    app.state.summary_evaluator = summary_eval
    app.state.db = mock_db

    return app


@pytest.fixture
def integration_client(integration_app):
    return TestClient(integration_app)


class TestFullEvaluationFlow:
    def test_health_check(self, integration_client):
        resp = integration_client.get("/health")
        assert resp.status_code == 200
        assert resp.json()["status"] == "healthy"

    def test_run_full_evaluation_end_to_end(self, integration_client):
        resp = integration_client.post(
            "/api/v1/evaluations/run",
            json={"window_days": 7},
        )

        assert resp.status_code == 200
        data = resp.json()
        assert data["evaluation_type"] == "full"
        assert data["overall_alert_level"] in ["ok", "warn", "critical"]
        assert len(data["job_ids"]) > 0
        # Verify all dimensions present
        assert data["genre_metrics"] is not None
        assert data["cluster_metrics"] is not None
        assert data["summary_metrics"] is not None
        assert data["pipeline_metrics"] is not None

    def test_latest_metrics_end_to_end(self, integration_client):
        resp = integration_client.get("/api/v1/metrics/latest")

        assert resp.status_code == 200
        data = resp.json()
        assert data["genre_macro_f1"] is not None
        assert data["pipeline_success_rate"] is not None
