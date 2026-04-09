"""Eval runner — multi-protocol evaluation orchestrator.

Combines checklist (rule-based) + rubric (LLM) evaluations
into a single EvalResult.
"""

from __future__ import annotations

from datetime import UTC, datetime
from typing import TYPE_CHECKING

from acolyte.domain.eval import EvalDimension, EvalResult
from acolyte.usecase.eval.checklist_evaluator import ChecklistEvaluator

if TYPE_CHECKING:
    from acolyte.usecase.eval.rubric_evaluator import RubricEvaluator


class EvalRunner:
    """Multi-protocol evaluation runner."""

    def __init__(
        self,
        checklist: ChecklistEvaluator | None = None,
        rubric: RubricEvaluator | None = None,
    ) -> None:
        self._checklist = checklist or ChecklistEvaluator()
        self._rubric = rubric

    async def evaluate(
        self,
        report_id: str,
        run_id: str,
        sections: dict[str, str],
        evidence: list[dict],
        scope: dict,
        outline: list[dict],
    ) -> EvalResult:
        dimensions: list[EvalDimension] = []

        # Checklist protocol (rule-based)
        cl_result = self._checklist.evaluate(scope, outline, sections)
        dimensions.append(EvalDimension(
            name="task_fulfillment",
            score=_item_score(cl_result.items, "topic_in_content"),
            protocol="checklist",
            details={"items": [{"name": i.name, "passed": i.passed} for i in cl_result.items]},
        ))
        dimensions.append(EvalDimension(
            name="coverage",
            score=_item_score(cl_result.items, "section_"),
            protocol="checklist",
        ))
        dimensions.append(EvalDimension(
            name="presentation",
            score=_item_score(cl_result.items, "no_meta"),
            protocol="checklist",
        ))

        # Rubric protocol (LLM-based)
        if self._rubric:
            rubric_dims = await self._rubric.evaluate(sections, evidence)
            dimensions.extend(rubric_dims)

        overall = sum(d.score for d in dimensions) / len(dimensions) if dimensions else 0.0
        return EvalResult(
            report_id=report_id,
            run_id=run_id,
            dimensions=dimensions,
            overall_score=overall,
            evaluated_at=datetime.now(UTC),
        )


def _item_score(items: list, prefix: str) -> float:
    """Score for checklist items matching a name prefix."""
    matching = [i for i in items if i.name.startswith(prefix)]
    if not matching:
        return 1.0  # No items to check = pass
    return sum(1 for i in matching if i.passed) / len(matching)
