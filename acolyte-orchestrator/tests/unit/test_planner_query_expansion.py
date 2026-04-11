"""Unit tests for Planner QueryExpansion with XML DSL.

Tests that the planner uses QueryExpansionOutput (flat dict of section_key → queries)
parsed from XML DSL output.
"""

from __future__ import annotations

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


# --- Planner uses XML DSL (no format kwarg) ---


@pytest.mark.asyncio
async def test_planner_no_format_passed_to_llm() -> None:
    """Planner must NOT pass format kwarg to generate (XML DSL mode)."""
    xml = "<plan><reasoning>test</reasoning><section><key>analysis</key><query>AI trends</query></section></plan>"

    class CaptureLLM:
        def __init__(self) -> None:
            self.kwargs_list: list[dict] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.kwargs_list.append(kwargs)
            return LLMResponse(text=xml, model="fake")

    llm = CaptureLLM()
    node = PlannerNode(llm)
    await node({"brief": {"topic": "test"}})

    assert len(llm.kwargs_list) >= 1
    assert "format" not in llm.kwargs_list[0]


# --- Skeleton generation without LLM output ---


@pytest.mark.asyncio
async def test_planner_without_llm_output_generates_skeleton() -> None:
    """When LLM returns invalid output, planner still produces 3-section skeleton."""
    llm = FakeLLM("not valid xml at all")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI semiconductor", "report_type": "market_analysis"}})

    outline = result["outline"]
    assert len(outline) == 3
    keys = [s["key"] for s in outline]
    assert keys == ["executive_summary", "analysis", "conclusion"]
    for section in outline:
        assert "search_queries" in section
        assert len(section["search_queries"]) >= 1


@pytest.mark.asyncio
async def test_query_expansion_failure_generates_deterministic_queries() -> None:
    """When LLM returns empty queries, topic-based queries are generated."""
    xml = "<plan><reasoning>nothing useful</reasoning></plan>"
    llm = FakeLLM(xml)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "quantum computing"}})

    outline = result["outline"]
    for section in outline:
        assert len(section["search_queries"]) >= 1
        for q in section["search_queries"]:
            assert "quantum computing" in q


# --- Market analysis always 3 sections ---


@pytest.mark.asyncio
async def test_market_analysis_always_3_sections_on_success() -> None:
    """market_analysis always produces exactly 3 sections on LLM success."""
    xml = """<plan><reasoning>good</reasoning>
      <section><key>executive_summary</key><query>overview</query></section>
      <section><key>analysis</key><query>deep dive</query></section>
      <section><key>conclusion</key><query>outlook</query></section>
    </plan>"""
    llm = FakeLLM(xml)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chips", "report_type": "market_analysis"}})
    assert len(result["outline"]) == 3


@pytest.mark.asyncio
async def test_market_analysis_always_3_sections_on_failure() -> None:
    """market_analysis always produces exactly 3 sections on LLM failure."""
    llm = FakeLLM("broken output")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chips", "report_type": "market_analysis"}})
    assert len(result["outline"]) == 3


# --- Query merging ---


@pytest.mark.asyncio
async def test_planner_success_merges_queries() -> None:
    """LLM queries are merged into skeleton sections."""
    xml = """<plan><reasoning>expanding</reasoning>
      <section><key>analysis</key><query>NVIDIA GPU market</query><query>AMD MI400</query></section>
    </plan>"""
    llm = FakeLLM(xml)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "GPU market"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert analysis["search_queries"] == ["NVIDIA GPU market", "AMD MI400"]


# --- Fallback structure identical to success ---


@pytest.mark.asyncio
async def test_fallback_identical_structure_to_success() -> None:
    """Fallback and success outlines must have identical structure (keys, roles, contracts)."""
    success_xml = """<plan><reasoning>good</reasoning>
      <section><key>executive_summary</key><query>overview</query></section>
      <section><key>analysis</key><query>analysis query</query></section>
      <section><key>conclusion</key><query>conclusion query</query></section>
    </plan>"""
    success_llm = FakeLLM(success_xml)
    success_node = PlannerNode(success_llm)
    success_result = await success_node({"brief": {"topic": "test", "report_type": "market_analysis"}})

    fail_llm = FakeLLM("invalid output")
    fail_node = PlannerNode(fail_llm)
    fail_result = await fail_node({"brief": {"topic": "test", "report_type": "market_analysis"}})

    success_outline = success_result["outline"]
    fail_outline = fail_result["outline"]

    assert len(success_outline) == len(fail_outline)

    for s, f in zip(success_outline, fail_outline, strict=True):
        assert s["key"] == f["key"]
        assert s["section_role"] == f["section_role"]
        assert s.get("synthesis_only") == f.get("synthesis_only")
        assert s.get("min_citations") == f.get("min_citations")
        assert s.get("max_claims") == f.get("max_claims")
        assert s.get("must_include_data_types") == f.get("must_include_data_types")
