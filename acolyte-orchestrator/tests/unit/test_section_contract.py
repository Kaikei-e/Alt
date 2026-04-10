"""Unit tests for SectionContract domain model and ROLE_CONTRACT_DEFAULTS."""

from __future__ import annotations

from acolyte.domain.section_contract import (
    ROLE_CONTRACT_DEFAULTS,
    PlannerOutput,
    PlannerSection,
    QueryExpansionOutput,
    SectionContract,
)


def test_section_contract_defaults() -> None:
    """Construct with required fields only; assert all new fields have sensible defaults."""
    contract = SectionContract(
        key="analysis",
        title="Analysis",
        search_queries=["AI trends"],
        section_role="analysis",
    )
    assert contract.must_include_data_types == []
    assert contract.min_citations == 0
    assert contract.novelty_against == []
    assert contract.max_claims == 7
    assert contract.query_facets == []
    assert contract.synthesis_only is False


def test_section_contract_json_roundtrip() -> None:
    """model_dump / model_validate roundtrip preserves all fields."""
    original = SectionContract(
        key="conclusion",
        title="Conclusion",
        search_queries=["summary"],
        section_role="conclusion",
        must_include_data_types=["statistic"],
        min_citations=3,
        novelty_against=["analysis"],
        max_claims=5,
        synthesis_only=True,
    )
    dumped = original.model_dump()
    restored = SectionContract.model_validate(dumped)
    assert restored == original


def test_section_contract_backward_compat_from_dict() -> None:
    """Old-format dict (key/title/search_queries/section_role only) validates with defaults."""
    old_format = {
        "key": "overview",
        "title": "Overview",
        "search_queries": ["overview topic"],
        "section_role": "general",
    }
    contract = SectionContract.model_validate(old_format)
    assert contract.key == "overview"
    assert contract.synthesis_only is False
    assert contract.min_citations == 0
    assert contract.max_claims == 7


def test_role_contract_defaults_covers_all_roles() -> None:
    """All 4 section roles have entries in ROLE_CONTRACT_DEFAULTS."""
    expected_roles = {"analysis", "conclusion", "executive_summary", "general"}
    assert set(ROLE_CONTRACT_DEFAULTS.keys()) == expected_roles


def test_role_contract_conclusion_is_synthesis_only() -> None:
    """Conclusion template must have synthesis_only=True."""
    assert ROLE_CONTRACT_DEFAULTS["conclusion"]["synthesis_only"] is True


def test_role_contract_analysis_requires_statistics() -> None:
    """Analysis template must include 'statistic' in must_include_data_types."""
    assert "statistic" in ROLE_CONTRACT_DEFAULTS["analysis"]["must_include_data_types"]


def test_planner_output_reasoning_first() -> None:
    """PlannerOutput schema has 'reasoning' field (ADR-632 requirement)."""
    schema = PlannerOutput.model_json_schema()
    assert "reasoning" in schema["properties"]


def test_planner_section_has_only_llm_fields() -> None:
    """PlannerSection must have only key/title/search_queries/section_role — no contract fields."""
    fields = set(PlannerSection.model_fields.keys())
    assert fields == {"key", "title", "search_queries", "section_role"}


def test_section_contract_query_facets_accepts_dict_list() -> None:
    """query_facets field accepts list of dicts (Issue 6 structured facets)."""
    contract = SectionContract(
        key="analysis",
        title="Analysis",
        search_queries=["AI trends"],
        section_role="analysis",
        query_facets=[
            {"intent": "investigate", "raw_query": "AI trends", "must_have_terms": ["AI"]},
        ],
    )
    assert len(contract.query_facets) == 1
    assert contract.query_facets[0]["intent"] == "investigate"


def test_planner_output_schema_is_valid_ollama_format() -> None:
    """PlannerOutput schema is a valid object type usable as Ollama format parameter."""
    schema = PlannerOutput.model_json_schema()
    assert schema["type"] == "object"
    assert "reasoning" in schema["properties"]
    assert "sections" in schema["properties"]
    assert "required" in schema


def test_query_expansion_output_schema_valid() -> None:
    """QueryExpansionOutput schema has reasoning + queries properties."""
    schema = QueryExpansionOutput.model_json_schema()
    assert schema["type"] == "object"
    assert "reasoning" in schema["properties"]
    assert "queries" in schema["properties"]


def test_query_expansion_output_reasoning_defaults_empty() -> None:
    """reasoning defaults to empty string (debug-only field)."""
    output = QueryExpansionOutput(queries={"analysis": ["q"]})
    assert output.reasoning == ""
