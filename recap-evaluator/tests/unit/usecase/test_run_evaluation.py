"""Tests for RunEvaluationUsecase."""

from unittest.mock import AsyncMock

import pytest

from recap_evaluator.domain.models import (
    AlertLevel,
    EvaluationType,
    GenreEvaluationResult,
    PipelineMetrics,
    SummaryMetrics,
)
from recap_evaluator.usecase.run_evaluation import RunEvaluationUsecase
from tests.fixtures.job_data import SAMPLE_JOB, SAMPLE_JOB_2


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
    m.evaluate_batch.return_value = {}
    return m


@pytest.fixture
def mock_summary_eval():
    m = AsyncMock()
    m.evaluate_batch.return_value = SummaryMetrics(
        overall_quality_score=0.7, alert_level=AlertLevel.OK
    )
    return m


@pytest.fixture
def mock_pipeline_eval():
    m = AsyncMock()
    m.evaluate_batch.return_value = PipelineMetrics(
        success_rate=0.95, alert_level=AlertLevel.OK
    )
    return m


@pytest.fixture
def run_evaluation_uc(
    mock_genre_eval, mock_cluster_eval, mock_summary_eval, mock_pipeline_eval, mock_db
):
    return RunEvaluationUsecase(
        genre_evaluator=mock_genre_eval,
        cluster_evaluator=mock_cluster_eval,
        summary_evaluator=mock_summary_eval,
        pipeline_evaluator=mock_pipeline_eval,
        db=mock_db,
    )


class TestRunEvaluationUsecase:
    async def test_execute_full_evaluation(self, run_evaluation_uc, mock_db):
        mock_db.fetch_recent_jobs.return_value = [SAMPLE_JOB, SAMPLE_JOB_2]

        run = await run_evaluation_uc.execute(window_days=7)

        assert run.evaluation_type == EvaluationType.FULL
        assert len(run.job_ids) == 2
        assert run.genre_metrics is not None
        assert run.summary_metrics is not None
        assert run.pipeline_metrics is not None
        assert run.overall_alert_level == AlertLevel.OK

    async def test_execute_no_jobs_found(self, run_evaluation_uc, mock_db):
        mock_db.fetch_recent_jobs.return_value = []

        run = await run_evaluation_uc.execute()

        assert run.job_ids == []
        assert run.cluster_metrics == {}

    async def test_execute_propagates_critical_alert(
        self, mock_genre_eval, mock_cluster_eval, mock_summary_eval,
        mock_pipeline_eval, mock_db
    ):
        mock_pipeline_eval.evaluate_batch.return_value = PipelineMetrics(
            success_rate=0.5, alert_level=AlertLevel.CRITICAL
        )

        uc = RunEvaluationUsecase(
            mock_genre_eval, mock_cluster_eval, mock_summary_eval,
            mock_pipeline_eval, mock_db,
        )
        mock_db.fetch_recent_jobs.return_value = [SAMPLE_JOB]
        run = await uc.execute()

        assert run.overall_alert_level == AlertLevel.CRITICAL

    async def test_execute_exclude_dimensions(self, run_evaluation_uc, mock_db):
        mock_db.fetch_recent_jobs.return_value = [SAMPLE_JOB]

        run = await run_evaluation_uc.execute(
            include_genre=False,
            include_cluster=False,
            include_summary=False,
            include_pipeline=False,
        )

        assert run.genre_metrics is None
        assert run.summary_metrics is None
        assert run.pipeline_metrics is None
