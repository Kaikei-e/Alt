"""Tests for ClusterEvaluator."""

from unittest.mock import AsyncMock
from uuid import uuid4

import numpy as np
import pytest

from recap_evaluator.domain.models import AlertLevel, ClusterMetrics
from recap_evaluator.evaluator.cluster_evaluator import ClusterEvaluator
from recap_evaluator.usecase.run_evaluation import RunEvaluationUsecase
from tests.fixtures.job_data import (
    SAMPLE_CLUSTER,
    SAMPLE_JOB,
    SAMPLE_SUBWORKER_RUN,
)


@pytest.fixture
def cluster_evaluator(mock_db, alert_thresholds):
    return ClusterEvaluator(mock_db, alert_thresholds)


class TestClusterEvaluator:
    async def test_evaluate_job_returns_per_genre_metrics(self, cluster_evaluator, mock_db):
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10 + i} for i in range(5)
        ]

        result = await cluster_evaluator.evaluate_job(uuid4())

        assert "technology" in result
        assert result["technology"].num_clusters == 5

    async def test_evaluate_job_skips_failed_runs(self, cluster_evaluator, mock_db):
        mock_db.fetch_subworker_runs.return_value = [{**SAMPLE_SUBWORKER_RUN, "status": "failed"}]

        result = await cluster_evaluator.evaluate_job(uuid4())

        assert result == {}

    async def test_evaluate_job_warns_on_few_clusters(self, cluster_evaluator, mock_db):
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "size": 15},
            {**SAMPLE_CLUSTER, "cluster_id": 1, "size": 10},
        ]

        result = await cluster_evaluator.evaluate_job(uuid4())

        assert result["technology"].alert_level == AlertLevel.WARN

    async def test_evaluate_batch_aggregates_across_jobs(self, cluster_evaluator, mock_db):
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10} for i in range(5)
        ]

        result = await cluster_evaluator.evaluate_batch([uuid4(), uuid4()])

        assert "technology" in result

    async def test_evaluate_batch_excludes_uncomputed_silhouette_from_alert(
        self, mock_db, alert_thresholds
    ):
        """evaluate_job never computes a real silhouette on the DB-only path
        (no embeddings available), so every per-job ClusterMetrics carries
        silhouette_score=None. evaluate_batch must exclude that from the
        threshold check instead of treating "not computed" as a 0.0 score —
        otherwise the aggregate is always CRITICAL regardless of real quality.
        """
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [
            {**SAMPLE_CLUSTER, "cluster_id": i, "size": 10} for i in range(5)
        ]

        evaluator = ClusterEvaluator(mock_db, alert_thresholds)
        result = await evaluator.evaluate_batch([uuid4()])

        assert result["technology"].silhouette_score is None
        assert result["technology"].alert_level == AlertLevel.OK

    async def test_evaluate_batch_keeps_worst_per_job_alert(self, cluster_evaluator, mock_db):
        """One collapsed night must not be averaged away by the healthy ones.

        The aggregate carries the worst per-job severity; taking the mean
        cluster count instead (5 and 1 average to 3, i.e. "OK") would hide
        exactly the night that needs looking at.
        """
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.side_effect = [
            [{**SAMPLE_CLUSTER, "cluster_id": i, "size": 10} for i in range(5)],
            [{**SAMPLE_CLUSTER, "size": 50}],
        ]

        result = await cluster_evaluator.evaluate_batch([uuid4(), uuid4()])

        assert result["technology"].alert_level == AlertLevel.WARN

    async def test_collapsed_clusters_reach_the_caller_as_warn(self, mock_db, alert_thresholds):
        """Production reaches ClusterEvaluator only through evaluate_batch.

        A night where a genre collapsed into a single cluster must surface as
        WARN on both the per-genre metrics and the run's overall level — the
        DB-only path has no silhouette to recompute from, so dropping the
        per-job alerts pins the whole dimension at OK forever.
        """
        mock_db.fetch_recent_jobs.return_value = [SAMPLE_JOB]
        mock_db.fetch_subworker_runs.return_value = [SAMPLE_SUBWORKER_RUN]
        mock_db.fetch_clusters_for_run.return_value = [{**SAMPLE_CLUSTER, "size": 42}]

        usecase = RunEvaluationUsecase(
            genre_evaluator=AsyncMock(),
            cluster_evaluator=ClusterEvaluator(mock_db, alert_thresholds),
            summary_evaluator=AsyncMock(),
            pipeline_evaluator=AsyncMock(),
            db=mock_db,
        )

        run = await usecase.execute(
            include_genre=False, include_summary=False, include_pipeline=False
        )

        assert run.cluster_metrics["technology"].alert_level == AlertLevel.WARN
        assert run.overall_alert_level == AlertLevel.WARN

    def test_evaluate_with_embeddings_reports_critical_on_degraded_clustering(
        self, cluster_evaluator
    ):
        """Meta-test: deliberately shredded clustering must trip CRITICAL.

        Two well-separated point pairs labelled across the gap give a strongly
        negative silhouette — if this returns OK the threshold wiring is dead.
        """
        embeddings = np.array([[0.0, 0.0], [0.1, 0.0], [10.0, 0.0], [10.1, 0.0]])
        labels = np.array([0, 1, 0, 1])

        metrics = cluster_evaluator.evaluate_with_embeddings(embeddings, labels)

        assert metrics.silhouette_score is not None
        assert metrics.silhouette_score < 0
        assert metrics.alert_level == AlertLevel.CRITICAL

    async def test_evaluate_batch_applies_thresholds_to_computed_silhouette(
        self, mock_db, alert_thresholds, monkeypatch
    ):
        """When a per-job silhouette score IS available (e.g. a future
        embeddings-based evaluate_job), evaluate_batch must still alert on
        it — the None-exclusion must not swallow real critical scores."""
        evaluator = ClusterEvaluator(mock_db, alert_thresholds)

        async def fake_evaluate_job(job_id):
            return {
                "technology": ClusterMetrics(
                    num_clusters=5,
                    avg_cluster_size=10.0,
                    min_cluster_size=10,
                    max_cluster_size=10,
                    silhouette_score=0.10,  # below critical threshold (0.15)
                )
            }

        monkeypatch.setattr(evaluator, "evaluate_job", fake_evaluate_job)

        result = await evaluator.evaluate_batch([uuid4()])

        assert result["technology"].silhouette_score == pytest.approx(0.10)
        assert result["technology"].alert_level == AlertLevel.CRITICAL
