"""Unit tests for planner search_queries per section (Phase 2)."""

from __future__ import annotations

import json

import pytest
from pydantic import BaseModel

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.planner_node import PlannerNode


class FakeLLM:
    """Returns pre-configured structured planner output."""

    def __init__(self, response_text: str) -> None:
        self._response_text = response_text
        self.call_count = 0
        self.last_format: dict | None = None

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        self.last_format = kwargs.get("format")
        return LLMResponse(text=self._response_text, model="fake")


@pytest.mark.asyncio
async def test_planner_returns_search_queries_per_section() -> None:
    """Planner output should include search_queries for each section."""
    response = json.dumps({
        "reasoning": "Need market and tech sections",
        "sections": [
            {
                "key": "market_trends",
                "title": "Market Trends",
                "search_queries": ["AI market size 2026", "semiconductor demand forecast"],
            },
            {
                "key": "tech_advances",
                "title": "Technology Advances",
                "search_queries": ["latest AI chip architectures"],
            },
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI semiconductor supply chain"}})

    assert "outline" in result
    assert len(result["outline"]) == 2
    assert result["outline"][0]["search_queries"] == [
        "AI market size 2026",
        "semiconductor demand forecast",
    ]
    assert result["outline"][1]["search_queries"] == ["latest AI chip architectures"]


@pytest.mark.asyncio
async def test_planner_fallback_generates_generic_queries() -> None:
    """When LLM returns invalid JSON, fallback sections should have generic search_queries."""
    llm = FakeLLM("not valid json at all")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI trends"}})

    outline = result["outline"]
    assert len(outline) >= 1
    for section in outline:
        assert "search_queries" in section
        assert len(section["search_queries"]) >= 1
        # Generic queries should contain the topic
        for q in section["search_queries"]:
            assert "AI trends" in q


@pytest.mark.asyncio
async def test_planner_empty_sections_fallback_has_queries() -> None:
    """When LLM returns empty sections, fallback should still have search_queries."""
    response = json.dumps({"reasoning": "nothing", "sections": []})
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "quantum computing"}})

    outline = result["outline"]
    assert len(outline) >= 1
    assert "search_queries" in outline[0]
    assert len(outline[0]["search_queries"]) >= 1


@pytest.mark.asyncio
async def test_planner_sections_missing_queries_get_defaults() -> None:
    """Sections returned by LLM without search_queries get defaults based on topic."""
    response = json.dumps({
        "reasoning": "Minimal plan",
        "sections": [
            {"key": "overview", "title": "Overview"},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI ethics"}})

    outline = result["outline"]
    assert len(outline) == 1
    assert "search_queries" in outline[0]
    assert len(outline[0]["search_queries"]) >= 1


# --- Section contract enrichment tests ---


@pytest.mark.asyncio
async def test_planner_enriches_with_contract_defaults() -> None:
    """LLM returns old-format sections; contract fields are added from template."""
    response = json.dumps({
        "reasoning": "Need analysis and conclusion",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["AI trends"]},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "search_queries": ["summary"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI trends"}})

    analysis = result["outline"][0]
    assert "must_include_data_types" in analysis
    assert "min_citations" in analysis
    assert "max_claims" in analysis
    assert "synthesis_only" in analysis
    assert "novelty_against" in analysis


@pytest.mark.asyncio
async def test_planner_conclusion_gets_synthesis_only_true() -> None:
    """Conclusion section must have synthesis_only=True after enrichment."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["q"]},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "search_queries": ["q"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    assert conclusion["synthesis_only"] is True


@pytest.mark.asyncio
async def test_planner_conclusion_novelty_against_contains_analysis_keys() -> None:
    """Conclusion's novelty_against must contain all analysis section keys."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "market_analysis", "title": "Market Analysis", "section_role": "analysis", "search_queries": ["q"]},
            {"key": "tech_analysis", "title": "Tech Analysis", "section_role": "analysis", "search_queries": ["q"]},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "search_queries": ["q"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    assert "market_analysis" in conclusion["novelty_against"]
    assert "tech_analysis" in conclusion["novelty_against"]


@pytest.mark.asyncio
async def test_planner_es_novelty_against_contains_all_non_es_keys() -> None:
    """ES's novelty_against must contain all non-ES section keys."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "executive_summary", "title": "ES", "section_role": "executive_summary", "search_queries": ["q"]},
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["q"]},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "search_queries": ["q"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    es = next(s for s in result["outline"] if s["key"] == "executive_summary")
    assert "analysis" in es["novelty_against"]
    assert "conclusion" in es["novelty_against"]
    assert "executive_summary" not in es["novelty_against"]


@pytest.mark.asyncio
async def test_planner_analysis_gets_default_data_types() -> None:
    """Analysis section must get must_include_data_types=['statistic', 'quote'] from template."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["q"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    analysis = result["outline"][0]
    assert "statistic" in analysis["must_include_data_types"]
    assert "quote" in analysis["must_include_data_types"]


@pytest.mark.asyncio
async def test_planner_uses_generate_validated() -> None:
    """Planner must use generate_validated (format parameter is PlannerOutput schema)."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["q"]},
        ],
    })

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
    assert fmt.get("type") == "object"
    # Must include reasoning (ADR-632)
    assert "reasoning" in fmt.get("properties", {})


# --- Issue 6: Query facet enrichment tests ---


@pytest.mark.asyncio
async def test_planner_enrichment_populates_query_facets() -> None:
    """Analysis section gets non-empty query_facets after enrichment."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["AI trends"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI trends"}})

    analysis = result["outline"][0]
    assert "query_facets" in analysis
    assert len(analysis["query_facets"]) >= 1
    assert analysis["query_facets"][0]["raw_query"] == "AI trends"


@pytest.mark.asyncio
async def test_planner_synthesis_only_sections_get_empty_facets() -> None:
    """Conclusion and ES sections get empty query_facets."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["q"]},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "search_queries": ["q"]},
            {"key": "executive_summary", "title": "ES", "section_role": "executive_summary", "search_queries": ["q"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    es = next(s for s in result["outline"] if s["key"] == "executive_summary")
    assert conclusion["query_facets"] == []
    assert es["query_facets"] == []


@pytest.mark.asyncio
async def test_planner_facets_include_brief_entities() -> None:
    """When brief has entities, matching entities appear in facet."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["NVIDIA GPU market"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "GPU market", "entities": ["NVIDIA", "AMD"]}})

    facets = result["outline"][0]["query_facets"]
    assert len(facets) == 1
    assert "NVIDIA" in facets[0]["entities"]


@pytest.mark.asyncio
async def test_planner_facets_include_brief_time_range() -> None:
    """When brief has time_range, it propagates to facets."""
    response = json.dumps({
        "reasoning": "plan",
        "sections": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis", "search_queries": ["AI adoption"]},
        ],
    })
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI adoption", "time_range": "2026-Q1"}})

    facets = result["outline"][0]["query_facets"]
    assert len(facets) == 1
    assert facets[0]["time_range"] == "2026-Q1"
