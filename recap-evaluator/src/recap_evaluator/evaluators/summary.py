"""Summary quality evaluator using multi-dimensional metrics.

Orchestrates G-Eval (Ollama), ROUGE, BERTScore, and Faithfulness evaluators
for comprehensive summary quality assessment.
"""

import asyncio
import random
from concurrent.futures import ThreadPoolExecutor
from typing import Literal
from uuid import UUID

import structlog

from recap_evaluator.config import alert_thresholds, settings
from recap_evaluator.domain.models import AlertLevel, SummaryMetrics
from recap_evaluator.evaluators.bertscore_eval import bertscore_evaluator
from recap_evaluator.evaluators.faithfulness_eval import faithfulness_evaluator
from recap_evaluator.evaluators.rouge_eval import rouge_evaluator
from recap_evaluator.infra.database import db
from recap_evaluator.infra.ollama import ollama_client

logger = structlog.get_logger()

# Thread pool for CPU-bound evaluations (ROUGE, BERTScore, Faithfulness)
_executor = ThreadPoolExecutor(max_workers=4)


class EvaluatorConfig:
    """Configuration for which evaluators to enable."""

    def __init__(
        self,
        enable_geval: bool = True,
        enable_rouge: bool = True,
        enable_bertscore: bool = True,
        enable_faithfulness: bool = True,
        lang: Literal["ja", "en"] = "ja",
    ):
        self.enable_geval = enable_geval
        self.enable_rouge = enable_rouge
        self.enable_bertscore = enable_bertscore
        self.enable_faithfulness = enable_faithfulness
        self.lang = lang

    # Weight distribution for composite score
    # Total = 100%
    WEIGHTS = {
        "geval": 0.40,       # G-Eval: 40%
        "bertscore": 0.25,   # BERTScore: 25%
        "faithfulness": 0.25, # Faithfulness: 25%
        "rouge_l": 0.10,     # ROUGE-L: 10%
    }


