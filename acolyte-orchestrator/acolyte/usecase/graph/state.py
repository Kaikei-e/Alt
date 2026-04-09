"""LangGraph state definition for report generation pipeline."""

from __future__ import annotations

from typing import TypedDict


class ReportGenerationState(TypedDict, total=False):
    """State passed between graph nodes."""

    report_id: str
    run_id: str
    scope: dict
    outline: list[dict]
    evidence: list[dict]
    curated: list[dict]
    sections: dict[str, str]  # section_key → body
    critique: dict | None
    revision_count: int
    final_version_no: int | None
    error: str | None
