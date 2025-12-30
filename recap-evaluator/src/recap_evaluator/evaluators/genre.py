"""Genre classification evaluator."""

from uuid import UUID

import httpx
import structlog

from recap_evaluator.config import alert_thresholds, settings
from recap_evaluator.domain.models import (
    AlertLevel,
    GenreEvaluationResult,
    GenreMetrics,
)
from recap_evaluator.infra.database import db

logger = structlog.get_logger()


class GenreEvaluator:
    """Evaluates genre classification quality."""

    def __init__(self, recap_worker_url: str | None = None) -> None:
        self.recap_worker_url = recap_worker_url or settings.recap_worker_url

    async def trigger_evaluation(self) -> dict | None:
        """Trigger genre evaluation on recap-worker API.

        Returns the evaluation run metadata if successful.
        """
        try:
            async with httpx.AsyncClient(timeout=120) as client:
                response = await client.post(
                    f"{self.recap_worker_url}/v1/evaluation/genres",
                )
                response.raise_for_status()
                return response.json()
        except httpx.HTTPStatusError as e:
            logger.error(
                "Failed to trigger genre evaluation",
                status_code=e.response.status_code,
            )
            return None
        except Exception as e:
            logger.error("Genre evaluation request failed", error=str(e))
            return None

    async def fetch_latest_evaluation(self) -> GenreEvaluationResult | None:
        """Fetch the latest genre evaluation from recap-worker."""
        try:
            async with httpx.AsyncClient(timeout=30) as client:
                response = await client.get(
                    f"{self.recap_worker_url}/v1/evaluation/genres/latest",
                )
                response.raise_for_status()
                data = response.json()

                return self._parse_evaluation_response(data)
        except httpx.HTTPStatusError as e:
            logger.error(
                "Failed to fetch genre evaluation",
                status_code=e.response.status_code,
            )
            return None
        except Exception as e:
            logger.error("Genre evaluation fetch failed", error=str(e))
            return None

    async def fetch_evaluation_by_id(
        self,
        run_id: str,
    ) -> GenreEvaluationResult | None:
        """Fetch a specific genre evaluation by run ID."""
        try:
            async with httpx.AsyncClient(timeout=30) as client:
                response = await client.get(
                    f"{self.recap_worker_url}/v1/evaluation/genres/{run_id}",
                )
                response.raise_for_status()
                data = response.json()

                return self._parse_evaluation_response(data)
        except httpx.HTTPStatusError as e:
            logger.error(
                "Failed to fetch genre evaluation",
                run_id=run_id,
                status_code=e.response.status_code,
            )
            return None
        except Exception as e:
            logger.error("Genre evaluation fetch failed", run_id=run_id, error=str(e))
            return None

    def _parse_evaluation_response(self, data: dict) -> GenreEvaluationResult:
        """Parse the evaluation response from recap-worker."""
        # Extract per-genre metrics
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

        # Set alert level based on macro F1
        threshold = alert_thresholds.get_threshold("genre_macro_f1")
        if threshold:
            if result.macro_f1 < threshold.critical:
                result.alert_level = AlertLevel.CRITICAL
            elif result.macro_f1 < threshold.warn:
                result.alert_level = AlertLevel.WARN
            else:
                result.alert_level = AlertLevel.OK

        return result

    async def analyze_learning_results(
        self,
        job_ids: list[UUID],
    ) -> dict:
        """Analyze genre classification learning results from database."""
        all_results = []

        for job_id in job_ids:
            learning_results = await db.fetch_genre_learning_results(job_id)
            all_results.extend(learning_results)

        if not all_results:
            return {"total_articles": 0}

        # Analyze coarse vs refine decisions
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


# Singleton instance
genre_evaluator = GenreEvaluator()
