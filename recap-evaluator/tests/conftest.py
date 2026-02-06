"""Shared test fixtures for recap-evaluator."""

from unittest.mock import AsyncMock

import pytest

from recap_evaluator.config import AlertThresholds, EvaluatorWeights, Settings
from recap_evaluator.domain.models import (
    ClusterMetrics,
    GenreEvaluationResult,
    PipelineMetrics,
    SummaryMetrics,
)
from tests.fixtures.job_data import (
    SAMPLE_ARTICLE,
    SAMPLE_CLUSTER,
    SAMPLE_JOB,
    SAMPLE_JOB_2,
    SAMPLE_JOB_ID,
    SAMPLE_OUTPUT,
    SAMPLE_PREPROCESS_METRICS,
    SAMPLE_STAGE_LOGS,
    SAMPLE_SUBWORKER_RUN,
)


@pytest.fixture
def mock_settings() -> Settings:
    """Settings with test-safe defaults."""
    return Settings(
        recap_db_dsn="postgres://test:test@localhost:5432/test",
        ollama_url="http://localhost:11434",
        ollama_model="test-model",
        ollama_timeout=10,
        ollama_concurrency=2,
        recap_worker_url="http://localhost:8081",
        evaluation_window_days=7,
        geval_sample_size=5,
        enable_scheduler=False,
        evaluation_thread_pool_size=2,
        cors_allowed_origins=["http://localhost:3000"],
        log_level="DEBUG",
        log_format="console",
    )


@pytest.fixture
def alert_thresholds() -> AlertThresholds:
    """Default alert thresholds for tests."""
    return AlertThresholds()


@pytest.fixture
def evaluator_weights() -> EvaluatorWeights:
    """Default evaluator weights for tests."""
    return EvaluatorWeights()


@pytest.fixture
def mock_db() -> AsyncMock:
    """Mock DatabasePort with sensible defaults."""
    db = AsyncMock()
    db.fetch_recent_jobs.return_value = [SAMPLE_JOB, SAMPLE_JOB_2]
    db.fetch_job_articles.return_value = [SAMPLE_ARTICLE]
    db.fetch_outputs.return_value = [SAMPLE_OUTPUT]
    db.fetch_stage_logs.return_value = SAMPLE_STAGE_LOGS
    db.fetch_stage_logs_batch.return_value = {SAMPLE_JOB_ID: SAMPLE_STAGE_LOGS}
    db.fetch_preprocess_metrics.return_value = SAMPLE_PREPROCESS_METRICS
    db.fetch_preprocess_metrics_batch.return_value = {
        SAMPLE_JOB_ID: SAMPLE_PREPROCESS_METRICS
    }
    db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
    db.fetch_clusters_for_run.return_value = [SAMPLE_CLUSTER]
    db.fetch_genre_learning_results.return_value = []
    db.fetch_evaluation_by_id.return_value = None
    db.fetch_evaluation_history.return_value = []
    db.save_evaluation_run.return_value = None
    return db


@pytest.fixture
def mock_ollama() -> AsyncMock:
    """Mock LLMPort with default G-Eval results."""
    ollama = AsyncMock()

    # Single evaluation result
    single_result = AsyncMock()
    single_result.coherence = 4.0
    single_result.consistency = 4.0
    single_result.fluency = 4.5
    single_result.relevance = 3.8
    single_result.error = None
    single_result.average_score = 4.075
    ollama.evaluate_summary.return_value = single_result

    # Batch result
    batch_result = AsyncMock()
    batch_result.results = [single_result]
    batch_result.count = 1
    batch_result.success_count = 1
    batch_result.avg_coherence = 4.0
    batch_result.avg_consistency = 4.0
    batch_result.avg_fluency = 4.5
    batch_result.avg_relevance = 3.8
    batch_result.avg_overall = 4.075
    ollama.evaluate_batch.return_value = batch_result

    ollama.health_check.return_value = True
    return ollama


@pytest.fixture
def mock_recap_worker() -> AsyncMock:
    """Mock RecapWorkerPort."""
    rw = AsyncMock()
    rw.trigger_genre_evaluation.return_value = {"run_id": "test-run-001"}
    rw.fetch_latest_genre_evaluation.return_value = {
        "macro_precision": 0.85,
        "macro_recall": 0.80,
        "macro_f1": 0.82,
        "micro_precision": 0.88,
        "micro_recall": 0.83,
        "micro_f1": 0.85,
        "weighted_f1": 0.84,
        "total_items": 88,
        "per_genre_metrics": [
            {
                "genre": "technology",
                "tp": 50,
                "fp": 5,
                "fn_count": 3,
                "precision": 0.91,
                "recall": 0.94,
                "f1_score": 0.92,
            },
        ],
    }
    rw.fetch_genre_evaluation_by_id.return_value = None
    return rw