class SummaryEvaluator:
    """Evaluates summary quality using multiple evaluation methods.

    Combines:
    - G-Eval (LLM-based): Coherence, Consistency, Fluency, Relevance
    - ROUGE: N-gram overlap (ROUGE-1, ROUGE-2, ROUGE-L)
    - BERTScore: Semantic similarity
    - Faithfulness: NLI-based hallucination detection
    """

    def __init__(
        self,
        sample_size: int | None = None,
        config: EvaluatorConfig | None = None,
    ) -> None:
        self.sample_size = sample_size or settings.geval_sample_size
        self.config = config or EvaluatorConfig()

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
            source_text = "\n\n".join(
                f"Title: {a['title']}\n{a.get('fulltext_html', '')[:500]}"
                for a in list(articles_by_id.values())[:5]
            )

            eval_items.append((source_text, summary))

        if not eval_items:
            logger.warning("No valid summaries found for evaluation", job_id=str(job_id))
            return SummaryMetrics()

        # Run multi-dimensional evaluation
        metrics = await self._run_multi_evaluation(eval_items)

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
                    f"Title: {a['title']}\n{a.get('fulltext_html', '')[:500]}"
                    for a in articles[:5]
                )

                all_eval_items.append((source_text, summary))

        # Limit total samples
        if len(all_eval_items) > self.sample_size:
            all_eval_items = random.sample(all_eval_items, self.sample_size)

        if not all_eval_items:
            logger.warning("No summaries found for batch evaluation")
            return SummaryMetrics()

        logger.info(
            "Starting multi-dimensional batch evaluation",
            total_items=len(all_eval_items),
            job_count=len(job_ids),
            evaluators_enabled={
                "geval": self.config.enable_geval,
                "rouge": self.config.enable_rouge,
                "bertscore": self.config.enable_bertscore,
                "faithfulness": self.config.enable_faithfulness,
            },
        )

        # Run multi-dimensional evaluation
        metrics = await self._run_multi_evaluation(all_eval_items)

        # Set alert level
        metrics.alert_level = self._determine_alert_level(metrics)

        logger.info(
            "Multi-dimensional batch evaluation completed",
            sample_count=metrics.sample_count,
            success_count=metrics.success_count,
            overall_quality_score=metrics.overall_quality_score,
            geval_overall=metrics.geval_overall,
            rouge_l_f1=metrics.rouge_l_f1,
            bertscore_f1=metrics.bertscore_f1,
            faithfulness_score=metrics.faithfulness_score,
            alert_level=metrics.alert_level.value,
        )

        return metrics

    async def _run_multi_evaluation(
        self, eval_items: list[tuple[str, str]]
    ) -> SummaryMetrics:
        """Run multiple evaluators and aggregate results.

        Args:
            eval_items: List of (source_text, summary) tuples.

        Returns:
            SummaryMetrics with all evaluation results.
        """
        sources = [item[0] for item in eval_items]
        summaries = [item[1] for item in eval_items]

        # Initialize metrics
        metrics = SummaryMetrics(
            sample_count=len(eval_items),
            success_count=0,
        )

        # Create evaluation tasks
        tasks = []

        # G-Eval (async, uses Ollama)
        if self.config.enable_geval:
            tasks.append(("geval", self._run_geval(eval_items)))

        # ROUGE (sync, run in thread pool)
        if self.config.enable_rouge:
            tasks.append(("rouge", self._run_rouge(summaries, sources)))

        # BERTScore (sync, run in thread pool)
        if self.config.enable_bertscore:
            tasks.append(("bertscore", self._run_bertscore(summaries, sources)))

        # Faithfulness (sync, run in thread pool)
        if self.config.enable_faithfulness:
            # Convert sources to sentence lists for faithfulness evaluation
            source_sentences = [self._split_to_sentences(s) for s in sources]
            tasks.append(
                ("faithfulness", self._run_faithfulness(summaries, source_sentences))
            )

        # Run all evaluations concurrently
        if tasks:
            results = await asyncio.gather(
                *[task[1] for task in tasks],
                return_exceptions=True,
            )

            # Process results
            for i, (name, _) in enumerate(tasks):
                result = results[i]
                if isinstance(result, Exception):
                    logger.error(
                        f"{name} evaluation failed",
                        error=str(result),
                    )
                    continue

                self._apply_result(metrics, name, result)

        # Calculate composite score
        metrics.overall_quality_score = self._calculate_composite_score(metrics)

        # Backward compatibility: set 'overall' to geval_overall
        metrics.overall = metrics.geval_overall

        return metrics

    async def _run_geval(
        self, eval_items: list[tuple[str, str]]
    ) -> dict:
        """Run G-Eval using Ollama."""
        try:
            batch_result = await ollama_client.evaluate_batch(eval_items)
            return {
                "coherence": batch_result.avg_coherence,
                "consistency": batch_result.avg_consistency,
                "fluency": batch_result.avg_fluency,
                "relevance": batch_result.avg_relevance,
                "overall": batch_result.avg_overall,
                "success_count": batch_result.success_count,
            }
        except Exception as e:
            logger.error("G-Eval failed", error=str(e))
            return {}

    async def _run_rouge(
        self, summaries: list[str], sources: list[str]
    ) -> dict:
        """Run ROUGE evaluation in thread pool."""
        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(
            _executor,
            lambda: rouge_evaluator.compute_batch(
                summaries, sources, lang=self.config.lang
            ),
        )

    async def _run_bertscore(
        self, summaries: list[str], sources: list[str]
    ) -> dict:
        """Run BERTScore evaluation in thread pool."""
        loop = asyncio.get_event_loop()
        try:
            return await loop.run_in_executor(
                _executor,
                lambda: bertscore_evaluator.evaluate_batch(
                    summaries, sources, lang=self.config.lang
                ),
            )
        except Exception as e:
            logger.error("BERTScore failed", error=str(e))
            return {}

    async def _run_faithfulness(
        self, summaries: list[str], source_sentences: list[list[str]]
    ) -> dict:
        """Run Faithfulness evaluation in thread pool."""
        loop = asyncio.get_event_loop()
        try:
            results = await loop.run_in_executor(
                _executor,
                lambda: faithfulness_evaluator.detect_batch(summaries, source_sentences),
            )
            # Aggregate results
            total_faith = sum(r.faithfulness_score for r in results)
            total_halluc = sum(r.hallucination_score for r in results)
            n = len(results)
            return {
                "faithfulness_score": total_faith / n if n > 0 else 0.0,
                "hallucination_rate": total_halluc / n if n > 0 else 0.0,
            }
        except Exception as e:
            logger.error("Faithfulness evaluation failed", error=str(e))
            return {}

    def _split_to_sentences(self, text: str) -> list[str]:
        """Split text into sentences for faithfulness evaluation."""
        import re
        pattern = r"(?<=[.!?。！？])\s*"
        sentences = re.split(pattern, text.strip())
        return [s.strip() for s in sentences if s.strip()]

    def _apply_result(
        self, metrics: SummaryMetrics, evaluator_name: str, result: dict
    ) -> None:
        """Apply evaluation result to metrics."""
        if evaluator_name == "geval":
            metrics.coherence = result.get("coherence", 0.0)
            metrics.consistency = result.get("consistency", 0.0)
            metrics.fluency = result.get("fluency", 0.0)
            metrics.relevance = result.get("relevance", 0.0)
            metrics.geval_overall = result.get("overall", 0.0)
            metrics.success_count = result.get("success_count", 0)
        elif evaluator_name == "rouge":
            metrics.rouge_1_f1 = result.get("rouge_1_f1", 0.0)
            metrics.rouge_2_f1 = result.get("rouge_2_f1", 0.0)
            metrics.rouge_l_f1 = result.get("rouge_l_f1", 0.0)
        elif evaluator_name == "bertscore":
            metrics.bertscore_precision = result.get("mean_precision", 0.0)
            metrics.bertscore_recall = result.get("mean_recall", 0.0)
            metrics.bertscore_f1 = result.get("mean_f1", 0.0)
        elif evaluator_name == "faithfulness":
            metrics.faithfulness_score = result.get("faithfulness_score", 0.0)
            metrics.hallucination_rate = result.get("hallucination_rate", 0.0)

    def _calculate_composite_score(self, metrics: SummaryMetrics) -> float:
        """Calculate weighted composite quality score.

        Weights:
        - G-Eval: 40% (normalized from 1-5 scale to 0-1)
        - BERTScore: 25%
        - Faithfulness: 25%
        - ROUGE-L: 10%
        """
        weights = EvaluatorConfig.WEIGHTS
        total_weight = 0.0
        weighted_sum = 0.0

        # G-Eval (normalize from 1-5 to 0-1)
        if self.config.enable_geval and metrics.geval_overall > 0:
            geval_normalized = (metrics.geval_overall - 1) / 4  # Map 1-5 to 0-1
            weighted_sum += weights["geval"] * geval_normalized
            total_weight += weights["geval"]

        # BERTScore F1
        if self.config.enable_bertscore and metrics.bertscore_f1 > 0:
            weighted_sum += weights["bertscore"] * metrics.bertscore_f1
            total_weight += weights["bertscore"]

        # Faithfulness
        if self.config.enable_faithfulness and metrics.faithfulness_score > 0:
            weighted_sum += weights["faithfulness"] * metrics.faithfulness_score
            total_weight += weights["faithfulness"]

        # ROUGE-L
        if self.config.enable_rouge and metrics.rouge_l_f1 > 0:
            weighted_sum += weights["rouge_l"] * metrics.rouge_l_f1
            total_weight += weights["rouge_l"]

        # Return weighted average (or 0 if no evaluators ran)
        if total_weight > 0:
            return weighted_sum / total_weight
        return 0.0

    def _determine_alert_level(self, metrics: SummaryMetrics) -> AlertLevel:
        """Determine alert level based on multi-dimensional scores."""
        # Check each G-Eval dimension
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

        # Check faithfulness (high hallucination rate is concerning)
        if metrics.hallucination_rate > 0.5:
            critical_count += 1
        elif metrics.hallucination_rate > 0.3:
            warn_count += 1

        # Check overall quality score
        if metrics.overall_quality_score < 0.3:
            critical_count += 1
        elif metrics.overall_quality_score < 0.5:
            warn_count += 1

        # Determine overall level
        if critical_count >= 2:
            return AlertLevel.CRITICAL
        if critical_count >= 1 or warn_count >= 2:
            return AlertLevel.WARN
        return AlertLevel.OK


# Singleton instance
summary_evaluator = SummaryEvaluator()
