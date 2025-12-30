"""Pipeline health evaluator."""

from uuid import UUID

import numpy as np
import structlog

from recap_evaluator.config import alert_thresholds
from recap_evaluator.domain.models import AlertLevel, PipelineMetrics
from recap_evaluator.infra.database import db

logger = structlog.get_logger()


class PipelineEvaluator:
    """Evaluates overall pipeline health metrics."""

    async def evaluate_job(self, job_id: UUID) -> PipelineMetrics:
        """Evaluate pipeline metrics for a single job."""
        # Fetch stage logs
        stage_logs = await db.fetch_stage_logs(job_id)
        if not stage_logs:
            return PipelineMetrics()

        # Calculate stage success rates
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

        # Calculate processing time
        if stage_logs:
            first_start = min(log["started_at"] for log in stage_logs if log.get("started_at"))
            last_finish = max(log["finished_at"] for log in stage_logs if log.get("finished_at"))
            if first_start and last_finish:
                processing_time = (last_finish - first_start).total_seconds()
            else:
                processing_time = 0.0
        else:
            processing_time = 0.0

        # Fetch preprocess metrics for article count
        preprocess = await db.fetch_preprocess_metrics(job_id)
        articles_count = preprocess.get("total_articles_fetched", 0) if preprocess else 0

        # Determine success based on all stages completing
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
        """Evaluate pipeline metrics across multiple jobs."""
        if not job_ids:
            return PipelineMetrics()

        # Fetch all jobs
        # Note: We need jobs with all statuses for accurate metrics
        all_jobs = await db.fetch_recent_jobs(
            days=14,
            status="completed",
        )
        job_id_set = set(job_ids)
        relevant_jobs = [j for j in all_jobs if j["job_id"] in job_id_set]

        # Also fetch failed jobs
        failed_jobs = await db.fetch_recent_jobs(days=14, status="failed")
        relevant_failed = [j for j in failed_jobs if j["job_id"] in job_id_set]

        total = len(relevant_jobs) + len(relevant_failed)
        completed = len(relevant_jobs)
        failed = len(relevant_failed)

        # Calculate aggregate stage success rates
        all_stage_results: dict[str, list[bool]] = {}
        all_processing_times: list[float] = []
        all_article_counts: list[int] = []

        for job_id in job_ids:
            stage_logs = await db.fetch_stage_logs(job_id)
            for log in stage_logs:
                stage = log["stage"]
                if stage not in all_stage_results:
                    all_stage_results[stage] = []
                all_stage_results[stage].append(log["status"] == "completed")

            # Calculate processing time
            if stage_logs:
                starts = [log["started_at"] for log in stage_logs if log.get("started_at")]
                finishes = [log["finished_at"] for log in stage_logs if log.get("finished_at")]
                if starts and finishes:
                    processing_time = (max(finishes) - min(starts)).total_seconds()
                    all_processing_times.append(processing_time)

            # Get article count
            preprocess = await db.fetch_preprocess_metrics(job_id)
            if preprocess:
                all_article_counts.append(preprocess.get("total_articles_fetched", 0))

        # Aggregate
        stage_success_rates = {
            stage: sum(results) / len(results)
            for stage, results in all_stage_results.items()
            if results
        }

        success_rate = completed / total if total > 0 else 0.0
        avg_processing_time = float(np.mean(all_processing_times)) if all_processing_times else 0.0
        avg_articles = float(np.mean(all_article_counts)) if all_article_counts else 0.0

        # Determine alert level
        threshold = alert_thresholds.get_threshold("pipeline_success_rate")
        if threshold:
            if success_rate < threshold.critical:
                alert_level = AlertLevel.CRITICAL
            elif success_rate < threshold.warn:
                alert_level = AlertLevel.WARN
            else:
                alert_level = AlertLevel.OK
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


# Singleton instance
pipeline_evaluator = PipelineEvaluator()
