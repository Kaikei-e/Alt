"""Summary quality evaluator using G-Eval (Ollama)."""

import random
from uuid import UUID

import structlog

from recap_evaluator.config import alert_thresholds, settings
from recap_evaluator.domain.models import AlertLevel, SummaryMetrics
from recap_evaluator.infra.database import db
from recap_evaluator.infra.ollama import ollama_client

logger = structlog.get_logger()


class SummaryEvaluator:
    """Evaluates summary quality using G-Eval with Ollama."""

    def __init__(self, sample_size: int | None = None) -> None:
        self.sample_size = sample_size or settings.geval_sample_size

    async def evaluate_job(self, job_id: UUID) -> SummaryMetrics:
        """Evaluate summary quality for a single job."""
        # Fetch outputs (summaries) for this job
        outputs = await db.fetch_outputs(job_id)
        if not outputs:
            logger.warning("No outputs found for job", job_id=str(job_id))
            return SummaryMetrics()

        # Fetch source articles
        articles = await db.fetch_job_articles(job_id)
        articles_by_id = {a["article_id"]: a for a in articles}

        # Prepare evaluation items
        eval_items: list[tuple[str, str]] = []
        for output in outputs:
            summary = output.get("summary_ja", "")
            if not summary:
                continue

            # Get source articles for this genre
            # Note: We don't have direct article-to-genre mapping in outputs
            # Using all articles for now, truncated
            source_text = "\n\n".join(
                f"Title: {a['title']}\n{a.get('fulltext_html', '')[:500]}"
                for a in list(articles_by_id.values())[:5]
            )

            eval_items.append((source_text, summary))

        if not eval_items:
            logger.warning("No valid summaries found for evaluation", job_id=str(job_id))
            return SummaryMetrics()

        # Run G-Eval
        batch_result = await ollama_client.evaluate_batch(eval_items)

        # Convert to metrics
        metrics = SummaryMetrics(
            coherence=batch_result.avg_coherence,
            consistency=batch_result.avg_consistency,
            fluency=batch_result.avg_fluency,
            relevance=batch_result.avg_relevance,
            overall=batch_result.avg_overall,
            sample_count=batch_result.count,
            success_count=batch_result.success_count,
        )

        # Set alert level
        metrics.alert_level = self._determine_alert_level(metrics)

        return metrics

    async def evaluate_batch(
        self,
        job_ids: list[UUID],
        sample_per_job: int = 3,
    ) -> SummaryMetrics:
        """Evaluate summary quality across multiple jobs with sampling."""
        all_eval_items: list[tuple[str, str]] = []

        for job_id in job_ids:
            # Fetch outputs for this job
            outputs = await db.fetch_outputs(job_id)
            if not outputs:
                continue

            # Fetch source articles
            articles = await db.fetch_job_articles(job_id)

            # Sample outputs
            sampled_outputs = random.sample(outputs, min(sample_per_job, len(outputs)))

            for output in sampled_outputs:
                summary = output.get("summary_ja", "")
                if not summary:
                    continue

                # Prepare source text
                source_text = "\n\n".join(
                    f"Title: {a['title']}\n{a.get('fulltext_html', '')[:500]}" for a in articles[:5]
                )

                all_eval_items.append((source_text, summary))

        # Limit total samples
        if len(all_eval_items) > self.sample_size:
            all_eval_items = random.sample(all_eval_items, self.sample_size)

        if not all_eval_items:
            logger.warning("No summaries found for batch evaluation")
            return SummaryMetrics()

        logger.info(
            "Starting batch G-Eval evaluation",
            total_items=len(all_eval_items),
            job_count=len(job_ids),
        )

        # Run G-Eval
        batch_result = await ollama_client.evaluate_batch(all_eval_items)

        # Convert to metrics
        metrics = SummaryMetrics(
            coherence=batch_result.avg_coherence,
            consistency=batch_result.avg_consistency,
            fluency=batch_result.avg_fluency,
            relevance=batch_result.avg_relevance,
            overall=batch_result.avg_overall,
            sample_count=batch_result.count,
            success_count=batch_result.success_count,
        )

        # Set alert level
        metrics.alert_level = self._determine_alert_level(metrics)

        logger.info(
            "Batch G-Eval evaluation completed",
            sample_count=metrics.sample_count,
            success_count=metrics.success_count,
            overall_score=metrics.overall,
            alert_level=metrics.alert_level.value,
        )

        return metrics

    def _determine_alert_level(self, metrics: SummaryMetrics) -> AlertLevel:
        """Determine alert level based on G-Eval scores."""
        # Check each dimension
        dimensions = ["coherence", "consistency", "fluency", "relevance"]
        critical_count = 0
        warn_count = 0

        for dim in dimensions:
            threshold = alert_thresholds.get_threshold(f"geval_{dim}")
            if not threshold:
                continue

            value = getattr(metrics, dim)
            if value < threshold.critical:
                critical_count += 1
            elif value < threshold.warn:
                warn_count += 1

        # Determine overall level
        if critical_count >= 2:
            return AlertLevel.CRITICAL
        if critical_count >= 1 or warn_count >= 2:
            return AlertLevel.WARN
        return AlertLevel.OK


# Singleton instance
summary_evaluator = SummaryEvaluator()
