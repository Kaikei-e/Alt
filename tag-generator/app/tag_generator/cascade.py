from __future__ import annotations

import threading
from dataclasses import dataclass
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from tag_extractor.extract import TagExtractionOutcome

logger = structlog.get_logger(__name__)


@dataclass
class CascadeConfig:
    """Configuration for the cascade controller that decides whether to refine tags."""

    confidence_threshold: float = 0.72
    min_tags_for_confident_exit: int = 5
    max_refine_ratio: float = 0.35
    inference_ms_threshold: float = 180.0
    # When there's insufficient confidence, we consider the article for refinement
    min_confidence_boost: float = 0.1


@dataclass
class CascadeDecision:
    needs_refine: bool
    reason: str
    confidence: float
    tag_count: int
    inference_ms: float
    refine_ratio: float

    def as_dict(self) -> dict[str, object]:
        return {
            "needs_refine": self.needs_refine,
            "reason": self.reason,
            "confidence": round(self.confidence, 3),
            "tag_count": self.tag_count,
            "inference_ms": round(self.inference_ms, 1),
            "refine_ratio": round(self.refine_ratio, 3),
        }


class CascadeController:
    """Decision helper implementing a Cost-Sensitive Cascade / EERO-style gating heuristic."""

    def __init__(self, config: CascadeConfig | None = None):
        self.config = config or CascadeConfig()
        self._lock = threading.Lock()
        self.total_evaluated = 0
        self.refine_candidates = 0

    def evaluate(self, outcome: TagExtractionOutcome) -> CascadeDecision:
        """
        Evaluate whether the extracted tags should be refined by downstream components.

        Args:
            outcome: TagExtractionOutcome containing confidence metrics

        Returns:
            CascadeDecision describing whether to push the article to the refine stage
        """
        with self._lock:
            self.total_evaluated += 1
            current_refine_ratio = self.refine_candidates / max(1, self.total_evaluated)
            needs_refine = False
            reason = "high_confidence_exit"

            if outcome.confidence < self.config.confidence_threshold:
                needs_refine = True
                reason = "low_confidence"
            elif outcome.tag_count < self.config.min_tags_for_confident_exit:
                needs_refine = True
                reason = "insufficient_tag_coverage"
            elif outcome.inference_ms > self.config.inference_ms_threshold:
                needs_refine = True
                reason = "slow_inference"

            if needs_refine and current_refine_ratio >= self.config.max_refine_ratio:
                needs_refine = False
                reason = "refine_ratio_budget_capped"

            if needs_refine:
                self.refine_candidates += 1

            decision = CascadeDecision(
                needs_refine=needs_refine,
                reason=reason,
                confidence=outcome.confidence,
                tag_count=outcome.tag_count,
                inference_ms=outcome.inference_ms,
                refine_ratio=self.refine_candidates / max(1, self.total_evaluated),
            )

            logger.debug(
                "Cascade decision computed",
                **decision.as_dict(),
                tag_source=outcome.model_name,
                language=outcome.language,
            )

            return decision
