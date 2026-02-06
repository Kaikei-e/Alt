"""Summary quality evaluator — orchestrates G-Eval, ROUGE, BERTScore, Faithfulness."""

import asyncio
import random
import re
from concurrent.futures import ThreadPoolExecutor
from typing import Literal
from uuid import UUID

import structlog

from recap_evaluator.config import AlertThresholds, EvaluatorWeights, Settings
from recap_evaluator.domain.models import AlertLevel, SummaryMetrics
from recap_evaluator.evaluator.bertscore import BERTScoreEvaluator
from recap_evaluator.evaluator.faithfulness import FaithfulnessEvaluator
from recap_evaluator.evaluator.rouge import ROUGEEvaluator
from recap_evaluator.gateway.ollama_gateway import OllamaGateway
from recap_evaluator.port.database_port import DatabasePort

logger = structlog.get_logger()


class SummaryEvaluator:
    """Multi-dimensional summary quality evaluator."""

    def __init__(
        self,
        ollama: OllamaGateway,
        db: DatabasePort,
        settings: Settings,
        thresholds: AlertThresholds,
        weights: EvaluatorWeights,
        rouge: ROUGEEvaluator | None = None,
        bertscore: BERTScoreEvaluator | None = None,
        faithfulness: FaithfulnessEvaluator | None = None,
        executor: ThreadPoolExecutor | None = None,
    ) -> None:
        self._ollama = ollama
        self._db = db
        self._sample_size = settings.geval_sample_size
        self._thresholds = thresholds
        self._weights = weights
        self._rouge = rouge or ROUGEEvaluator()
        self._bertscore = bertscore or BERTScoreEvaluator()
        self._faithfulness = faithfulness or FaithfulnessEvaluator()
        self._executor = executor or ThreadPoolExecutor(
            max_workers=settings.evaluation_thread_pool_size
        )
        self._lang: Literal["ja", "en"] = "ja"

    async def evaluate_batch(
        self,
        job_ids: list[UUID],
        sample_per_job: int = 3,
    ) -> SummaryMetrics:
        all_eval_items: list[tuple[str, str]] = []

        for job_id in job_ids:
            outputs = await self._db.fetch_outputs(job_id)
            if not outputs:
                continue

            articles = await self._db.fetch_job_articles(job_id)
            sampled_outputs = random.sample(outputs, min(sample_per_job, len(outputs)))

            for output in sampled_outputs:
                summary = output.get("summary_ja", "")
                if not summary:
                    continue

                source_text = "\n\n".join(
                    f"Title: {a['title']}\n{a.get('fulltext_html', '')[:500]}"
                    for a in articles[:5]
                )
                all_eval_items.append((source_text, summary))

        if len(all_eval_items) > self._sample_size:
            all_eval_items = random.sample(all_eval_items, self._sample_size)

        if not all_eval_items:
            logger.warning("No summaries found for batch evaluation")
            return SummaryMetrics()

        logger.info(
            "Starting multi-dimensional batch evaluation",
            total_items=len(all_eval_items),
            job_count=len(job_ids),
        )

        metrics = await self._run_multi_evaluation(all_eval_items)
        metrics.alert_level = self._determine_alert_level(metrics)

        logger.info(
            "Multi-dimensional batch evaluation completed",
            sample_count=metrics.sample_count,
            overall_quality_score=metrics.overall_quality_score,
            alert_level=metrics.alert_level.value,
        )

        return metrics

    async def _run_multi_evaluation(
        self, eval_items: list[tuple[str, str]]
    ) -> SummaryMetrics:
        sources = [item[0] for item in eval_items]
        summaries = [item[1] for item in eval_items]

        metrics = SummaryMetrics(sample_count=len(eval_items), success_count=0)

        tasks: list[tuple[str, asyncio.Task]] = []
        loop = asyncio.get_event_loop()

        # G-Eval (async)
        tasks.append(("geval", asyncio.ensure_future(self._run_geval(eval_items))))

        # ROUGE (CPU-bound, thread pool)
        tasks.append((
            "rouge",
            asyncio.ensure_future(
                loop.run_in_executor(
                    self._executor,
                    lambda: self._rouge.compute_batch(summaries, sources, lang=self._lang),
                )
            ),
        ))

        # BERTScore (CPU-bound, thread pool)
        tasks.append((
            "bertscore",
            asyncio.ensure_future(
                loop.run_in_executor(
                    self._executor,
                    lambda: self._bertscore.evaluate_batch(
                        summaries, sources, lang=self._lang
                    ),
                )
            ),
        ))

        # Faithfulness (CPU-bound, thread pool)
        source_sentences = [self._split_to_sentences(s) for s in sources]
        tasks.append((
            "faithfulness",
            asyncio.ensure_future(
                loop.run_in_executor(
                    self._executor,
                    lambda: self._run_faithfulness_sync(summaries, source_sentences),
                )
            ),
        ))

        results = await asyncio.gather(
            *[task[1] for task in tasks], return_exceptions=True
        )

        for i, (name, _) in enumerate(tasks):
            result = results[i]
            if isinstance(result, Exception):
                logger.error(f"{name} evaluation failed", error=str(result))
                continue
            self._apply_result(metrics, name, result)

        metrics.overall_quality_score = self._calculate_composite_score(metrics)
        return metrics

    async def _run_geval(self, eval_items: list[tuple[str, str]]) -> dict:
        try:
            batch_result = await self._ollama.evaluate_batch(eval_items)
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

    def _run_faithfulness_sync(
        self, summaries: list[str], source_sentences: list[list[str]]
    ) -> dict:
        try:
            results = self._faithfulness.detect_batch(summaries, source_sentences)
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
        pattern = r"(?<=[.!?。！？])\s*"
        sentences = re.split(pattern, text.strip())
        return [s.strip() for s in sentences if s.strip()]

    def _apply_result(
        self, metrics: SummaryMetrics, evaluator_name: str, result: dict
    ) -> None:
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
        total_weight = 0.0
        weighted_sum = 0.0

        if metrics.geval_overall > 0:
            geval_normalized = (metrics.geval_overall - 1) / 4
            weighted_sum += self._weights.geval * geval_normalized
            total_weight += self._weights.geval

        if metrics.bertscore_f1 > 0:
            weighted_sum += self._weights.bertscore * metrics.bertscore_f1
            total_weight += self._weights.bertscore

        if metrics.faithfulness_score > 0:
            weighted_sum += self._weights.faithfulness * metrics.faithfulness_score
            total_weight += self._weights.faithfulness

        if metrics.rouge_l_f1 > 0:
            weighted_sum += self._weights.rouge_l * metrics.rouge_l_f1
            total_weight += self._weights.rouge_l

        return weighted_sum / total_weight if total_weight > 0 else 0.0

    def _determine_alert_level(self, metrics: SummaryMetrics) -> AlertLevel:
        critical_count = 0
        warn_count = 0

        for dim in ["coherence", "consistency", "fluency", "relevance"]:
            warn = self._thresholds.get_warn(f"geval_{dim}")
            critical = self._thresholds.get_critical(f"geval_{dim}")
            value = getattr(metrics, dim)

            if critical is not None and value > 0 and value < critical:
                critical_count += 1
            elif warn is not None and value > 0 and value < warn:
                warn_count += 1

        if metrics.hallucination_rate > 0.5:
            critical_count += 1
        elif metrics.hallucination_rate > 0.3:
            warn_count += 1

        if metrics.overall_quality_score > 0 and metrics.overall_quality_score < 0.3:
            critical_count += 1
        elif metrics.overall_quality_score > 0 and metrics.overall_quality_score < 0.5:
            warn_count += 1

        if critical_count >= 2:
            return AlertLevel.CRITICAL
        if critical_count >= 1 or warn_count >= 2:
            return AlertLevel.WARN
        return AlertLevel.OK
