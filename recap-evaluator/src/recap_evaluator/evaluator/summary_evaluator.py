"""Summary quality evaluator — orchestrates G-Eval, ROUGE, BERTScore, Faithfulness."""

import asyncio
import random
import re
from concurrent.futures import ThreadPoolExecutor
from typing import Any, Final, Literal
from uuid import UUID

import structlog

from recap_evaluator.config import AlertThresholds, EvaluatorWeights, Settings
from recap_evaluator.domain.models import AlertLevel, SummaryMetrics
from recap_evaluator.evaluator.bertscore import BERTScoreEvaluator
from recap_evaluator.evaluator.faithfulness import FaithfulnessEvaluator
from recap_evaluator.evaluator.fallback_rate import FallbackRateEvaluator
from recap_evaluator.evaluator.json_repair_rate import JsonRepairRateEvaluator
from recap_evaluator.evaluator.readability import ReadabilityEvaluator
from recap_evaluator.evaluator.redundancy import RedundancyEvaluator
from recap_evaluator.evaluator.rouge import ROUGEEvaluator
from recap_evaluator.evaluator.source_grounding import SourceGroundingEvaluator
from recap_evaluator.gateway.ollama_gateway import OllamaGateway
from recap_evaluator.port.database_port import DatabasePort

logger = structlog.get_logger()

