"""LangGraph state definition for report generation pipeline."""

from __future__ import annotations

from typing import TypedDict


class ReportGenerationState(TypedDict, total=False):
    """State passed between graph nodes."""

    report_id: str
    run_id: str
    brief: dict  # ReportBrief.to_dict() — typed input specification
    scope: dict  # deprecated, kept for backward compat during migration
    outline: list[dict]
    evidence: list[dict]
    curated: list[dict]
    curated_by_section: dict[str, list[dict]]  # section_key → curated evidence
    hydrated_evidence: dict[str, str]  # article_id → body text
    extracted_facts: list[dict]  # ExtractedFact dicts from extractor
    section_citations: dict[str, list[dict]]  # section_key → citation objects
    sections: dict[str, str]  # section_key → body
    critique: dict | None
    failure_modes: list[dict]  # GroUSE failure mode detections
    revision_count: int
    final_version_no: int | None
    error: str | None
