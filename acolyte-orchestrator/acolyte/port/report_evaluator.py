"""Report evaluator port — interface for multi-protocol report quality assessment."""

from __future__ import annotations

from typing import Protocol

from acolyte.domain.eval import EvalResult


class ReportEvaluatorPort(Protocol):
    async def evaluate(
        self,
        sections: dict[str, str],
        evidence: list[dict],
        scope: dict,
        outline: list[dict],
    ) -> EvalResult: ...
