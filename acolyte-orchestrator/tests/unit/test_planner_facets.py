"""Unit tests for planner search_queries per section and query facet enrichment."""

from __future__ import annotations

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.planner_node import PlannerNode


def _xml_plan(queries: dict[str, list[str]], reasoning: str = "plan") -> str:
    """Build XML plan response for testing."""
    sections = "\n".join(
        f"  <section><key>{k}</key>" + "".join(f"<query>{q}</query>" for q in v) + "</section>"
        for k, v in queries.items()
    )
    return f"<plan><reasoning>{reasoning}</reasoning>\n{sections}\n</plan>"


class FakeLLM:
    def __init__(self, response_text: str) -> None:
        self._response_text = response_text
        self.call_count = 0
        self.last_kwargs: dict = {}

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        self.last_kwargs = kwargs
        return LLMResponse(text=self._response_text, model="fake")


@pytest.mark.asyncio
async def test_planner_returns_search_queries_per_section() -> None:
    xml = _xml_plan(
        {
            "executive_summary": ["AI semiconductor overview"],
            "analysis": ["AI market size 2026", "semiconductor demand forecast"],
            "conclusion": ["AI semiconductor outlook"],
        }
    )
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "AI semiconductor supply chain"}})

    assert len(result["outline"]) == 3
    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert analysis["search_queries"] == ["AI market size 2026", "semiconductor demand forecast"]


@pytest.mark.asyncio
async def test_planner_fallback_generates_generic_queries() -> None:
    llm = FakeLLM("not valid xml at all")
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "AI trends"}})

    for section in result["outline"]:
        assert len(section["search_queries"]) >= 1
        for q in section["search_queries"]:
            assert "AI trends" in q


@pytest.mark.asyncio
async def test_planner_empty_queries_fallback_has_queries() -> None:
    llm = FakeLLM("<plan><reasoning>nothing</reasoning></plan>")
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "quantum computing"}})

    assert len(result["outline"][0]["search_queries"]) >= 1


@pytest.mark.asyncio
async def test_planner_sections_missing_queries_get_defaults() -> None:
    llm = FakeLLM("<plan><reasoning>minimal</reasoning></plan>")
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "AI ethics"}})

    for section in result["outline"]:
        assert len(section["search_queries"]) >= 1


@pytest.mark.asyncio
async def test_planner_enriches_with_contract_defaults() -> None:
    xml = _xml_plan({"analysis": ["AI trends"], "conclusion": ["summary"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "AI trends"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert "must_include_data_types" in analysis
    assert "min_citations" in analysis
    assert "max_claims" in analysis
    assert "synthesis_only" in analysis
    assert "novelty_against" in analysis


@pytest.mark.asyncio
async def test_planner_conclusion_gets_synthesis_only_true() -> None:
    xml = _xml_plan({"analysis": ["q"], "conclusion": ["q"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    assert conclusion["synthesis_only"] is True


@pytest.mark.asyncio
async def test_planner_conclusion_novelty_against_contains_analysis_keys() -> None:
    xml = _xml_plan({"executive_summary": ["q"], "analysis": ["q"], "conclusion": ["q"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    assert "analysis" in conclusion["novelty_against"]


@pytest.mark.asyncio
async def test_planner_es_novelty_against_contains_all_non_es_keys() -> None:
    xml = _xml_plan({"executive_summary": ["q"], "analysis": ["q"], "conclusion": ["q"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "test"}})

    es = next(s for s in result["outline"] if s["key"] == "executive_summary")
    assert "analysis" in es["novelty_against"]
    assert "conclusion" in es["novelty_against"]
    assert "executive_summary" not in es["novelty_against"]


@pytest.mark.asyncio
async def test_planner_analysis_gets_default_data_types() -> None:
    xml = _xml_plan({"analysis": ["q"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "test"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert "statistic" in analysis["must_include_data_types"]
    assert "quote" in analysis["must_include_data_types"]


@pytest.mark.asyncio
async def test_planner_no_format_in_llm_kwargs() -> None:
    """Planner must NOT pass format kwarg (XML DSL mode)."""
    xml = _xml_plan({"analysis": ["q"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    await node({"brief": {"topic": "test"}})

    assert "format" not in llm.last_kwargs


@pytest.mark.asyncio
async def test_planner_enrichment_populates_query_facets() -> None:
    xml = _xml_plan({"analysis": ["AI trends"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "AI trends"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert "query_facets" in analysis
    assert len(analysis["query_facets"]) >= 1


@pytest.mark.asyncio
async def test_planner_synthesis_only_sections_get_empty_facets() -> None:
    xml = _xml_plan({"analysis": ["q"], "conclusion": ["q"], "executive_summary": ["q"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    es = next(s for s in result["outline"] if s["key"] == "executive_summary")
    assert conclusion["query_facets"] == []
    assert es["query_facets"] == []


@pytest.mark.asyncio
async def test_planner_facets_include_brief_entities() -> None:
    xml = _xml_plan({"analysis": ["NVIDIA GPU market"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "GPU market", "entities": ["NVIDIA", "AMD"]}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    facets = analysis["query_facets"]
    assert len(facets) >= 1
    assert "NVIDIA" in facets[0]["entities"]


@pytest.mark.asyncio
async def test_planner_skeleton_used_for_known_report_type() -> None:
    xml = _xml_plan({"analysis": ["q"]})
    llm = FakeLLM(xml)
    node = PlannerNode(llm)
    result = await node({"brief": {"topic": "test", "report_type": "market_analysis"}})

    keys = [s["key"] for s in result["outline"]]
    assert keys == ["executive_summary", "analysis", "conclusion"]
