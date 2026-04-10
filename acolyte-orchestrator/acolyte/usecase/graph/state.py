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
    compressed_evidence: dict[str, list[dict]]  # article_id → CompressedSpan dicts
    quote_selector_work_items: list[dict]  # checkpoint-safe per-article queue
    quote_selector_cursor: int  # current position in quote_selector_work_items
    selected_quotes: list[dict]  # SelectedQuote dicts from QuoteSelectorNode
    fact_normalizer_work_quotes: list[dict]  # checkpoint-safe per-quote queue
    fact_normalizer_cursor: int  # current position in fact_normalizer_work_quotes
    extracted_facts: list[dict]  # ExtractedFact dicts from FactNormalizerNode
    claim_plans: dict[str, list[dict]]  # section_key → PlannedClaim dicts
    section_citations: dict[str, list[dict]]  # section_key → citation objects
    sections: dict[str, str]  # section_key → body
    critique: dict | None
    critic_revision_no: int  # monotonic marker to keep revision loops progressing
    failure_modes: list[dict]  # GroUSE failure mode detections
    weak_facets: list[dict]  # Facets with hit_count < threshold, for future query rewrite
    retrieval_debug: dict  # Per-facet variant hit counts for debugging (Issue 7)
    revision_count: int
    section_paragraphs: dict[str, list[dict]]  # section_key → GeneratedParagraph dicts
    best_sections: dict[str, str]  # section_key → best non-empty, non-blocking body
    best_section_metrics: dict[str, dict]  # section_key → {"blocking_count": int, "char_len": int}
    claim_feedbacks: dict[str, list[dict]]  # section_key → [{"claim_id", "action", "reason"}]
    accepted_claims_by_section: dict[str, list[dict]]  # section_key → accepted claims used for synthesis/debug
    final_version_no: int | None
    error: str | None
