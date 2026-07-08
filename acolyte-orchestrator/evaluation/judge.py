"""Faithfulness judge facade.

``metrics.faithfulness`` passes a callable ``judge(prompt) -> float``.
This module wires the prompt builder, few-shot bank, and one of the
concrete judges (mock or Gemma4) into a single callable that the
evaluation harness can depend on.

Design:

- :class:`StrictFaithfulnessJudge` builds the rubric prompt itself and
  delegates scoring to an inner judge. This removes the responsibility
  from ``metrics.faithfulness`` (which currently hand-rolls the prompt)
  — new callers should always use this facade so the prompt cannot drift.
"""

from __future__ import annotations

from collections.abc import Callable

import structlog

from evaluation.judges.mock import MockRubricJudge
from evaluation.judges.prompt import build_judge_prompt
from evaluation.judges.shots import DEFAULT_SHOTS

logger = structlog.get_logger(__name__)


class StrictFaithfulnessJudge:
    """High-level judge that assembles the rubric prompt and scores it.

    Compatible with the ``judge(prompt: str) -> float`` signature that
    ``evaluation.metrics.faithfulness`` expects, but also exposes
    :meth:`score_case` for callers that want to skip the legacy prompt
    the metric builds.
    """

    def __init__(
        self,
        inner: Callable[[str], float | None] | None = None,
        *,
        shots: list[dict] | None = None,
        mock_score: float = 0.5,
    ) -> None:
        if inner is None:
            logger.warning(
                "StrictFaithfulnessJudge: no inner judge provided, falling back to "
                "MockRubricJudge — faithfulness scores will be a constant stub value",
                mock_score=mock_score,
            )
        self._inner = inner if inner is not None else MockRubricJudge(mock_score=mock_score)
        self._shots = shots if shots is not None else DEFAULT_SHOTS

    def __call__(self, prompt: str) -> float | None:
        """Legacy callable shape for evaluation.metrics.faithfulness."""
        return self._inner(prompt)

    def score_case(self, body: str, evidence_by_short_id: dict[str, str]) -> float | None:
        """Build the canonical rubric prompt and delegate to the inner judge."""
        prompt = build_judge_prompt(body, evidence_by_short_id, self._shots)
        return self._inner(prompt)
