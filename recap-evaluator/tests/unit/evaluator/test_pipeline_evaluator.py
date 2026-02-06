"""Tests for PipelineEvaluator."""

from uuid import uuid4

import pytest

from recap_evaluator.config import AlertThresholds
from recap_evaluator.domain.models import AlertLevel
from recap_evaluator.evaluator.pipeline_evaluator import PipelineEvaluator
from tests.fixtures.job_data import (
    SAMPLE_JOB,
    SAMPLE_JOB_ID,
    SAMPLE_PREPROCESS_METRICS,
    SAMPLE_STAGE_LOGS,
)


@pytest.fixture
def pipeline_evaluator(mock_db, alert_thresholds):
    return PipelineEvaluator(mock_db, alert_thresholds)


class TestPipelineEvaluator:
    async def test_evaluate_job_all_stages_completed(
        self, pipeline_evaluator, mock_db
    ):
        result = await pipeline_evaluator.evaluate_job(SAMPLE_JOB_ID)

        assert result.total_jobs == 1
        assert result.completed_jobs == 1
        assert result.success_rate == 1.0
        assert result.avg_articles_per_job == 100.0

    async def test_evaluate_job_with_failed_stage(
        self, pipeline_evaluator, mock_db
    ):
        failed_logs = [
            {**SAMPLE_STAGE_LOGS[0]},
            {**SAMPLE_STAGE_LOGS[1], "status": "failed"},
        ]
        mock_db.fetch_stage_logs.return_value = failed_logs

        result = await pipeline_evaluator.evaluate_job(uuid4())

        assert result.failed_jobs == 1
        assert result.success_rate == 0.0

    async def test_evaluate_job_empty_logs(self, pipeline_evaluator, mock_db):
        mock_db.fetch_stage_logs.return_value = []

        result = await pipeline_evaluator.evaluate_job(uuid4())

        assert result.total_jobs == 0

    async def test_evaluate_batch_uses_batch_queries(
        self, pipeline_evaluator, mock_db
    ):
        job_id = SAMPLE_JOB_ID
        mock_db.fetch_recent_jobs.side_effect = [
            [SAMPLE_JOB],  # completed
            [],  # failed
        ]
        mock_db.fetch_stage_logs_batch.return_value = {job_id: SAMPLE_STAGE_LOGS}
        mock_db.fetch_preprocess_metrics_batch.return_value = {
            job_id: SAMPLE_PREPROCESS_METRICS
        }

        result = await pipeline_evaluator.evaluate_batch([job_id])

        assert result.total_jobs == 1
        assert result.completed_jobs == 1
        mock_db.fetch_stage_logs_batch.assert_called_once()
        mock_db.fetch_preprocess_metrics_batch.assert_called_once()

    async def test_evaluate_batch_empty_returns_default(
        self, pipeline_evaluator
    ):
        result = await pipeline_evaluator.evaluate_batch([])

        assert result.total_jobs == 0
        assert result.success_rate == 0.0

    async def test_alert_level_critical_on_low_success_rate(
        self, pipeline_evaluator, mock_db
    ):
        job_ids = [uuid4()]
        mock_db.fetch_recent_jobs.side_effect = [
            [],  # no completed
            [{"job_id": job_ids[0], "status": "failed"}],  # all failed
        ]
        mock_db.fetch_stage_logs_batch.return_value = {}
        mock_db.fetch_preprocess_metrics_batch.return_value = {}

        result = await pipeline_evaluator.evaluate_batch(job_ids)

        assert result.alert_level == AlertLevel.CRITICAL
