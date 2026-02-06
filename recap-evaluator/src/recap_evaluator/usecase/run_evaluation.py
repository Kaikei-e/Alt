"""Run evaluation usecase — orchestrates full/individual evaluation runs."""

from datetime import UTC, datetime
from uuid import uuid4

import structlog

from recap_evaluator.domain.models import AlertLevel, EvaluationRun, EvaluationType
from recap_evaluator.evaluator.cluster_evaluator import ClusterEvaluator
from recap_evaluator.evaluator.genre_evaluator import GenreEvaluator
from recap_evaluator.evaluator.pipeline_evaluator import PipelineEvaluator
from recap_evaluator.evaluator.summary_evaluator import SummaryEvaluator
from recap_evaluator.port.database_port import DatabasePort
from recap_evaluator.usecase.alert_resolver import AlertResolver

logger = structlog.get_logger()


class RunEvaluationUsecase:
    """Orchestrates evaluation across all dimensions."""

    def __init__(
        self,
        genre_evaluator: GenreEvaluator,
        cluster_evaluator: ClusterEvaluator,
        summary_evaluator: SummaryEvaluator,
        pipeline_evaluator: PipelineEvaluator,
        db: DatabasePort,
    ) -> None:
        self._genre = genre_evaluator
        self._cluster = cluster_evaluator
        self._summary = summary_evaluator
        self._pipeline = pipeline_evaluator
        self._db = db

    async def execute(
        self,
        window_days: int = 14,
        include_genre: bool = True,
        include_cluster: bool = True,
        include_summary: bool = True,
        include_pipeline: bool = True,
        sample_per_job: int = 3,
    ) -> EvaluationRun:
        evaluation_id = uuid4()
        created_at = datetime.now(UTC)

        logger.info(
            "Starting full evaluation",
            evaluation_id=str(evaluation_id),
            window_days=window_days,
        )

        jobs = await self._db.fetch_recent_jobs(days=window_days)
        job_ids = [job["job_id"] for job in jobs]

        run = EvaluationRun(
            evaluation_id=evaluation_id,
            evaluation_type=EvaluationType.FULL,
            job_ids=job_ids,
            created_at=created_at,
            window_days=window_days,
        )

        alert_levels: list[AlertLevel] = []

        if include_genre:
            genre_result = await self._genre.fetch_latest_evaluation()
            if genre_result:
                run.genre_metrics = genre_result
                alert_levels.append(genre_result.alert_level)

        if include_cluster and job_ids:
            run.cluster_metrics = await self._cluster.evaluate_batch(job_ids)

        if include_summary and job_ids:
            run.summary_metrics = await self._summary.evaluate_batch(
                job_ids, sample_per_job=sample_per_job
            )
            if run.summary_metrics:
                alert_levels.append(run.summary_metrics.alert_level)

        if include_pipeline and job_ids:
            run.pipeline_metrics = await self._pipeline.evaluate_batch(job_ids)
            if run.pipeline_metrics:
                alert_levels.append(run.pipeline_metrics.alert_level)

        run.overall_alert_level = AlertResolver.resolve(alert_levels)

        logger.info(
            "Full evaluation completed",
            evaluation_id=str(evaluation_id),
            overall_alert_level=run.overall_alert_level.value,
        )

        return run
