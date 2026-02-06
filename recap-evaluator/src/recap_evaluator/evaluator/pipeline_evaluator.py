"""Pipeline health evaluator with DI."""

from uuid import UUID

import numpy as np
import structlog

from recap_evaluator.config import AlertThresholds
from recap_evaluator.domain.models import AlertLevel, PipelineMetrics
from recap_evaluator.port.database_port import DatabasePort

logger = structlog.get_logger()


class PipelineEvaluator:
    """Evaluates overall pipeline health metrics."""

    def __init__(self, db: DatabasePort, thresholds: AlertThresholds) -> None:
        self._db = db
        self._thresholds = thresholds

    async def evaluate_job(self, job_id: UUID) -> PipelineMetrics:
        stage_logs = await self._db.fetch_stage_logs(job_id)
        if not stage_logs:
            return PipelineMetrics()

        stage_results: dict[str, list[bool]] = {}
        for log in stage_logs:
            stage = log["stage"]
            if stage not in stage_results:
                stage_results[stage] = []
            stage_results[stage].append(log["status"] == "completed")

        stage_success_rates = {
            stage: sum(results) / len(results)
            for stage, results in stage_results.items()
            if results
        }

        processing_time = 0.0
        if stage_logs:
            starts = [log["started_at"] for log in stage_logs if log.get("started_at")]
            finishes = [log["finished_at"] for log in stage_logs if log.get("finished_at")]
            if starts and finishes:
                processing_time = (max(finishes) - min(starts)).total_seconds()

        preprocess = await self._db.fetch_preprocess_metrics(job_id)
        articles_count = preprocess.get("total_articles_fetched", 0) if preprocess else 0

        all_completed = all(log["status"] == "completed" for log in stage_logs)

        return PipelineMetrics(
            total_jobs=1,
            completed_jobs=1 if all_completed else 0,
            failed_jobs=0 if all_completed else 1,
            success_rate=1.0 if all_completed else 0.0,
            avg_articles_per_job=float(articles_count),
            avg_processing_time_seconds=processing_time,
            stage_success_rates=stage_success_rates,
        )

    async def evaluate_batch(self, job_ids: list[UUID]) -> PipelineMetrics:
        if not job_ids:
            return PipelineMetrics()

        all_jobs = await self._db.fetch_recent_jobs(days=14, status="completed")
        job_id_set = set(job_ids)
        relevant_jobs = [j for j in all_jobs if j["job_id"] in job_id_set]

        failed_jobs = await self._db.fetch_recent_jobs(days=14, status="failed")
        relevant_failed = [j for j in failed_jobs if j["job_id"] in job_id_set]

        total = len(relevant_jobs) + len(relevant_failed)
        completed = len(relevant_jobs)
        failed = len(relevant_failed)

        # Use batch query for stage logs
        stage_logs_map = await self._db.fetch_stage_logs_batch(job_ids)
        preprocess_map = await self._db.fetch_preprocess_metrics_batch(job_ids)

        all_stage_results: dict[str, list[bool]] = {}
        all_processing_times: list[float] = []
        all_article_counts: list[int] = []

        for job_id in job_ids:
            stage_logs = stage_logs_map.get(job_id, [])
            for log in stage_logs:
                stage = log["stage"]
                if stage not in all_stage_results:
                    all_stage_results[stage] = []
                all_stage_results[stage].append(log["status"] == "completed")

            if stage_logs:
                starts = [log["started_at"] for log in stage_logs if log.get("started_at")]
                finishes = [log["finished_at"] for log in stage_logs if log.get("finished_at")]
                if starts and finishes:
                    processing_time = (max(finishes) - min(starts)).total_seconds()
                    all_processing_times.append(processing_time)

            preprocess = preprocess_map.get(job_id)
            if preprocess:
                all_article_counts.append(preprocess.get("total_articles_fetched", 0))

        stage_success_rates = {
            stage: sum(results) / len(results)
            for stage, results in all_stage_results.items()
            if results
        }

        success_rate = completed / total if total > 0 else 0.0
        avg_processing_time = (
            float(np.mean(all_processing_times)) if all_processing_times else 0.0
        )
        avg_articles = float(np.mean(all_article_counts)) if all_article_counts else 0.0

        warn = self._thresholds.get_warn("pipeline_success_rate")
        critical = self._thresholds.get_critical("pipeline_success_rate")
        if critical is not None and success_rate < critical:
            alert_level = AlertLevel.CRITICAL
        elif warn is not None and success_rate < warn:
            alert_level = AlertLevel.WARN
        else:
            alert_level = AlertLevel.OK

        metrics = PipelineMetrics(
            total_jobs=total,
            completed_jobs=completed,
            failed_jobs=failed,
            success_rate=success_rate,
            avg_articles_per_job=avg_articles,
            avg_processing_time_seconds=avg_processing_time,
            stage_success_rates=stage_success_rates,
            alert_level=alert_level,
        )

        logger.info(
            "Pipeline evaluation completed",
            total_jobs=total,
            completed=completed,
            failed=failed,
            success_rate=success_rate,
            alert_level=alert_level.value,
        )

        return metrics
