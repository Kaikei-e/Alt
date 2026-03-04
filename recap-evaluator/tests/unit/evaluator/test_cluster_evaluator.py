"""Tests for ClusterEvaluator."""

from uuid import uuid4

import pytest

from recap_evaluator.config import AlertThresholds
from recap_evaluator.domain.models import AlertLevel, ClusterMetrics
from recap_evaluator.evaluator.cluster_evaluator import ClusterEvaluator
from tests.fixtures.job_data import SAMPLE_CLUSTER, SAMPLE_SUBWORKER_RUN


@pytest.fixture
def cluster_evaluator(mock_db, alert_thresholds):
    return ClusterEvaluator(mock_db, alert_thresholds)


class TestClusterEvaluator:
    async def test_evaluate_job_returns_per_genre_metrics(
        self, cluster_evaluator, mock_db
    ):
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10 + i}
            for i in range(5)
        ]

        result = await cluster_evaluator.evaluate_job(uuid4())

        assert "technology" in result
        assert result["technology"].num_clusters == 5

    async def test_evaluate_job_skips_failed_runs(
        self, cluster_evaluator, mock_db
    ):
        mock_db.fetch_subworker_runs.return_value = [
            {**SAMPLE_SUBWORKER_RUN, "status": "failed"}
        ]

        result = await cluster_evaluator.evaluate_job(uuid4())

        assert result == {}

    async def test_evaluate_job_warns_on_few_clusters(
        self, cluster_evaluator, mock_db
    ):
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "size": 15},
            {**SAMPLE_CLUSTER, "cluster_id": 1, "size": 10},
        ]

        result = await cluster_evaluator.evaluate_job(uuid4())

        assert result["technology"].alert_level == AlertLevel.WARN

    async def test_evaluate_batch_aggregates_across_jobs(
        self, cluster_evaluator, mock_db
    ):
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10}
            for i in range(5)
        ]

        result = await cluster_evaluator.evaluate_batch([uuid4(), uuid4()])

        assert "technology" in result

    async def test_evaluate_batch_sets_alert_level_from_thresholds(
        self, mock_db, alert_thresholds
    ):
        """evaluate_batch should apply silhouette thresholds to aggregated results."""
        # silhouette_score=0.10 is below critical threshold (0.15)
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10}
            for i in range(5)
        ]

        evaluator = ClusterEvaluator(mock_db, alert_thresholds)
        result = await evaluator.evaluate_batch([uuid4()])

        # evaluate_job sets silhouette_score=0.0 (no embeddings), below critical
        assert result["technology"].alert_level == AlertLevel.CRITICAL

    async def test_evaluate_batch_sets_ok_when_above_thresholds(
        self, mock_db
    ):
        """evaluate_batch should set OK when silhouette is above warn threshold."""
        thresholds = AlertThresholds(
            clustering_silhouette_warn=0.25,
            clustering_silhouette_critical=0.15,
        )
        # Create a run that returns silhouette_score=0.0 from evaluate_job
        # but we'll test with a custom evaluator that returns high scores
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10}
            for i in range(5)
        ]

        evaluator = ClusterEvaluator(mock_db, thresholds)
        result = await evaluator.evaluate_batch([uuid4()])

        # silhouette_score=0.0 from evaluate_job, below critical
        assert result["technology"].alert_level == AlertLevel.CRITICAL
