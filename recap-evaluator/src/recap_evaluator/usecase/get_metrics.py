"""Get metrics usecase — retrieves latest metrics and trends."""

from datetime import datetime
from uuid import UUID

import structlog

from recap_evaluator.evaluator.cluster_evaluator import ClusterEvaluator
from recap_evaluator.evaluator.genre_evaluator import GenreEvaluator
from recap_evaluator.evaluator.pipeline_evaluator import PipelineEvaluator
from recap_evaluator.port.database_port import DatabasePort

logger = structlog.get_logger()

# Key metrics to extract from saved evaluation JSONB
_TREND_METRICS = {
    "genre_macro_f1": lambda m: m.get("genre", {}).get("macro_f1"),
    "cluster_avg_silhouette": lambda m: _avg_cluster_silhouette(m),
    "pipeline_success_rate": lambda m: m.get("pipeline", {}).get("success_rate"),
    "overall_quality_score": lambda m: m.get("summary", {}).get("overall_quality_score"),
}


def _avg_cluster_silhouette(metrics: dict) -> float | None:
    cluster = metrics.get("cluster")
    if not cluster:
        return None
    scores = [g.get("silhouette_score", 0) for g in cluster.values()]
    return sum(scores) / len(scores) if scores else None


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

    async def get_trends(self, window_days: int = 30) -> list[dict]:
        """Get metric trends from saved evaluation history."""
        history = await self._db.fetch_evaluation_history(limit=window_days)

        if not history:
            return []

        # Build time-series for each key metric
        trends: list[dict] = []
        for metric_name, extractor in _TREND_METRICS.items():
            data_points: list[dict] = []
            for record in reversed(history):  # oldest first
                metrics = record.get("metrics")
                if not metrics or not isinstance(metrics, dict):
                    continue
                value = extractor(metrics)
                if value is not None:
                    data_points.append({
                        "timestamp": record["created_at"],
                        "value": value,
                    })

            if not data_points:
                continue

            current = data_points[-1]["value"]
            change_7d = self._compute_change(data_points, 7, current)
            change_30d = self._compute_change(data_points, 30, current)

            trends.append({
                "metric_name": metric_name,
                "data_points": data_points,
                "current_value": current,
                "change_7d": change_7d,
                "change_30d": change_30d,
            })

        return trends

    @staticmethod
    def _compute_change(
        data_points: list[dict], days: int, current: float
    ) -> float | None:
        """Compute relative change over the given window."""
        if len(data_points) < 2:
            return None

        now = data_points[-1]["timestamp"]
        if isinstance(now, str):
            return None

        from datetime import timedelta

        cutoff = now - timedelta(days=days)
        # Find the oldest point within the window
        for point in data_points:
            ts = point["timestamp"]
            if isinstance(ts, datetime) and ts >= cutoff:
                old_value = point["value"]
                if old_value == 0:
                    return None
                return (current - old_value) / abs(old_value)
        return None