# The axes that carry a weight in the composite score. An axis that raised in
# _run_multi_evaluation never reaches _apply_result, so its metrics stay at
# 0.0 — indistinguishable from a genuinely terrible measurement. Coverage is
# therefore tracked by name and never inferred from a value, for the same
# reason success_count exists for the G-Eval axis.
_COMPOSITE_AXES: Final[frozenset[str]] = frozenset(
    {"geval", "bertscore", "faithfulness", "rouge"}
)


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
        self._fallback = FallbackRateEvaluator()
        self._json_repair = JsonRepairRateEvaluator()
        self._redundancy = RedundancyEvaluator(rouge=self._rouge)
        self._readability = ReadabilityEvaluator(ollama)
        self._source_grounding = SourceGroundingEvaluator()
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
        all_outputs_flat: list[dict[str, Any]] = []

        for job_id in job_ids:
            outputs = await self._db.fetch_outputs(job_id)
            if not outputs:
                continue
            all_outputs_flat.extend(outputs)

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

        if not all_eval_items and not all_outputs_flat:
            logger.warning("No summaries found for batch evaluation")
            return SummaryMetrics()

        logger.info(
            "Starting multi-dimensional batch evaluation",
            total_items=len(all_eval_items),
            job_count=len(job_ids),
        )

        if all_eval_items:
            metrics, measured_axes = await self._run_multi_evaluation(all_eval_items)
        else:
            metrics = SummaryMetrics(sample_count=0, success_count=0)
            measured_axes = frozenset()

        await self._apply_morning_letter_axes(metrics, all_outputs_flat)

        metrics.alert_level = self._determine_alert_level(metrics, measured_axes)

        logger.info(
            "Multi-dimensional batch evaluation completed",
            sample_count=metrics.sample_count,
            overall_quality_score=metrics.overall_quality_score,
            # The composite is only comparable between runs when the same
            # axes fed it, and SummaryMetrics has nowhere to carry that, so
            # the coverage lives in the run log.
            measured_axes=sorted(measured_axes),
            alert_level=metrics.alert_level.value,
        )

        return metrics

    def shutdown(self) -> None:
        """Release the thread pool used for sync G-Eval/faithfulness calls.

        Must be called from the app lifespan's shutdown phase — the executor
        is otherwise never released for the life of the process.
        """
        self._executor.shutdown(wait=True, cancel_futures=True)

    async def _run_multi_evaluation(
        self, eval_items: list[tuple[str, str]]
    ) -> tuple[SummaryMetrics, frozenset[str]]:
        sources = [item[0] for item in eval_items]
        summaries = [item[1] for item in eval_items]

        metrics = SummaryMetrics(sample_count=len(eval_items), success_count=0)

        tasks: list[tuple[str, asyncio.Future[Any]]] = []
        loop = asyncio.get_running_loop()

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
            *[task[1] for task in tasks],
            return_exceptions=True,
        )
        # gather(return_exceptions=True) is intentional: each evaluator axis is
        # independent and we want partial metrics when one axis fails. TaskGroup
        # would cancel siblings on the first exception (DECREE §6 prefers
        # TaskGroup for all-or-nothing fan-out; this path needs best-effort).

        measured_axes: set[str] = set()
        for i, (name, _) in enumerate(tasks):
            result = results[i]
            if isinstance(result, BaseException):
                logger.error(
                    "evaluator_axis_failed",
                    evaluator=name,
                    error=str(result),
                )
                continue
            self._apply_result(metrics, name, result)
            measured_axes.add(name)

        # The G-Eval task can return without a single successful judgement
        # (every item errored inside the gateway). success_count is the only
        # record of that, so the axis counts as unmeasured even though the
        # task itself did not raise.
        if metrics.success_count == 0:
            measured_axes.discard("geval")

        axes = frozenset(measured_axes)
        metrics.overall_quality_score = self._calculate_composite_score(metrics, axes)
        return metrics, axes

    async def _run_geval(self, eval_items: list[tuple[str, str]]) -> dict:
        # self._ollama.evaluate_batch() already absorbs per-item failures via
        # gather(return_exceptions=True); anything it still raises is
        # unexpected and must surface to _run_multi_evaluation's outer
        # gather, which logs it and skips this axis — a local silent {}
        # fallback here would just mask the same failure with no traceback.
        batch_result = await self._ollama.evaluate_batch(eval_items)
        return {
            "coherence": batch_result.avg_coherence,
            "consistency": batch_result.avg_consistency,
            "fluency": batch_result.avg_fluency,
            "relevance": batch_result.avg_relevance,
            "overall": batch_result.avg_overall,
            "success_count": batch_result.success_count,
        }

    def _run_faithfulness_sync(
        self, summaries: list[str], source_sentences: list[list[str]]
    ) -> dict:
        # See _run_geval: failures propagate to the outer gather in
        # _run_multi_evaluation instead of being swallowed here.
        results = self._faithfulness.detect_batch(summaries, source_sentences)
        total_faith = sum(r.faithfulness_score for r in results)
        total_halluc = sum(r.hallucination_score for r in results)
        n = len(results)
        return {
            "faithfulness_score": total_faith / n if n > 0 else 0.0,
            "hallucination_rate": total_halluc / n if n > 0 else 0.0,
        }

    async def _apply_morning_letter_axes(
        self, metrics: SummaryMetrics, outputs: list[dict[str, Any]]
    ) -> None:
        if not outputs:
            return

        metrics.fallback_rate = self._fallback.compute(outputs)
        metrics.json_repair_rate = self._json_repair.compute(outputs)
        metrics.source_grounding_score = self._source_grounding.compute_batch(outputs)

        per_output_redundancy: list[float] = []
        for output in outputs:
            bullet_texts = self._extract_bullet_texts(output)
            if len(bullet_texts) >= 2:
                per_output_redundancy.append(self._redundancy.compute(bullet_texts))
        metrics.redundancy_score = (
            sum(per_output_redundancy) / len(per_output_redundancy)
            if per_output_redundancy
            else 0.0
        )

        summaries = [
            output.get("summary_ja", "")
            for output in outputs
            if output.get("summary_ja")
        ]
        if summaries:
            # ReadabilityEvaluator.evaluate_batch already absorbs recoverable
            # per-item failures (network/HTTP/unparseable LLM response) and
            # logs them; anything it still raises is a wiring bug (e.g. a
            # broken score_readability implementation) that must propagate
            # per CLAUDE.md rule 8, not be masked as a silent 0.0 here.
            metrics.readability_score = await self._readability.evaluate_batch(
                summaries[: self._sample_size]
            )

    @staticmethod
    def _extract_bullet_texts(output: dict[str, Any]) -> list[str]:
        body = output.get("body_json") or {}
        bullets = body.get("bullets") or []
        texts: list[str] = []
        for bullet in bullets:
            if isinstance(bullet, str):
                texts.append(bullet)
            elif isinstance(bullet, dict):
                text = bullet.get("text")
                if isinstance(text, str):
                    texts.append(text)
        return texts

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

    def _calculate_composite_score(
        self, metrics: SummaryMetrics, measured_axes: frozenset[str]
    ) -> float:
        """Weighted mean over the *configured* weights, not the surviving ones.

        Dividing by the surviving weight makes an outage look like an
        improvement — losing the 40 % G-Eval axis used to leave a score of
        ~0.71, comfortably above the 0.50 warn line — and makes the value
        stored in recap_evaluation_runs incomparable between runs. An
        unmeasured axis contributes nothing and is still paid for, so the
        composite is a floor on quality that only ever moves downward.
        """
        weighted_sum = 0.0

        if "geval" in measured_axes:
            geval_normalized = (metrics.geval_overall - 1) / 4
            weighted_sum += self._weights.geval * geval_normalized

        if "bertscore" in measured_axes:
            weighted_sum += self._weights.bertscore * metrics.bertscore_f1

        if "faithfulness" in measured_axes:
            weighted_sum += self._weights.faithfulness * metrics.faithfulness_score

        if "rouge" in measured_axes:
            weighted_sum += self._weights.rouge_l * metrics.rouge_l_f1

        # EvaluatorWeights.validate_sum keeps this at 1.0 ± 0.001.
        total_weight = (
            self._weights.geval
            + self._weights.bertscore
            + self._weights.faithfulness
            + self._weights.rouge_l
        )
        return weighted_sum / total_weight

    def _count_geval_breaches(self, metrics: SummaryMetrics) -> tuple[int, int]:
        """(critical, warn) counts across the four G-Eval dimensions."""
        critical_count = 0
        warn_count = 0

        for dim in ["coherence", "consistency", "fluency", "relevance"]:
            warn = self._thresholds.get_warn(f"geval_{dim}")
            critical = self._thresholds.get_critical(f"geval_{dim}")
            value = getattr(metrics, dim)

            if critical is not None and value < critical:
                critical_count += 1
            elif warn is not None and value < warn:
                warn_count += 1

        return critical_count, warn_count

    def _determine_alert_level(
        self, metrics: SummaryMetrics, measured_axes: frozenset[str]
    ) -> AlertLevel:
        # measured_axes is the only explicit record of whether an axis ran.
        # The G-Eval axes and the composite sit at 0.0 both when nothing was
        # measured and when the judge scored the batch at rock bottom, so
        # measurement is decided here and never inferred from a value.
        geval_measured = "geval" in measured_axes

        if metrics.sample_count > 0 and not geval_measured:
            logger.error(
                "geval_produced_no_measurements",
                sample_count=metrics.sample_count,
            )
            return AlertLevel.CRITICAL

        critical_count = 0
        warn_count = 0

        # An axis that never ran leaves its metrics at 0.0, so the composite
        # is a floor rather than a measurement — one outage already makes the
        # run's quality verdict unreliable, two make it worthless. Counting
        # them as failed dimensions is what keeps a silent evaluator outage
        # from being reported as healthy summaries.
        missing_axes = sorted(_COMPOSITE_AXES - measured_axes)
        if metrics.sample_count > 0 and missing_axes:
            logger.error(
                "composite_axes_unmeasured",
                missing_axes=missing_axes,
                sample_count=metrics.sample_count,
            )
            critical_count += len(missing_axes)

        if geval_measured:
            geval_critical, geval_warn = self._count_geval_breaches(metrics)
            critical_count += geval_critical
            warn_count += geval_warn

        if metrics.hallucination_rate > 0.5:
            critical_count += 1
        elif metrics.hallucination_rate > 0.3:
            warn_count += 1

        # Only judge the composite when every weighted axis fed it; an
        # incomplete composite is already counted above and reading it as a
        # quality signal would just double-count the same outage.
        if geval_measured and not missing_axes:
            if metrics.overall_quality_score < 0.3:
                critical_count += 1
            elif metrics.overall_quality_score < 0.5:
                warn_count += 1

        if critical_count >= 2:
            return AlertLevel.CRITICAL
        if critical_count >= 1 or warn_count >= 2:
            return AlertLevel.WARN
        return AlertLevel.OK
