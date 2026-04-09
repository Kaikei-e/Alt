"""Unit tests for section_role support in PlannerNode."""

from __future__ import annotations

from acolyte.domain.section_contract import PlannerOutput
from acolyte.usecase.graph.nodes.planner_node import (
    _default_fallback_sections,
    _infer_section_role,
)


def test_fallback_sections_include_section_role() -> None:
    """Every fallback section must have a section_role field."""
    sections = _default_fallback_sections("test topic")
    for section in sections:
        assert "section_role" in section, f"Section '{section['key']}' missing section_role"
    roles = {s["key"]: s["section_role"] for s in sections}
    assert roles["executive_summary"] == "executive_summary"
    assert roles["analysis"] == "analysis"
    assert roles["conclusion"] == "conclusion"


def test_planner_output_schema_includes_section_role() -> None:
    """PlannerOutput JSON schema used for Ollama format must include section_role in sections."""
    schema = PlannerOutput.model_json_schema()
    # Navigate: properties -> sections -> items -> properties (via $defs)
    defs = schema.get("$defs", {})
    planner_section_schema = defs.get("PlannerSection", {})
    assert "section_role" in planner_section_schema.get("properties", {})


def test_infer_section_role_from_key() -> None:
    """Heuristic role inference from section key/title."""
    assert _infer_section_role("conclusion", "Conclusion") == "conclusion"
    assert _infer_section_role("executive_summary", "Executive Summary") == "executive_summary"
    assert _infer_section_role("analysis", "Analysis") == "analysis"
    assert _infer_section_role("market_trends", "Market Trends") == "general"
    # Title-based inference when key is generic
    assert _infer_section_role("section_3", "Conclusion and Outlook") == "conclusion"
    assert _infer_section_role("overview", "Executive Summary") == "executive_summary"
