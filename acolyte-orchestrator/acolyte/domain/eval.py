"""Evaluation domain models for multi-protocol report quality assessment."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime


@dataclass(frozen=True)
class ChecklistItem:
    """Single binary check result."""

    name: str
    passed: bool
    detail: str = ""


@dataclass(frozen=True)
class EvalDimension:
    """Score for a single evaluation dimension."""

    name: str
    score: float  # 0.0 - 1.0
    protocol: str  # "checklist" | "rubric" | "pairwise"
    details: dict = field(default_factory=dict)


@dataclass(frozen=True)
class EvalResult:
    """Multi-protocol evaluation result for a report."""

    report_id: str
    run_id: str
    dimensions: list[EvalDimension]
    overall_score: float
    evaluated_at: datetime | None = None

    @property
    def dimension_map(self) -> dict[str, float]:
        return {d.name: d.score for d in self.dimensions}
