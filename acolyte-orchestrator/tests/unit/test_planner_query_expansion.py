"""Unit tests for Issue 1: Planner QueryExpansion schema simplification.

Tests that the planner uses QueryExpansionOutput (flat dict of section_key → queries)
instead of PlannerOutput (full section structure).
"""

from __future__ import annotations

import json

import pytest

from acolyte.domain.section_contract import QueryExpansionOutput
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.planner_node import PlannerNode


class FakeLLM:
    def __init__(self, response_text: str) -> None:
        self._response_text = response_text
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.calls.append({"prompt": prompt, **kwargs})
        return LLMResponse(text=self._response_text, model="fake")


# --- QueryExpansionOutput schema tests ---


def test_query_expansion_output_schema_valid() -> None:
    """QueryExpansionOutput schema must have reasoning and queries properties."""
    schema = QueryExpansionOutput.model_json_schema()
    assert schema["type"] == "object"
    assert "reasoning" in schema["properties"]
    assert "queries" in schema["properties"]


def test_query_expansion_output_reasoning_default_empty() -> None:
    """reasoning defaults to empty string (debug-only field)."""
    output = QueryExpansionOutput(queries={"analysis": ["q1"]})
    assert output.reasoning == ""


# --- Planner uses QueryExpansionOutput ---


@pytest.mark.asyncio
async def test_planner_uses_query_expansion_schema() -> None:
    """Planner must pass QueryExpansionOutput schema as format to generate_validated."""
    response = json.dumps(
        {
            "reasoning": "test",
            "queries": {"analysis": ["AI trends"]},
        }
    )

    class CaptureLLM:
        def __init__(self) -> None:
            self.formats: list = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.formats.append(kwargs.get("format"))
            return LLMResponse(text=response, model="fake")

    llm = CaptureLLM()
    node = PlannerNode(llm)
    await node({"brief": {"topic": "test"}})

    assert len(llm.formats) >= 1
    fmt = llm.formats[0]
    assert isinstance(fmt, dict)
    # Must have queries (dict), not sections (list)
    assert "queries" in fmt.get("properties", {})
    assert "sections" not in fmt.get("properties", {})


# --- Skeleton generation without LLM output ---


@pytest.mark.asyncio
async def test_planner_without_llm_output_generates_skeleton() -> None:
    """When LLM returns invalid JSON, planner still produces 3-section skeleton."""
    llm = FakeLLM("not valid json at all")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI semiconductor", "report_type": "market_analysis"}})

    outline = result["outline"]
    assert len(outline) == 3
    keys = [s["key"] for s in outline]
    assert keys == ["executive_summary", "analysis", "conclusion"]
    # Every section has search_queries
    for section in outline:
        assert "search_queries" in section
        assert len(section["search_queries"]) >= 1


@pytest.mark.asyncio
async def test_query_expansion_failure_generates_deterministic_queries() -> None:
    """When LLM returns empty queries={}, topic-based queries are generated."""
    response = json.dumps({"reasoning": "nothing useful", "queries": {}})
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "quantum computing"}})

    outline = result["outline"]
    for section in outline:
        assert len(section["search_queries"]) >= 1
        # Default queries contain the topic
        for q in section["search_queries"]:
            assert "quantum computing" in q


# --- Market analysis always 3 sections ---


@pytest.mark.asyncio
async def test_market_analysis_always_3_sections_on_success() -> None:
    """market_analysis always produces exactly 3 sections on LLM success."""
    response = json.dumps(
        {
            "reasoning": "good",
            "queries": {
                "executive_summary": ["overview"],
                "analysis": ["deep dive"],
                "conclusion": ["outlook"],
            },
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chips", "report_type": "market_analysis"}})
    assert len(result["outline"]) == 3


@pytest.mark.asyncio
async def test_market_analysis_always_3_sections_on_failure() -> None:
    """market_analysis always produces exactly 3 sections on LLM failure."""
    llm = FakeLLM("broken json")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chips", "report_type": "market_analysis"}})
    assert len(result["outline"]) == 3


# --- Query merging ---


@pytest.mark.asyncio
async def test_planner_success_merges_queries() -> None:
    """LLM queries are merged into skeleton sections."""
    response = json.dumps(
        {
            "reasoning": "expanding",
            "queries": {
                "analysis": ["NVIDIA GPU market", "AMD MI400"],
            },
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "GPU market"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert analysis["search_queries"] == ["NVIDIA GPU market", "AMD MI400"]


# --- Fallback structure identical to success ---


@pytest.mark.asyncio
async def test_fallback_identical_structure_to_success() -> None:
    """Fallback and success outlines must have identical structure (keys, roles, contracts)."""
    # Success path
    success_response = json.dumps(
        {
            "reasoning": "good",
            "queries": {
                "executive_summary": ["overview"],
                "analysis": ["analysis query"],
                "conclusion": ["conclusion query"],
            },
        }
    )
    success_llm = FakeLLM(success_response)
    success_node = PlannerNode(success_llm)
    success_result = await success_node({"brief": {"topic": "test", "report_type": "market_analysis"}})

    # Failure path
    fail_llm = FakeLLM("invalid json")
    fail_node = PlannerNode(fail_llm)
    fail_result = await fail_node({"brief": {"topic": "test", "report_type": "market_analysis"}})

    success_outline = success_result["outline"]
    fail_outline = fail_result["outline"]

    # Same number of sections
    assert len(success_outline) == len(fail_outline)

    # Same keys, roles, and contract fields (search_queries may differ)
    for s, f in zip(success_outline, fail_outline, strict=True):
        assert s["key"] == f["key"]
        assert s["section_role"] == f["section_role"]
        assert s.get("synthesis_only") == f.get("synthesis_only")
        assert s.get("min_citations") == f.get("min_citations")
        assert s.get("max_claims") == f.get("max_claims")
        assert s.get("must_include_data_types") == f.get("must_include_data_types")
