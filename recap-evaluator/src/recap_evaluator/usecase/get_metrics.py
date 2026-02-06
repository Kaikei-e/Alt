"""Get metrics usecase — retrieves latest metrics and trends."""

from uuid import UUID

import structlog

from recap_evaluator.evaluator.cluster_evaluator import ClusterEvaluator
from recap_evaluator.evaluator.genre_evaluator import GenreEvaluator
from recap_evaluator.evaluator.pipeline_evaluator import PipelineEvaluator
from recap_evaluator.port.database_port import DatabasePort

logger = structlog.get_logger()


class GetMetricsUsecase:
    """Retrieves latest metrics and evaluation history."""

    def __init__(
        self,
        genre_evaluator: GenreEvaluator,
        cluster_evaluator: ClusterEvaluator,
        pipeline_evaluator: PipelineEvaluator,
        db: DatabasePort,
    ) -> None:
        self._genre = genre_evaluator
        self._cluster = cluster_evaluator
        self._pipeline = pipeline_evaluator
        self._db = db

    async def get_latest(self) -> dict:
        """Get latest metrics summary across all dimensions."""
        result: dict = {}

        genre_result = await self._genre.fetch_latest_evaluation()
        if genre_result:
            result["genre_macro_f1"] = genre_result.macro_f1
            result["genre_alert_level"] = genre_result.alert_level.value

        jobs = await self._db.fetch_recent_jobs(days=7)
        if jobs:
            job_ids = [job["job_id"] for job in jobs]

            cluster_results = await self._cluster.evaluate_batch(job_ids[:5])
            if cluster_results:
                avg_silhouette = sum(
                    m.silhouette_score for m in cluster_results.values()
                ) / len(cluster_results)
                result["cluster_avg_silhouette"] = avg_silhouette
                if avg_silhouette < 0.15:
                    result["cluster_alert_level"] = "critical"
                elif avg_silhouette < 0.25:
                    result["cluster_alert_level"] = "warn"
                else:
                    result["cluster_alert_level"] = "ok"

            pipeline_result = await self._pipeline.evaluate_batch(job_ids)
            result["pipeline_success_rate"] = pipeline_result.success_rate
            result["pipeline_alert_level"] = pipeline_result.alert_level.value

            result["last_evaluation_at"] = jobs[0]["kicked_at"]

        return result

    async def get_evaluation_by_id(self, evaluation_id: UUID) -> dict | None:
        """Get a specific evaluation by ID."""
        return await self._db.fetch_evaluation_by_id(evaluation_id)

    async def get_evaluation_history(
        self, evaluation_type: str | None = None, limit: int = 30
    ) -> list[dict]:
        """Get evaluation run history."""
        return await self._db.fetch_evaluation_history(
            evaluation_type=evaluation_type, limit=limit
        )
