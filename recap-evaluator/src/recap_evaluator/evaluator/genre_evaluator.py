"""Genre classification evaluator with DI."""

from uuid import UUID

import structlog

from recap_evaluator.config import AlertThresholds
from recap_evaluator.domain.models import (
    AlertLevel,
    GenreEvaluationResult,
    GenreMetrics,
)
from recap_evaluator.port.database_port import DatabasePort
from recap_evaluator.port.recap_worker_port import RecapWorkerPort

logger = structlog.get_logger()


class GenreEvaluator:
    """Evaluates genre classification quality via recap-worker API."""

    def __init__(
        self,
        recap_worker: RecapWorkerPort,
        db: DatabasePort,
        thresholds: AlertThresholds,
    ) -> None:
        self._recap_worker = recap_worker
        self._db = db
        self._thresholds = thresholds

    async def trigger_evaluation(self) -> dict | None:
        return await self._recap_worker.trigger_genre_evaluation()

    async def fetch_latest_evaluation(self) -> GenreEvaluationResult | None:
        data = await self._recap_worker.fetch_latest_genre_evaluation()
        if not data:
            return None
        return self._parse_evaluation_response(data)

    async def fetch_evaluation_by_id(
        self, run_id: str
    ) -> GenreEvaluationResult | None:
        data = await self._recap_worker.fetch_genre_evaluation_by_id(run_id)
        if not data:
            return None
        return self._parse_evaluation_response(data)

    def _parse_evaluation_response(self, data: dict) -> GenreEvaluationResult:
        per_genre = []
        for genre_data in data.get("per_genre_metrics", []):
            per_genre.append(
                GenreMetrics(
                    genre=genre_data.get("genre", ""),
                    tp=genre_data.get("tp", 0),
                    fp=genre_data.get("fp", 0),
                    fn=genre_data.get("fn_count", 0),
                    precision=genre_data.get("precision", 0.0),
                    recall=genre_data.get("recall", 0.0),
                    f1_score=genre_data.get("f1_score", 0.0),
                    support=genre_data.get("tp", 0) + genre_data.get("fn_count", 0),
                )
            )

        result = GenreEvaluationResult(
            macro_precision=data.get("macro_precision", 0.0),
            macro_recall=data.get("macro_recall", 0.0),
            macro_f1=data.get("macro_f1", 0.0),
            micro_precision=data.get("micro_precision", 0.0),
            micro_recall=data.get("micro_recall", 0.0),
            micro_f1=data.get("micro_f1", 0.0),
            weighted_f1=data.get("weighted_f1", 0.0),
            per_genre_metrics=per_genre,
            total_samples=data.get("total_items", 0),
        )

        warn = self._thresholds.get_warn("genre_macro_f1")
        critical = self._thresholds.get_critical("genre_macro_f1")
        if critical is not None and result.macro_f1 < critical:
            result.alert_level = AlertLevel.CRITICAL
        elif warn is not None and result.macro_f1 < warn:
            result.alert_level = AlertLevel.WARN
        else:
            result.alert_level = AlertLevel.OK

        return result

    async def analyze_learning_results(self, job_ids: list[UUID]) -> dict:
        all_results = []
        for job_id in job_ids:
            learning_results = await self._db.fetch_genre_learning_results(job_id)
            all_results.extend(learning_results)

        if not all_results:
            return {"total_articles": 0}

        coarse_only = 0
        refined = 0
        high_confidence = 0
        low_confidence = 0

        for result in all_results:
            refine_decision = result.get("refine_decision", {})
            if not refine_decision:
                coarse_only += 1
                continue

            strategy = refine_decision.get("strategy", "")
            confidence = refine_decision.get("confidence", 0.0)

            if strategy in ["coarse_high_confidence", "coarse_only"]:
                coarse_only += 1
            else:
                refined += 1

            if confidence >= 0.7:
                high_confidence += 1
            else:
                low_confidence += 1

        total = len(all_results)
        return {
            "total_articles": total,
            "coarse_only_count": coarse_only,
            "refined_count": refined,
            "coarse_only_rate": coarse_only / total if total > 0 else 0.0,
            "refined_rate": refined / total if total > 0 else 0.0,
            "high_confidence_count": high_confidence,
            "low_confidence_count": low_confidence,
            "high_confidence_rate": high_confidence / total if total > 0 else 0.0,
        }
