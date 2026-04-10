"""Unit tests for planner search_queries per section (Phase 2)."""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.planner_node import PlannerNode


class FakeLLM:
    """Returns pre-configured structured planner output."""

    def __init__(self, response_text: str) -> None:
        self._response_text = response_text
        self.call_count = 0
        self.last_format: object | None = None

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        self.last_format = kwargs.get("format")
        return LLMResponse(text=self._response_text, model="fake")


@pytest.mark.asyncio
async def test_planner_returns_search_queries_per_section() -> None:
    """Planner output should include search_queries for each section (skeleton + LLM queries)."""
    response = json.dumps(
        {
            "reasoning": "Need market and tech sections",
            "queries": {
                "executive_summary": ["AI semiconductor overview"],
                "analysis": ["AI market size 2026", "semiconductor demand forecast"],
                "conclusion": ["AI semiconductor outlook"],
            },
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI semiconductor supply chain"}})

    assert "outline" in result
    assert len(result["outline"]) == 3  # skeleton has 3 sections
    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert analysis["search_queries"] == ["AI market size 2026", "semiconductor demand forecast"]


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
async def test_planner_empty_queries_fallback_has_queries() -> None:
    """When LLM returns empty queries, fallback should still have search_queries."""
    response = json.dumps({"reasoning": "nothing", "queries": {}})
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "quantum computing"}})

    outline = result["outline"]
    assert len(outline) >= 1
    assert "search_queries" in outline[0]
    assert len(outline[0]["search_queries"]) >= 1


@pytest.mark.asyncio
async def test_planner_sections_missing_queries_get_defaults() -> None:
    """Sections without LLM queries get topic-based defaults from skeleton."""
    response = json.dumps(
        {
            "reasoning": "Minimal plan",
            "queries": {},  # LLM returns nothing
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI ethics"}})

    outline = result["outline"]
    assert len(outline) >= 3  # skeleton provides 3 sections
    for section in outline:
        assert "search_queries" in section
        assert len(section["search_queries"]) >= 1


# --- Section contract enrichment tests ---


@pytest.mark.asyncio
async def test_planner_enriches_with_contract_defaults() -> None:
    """Contract fields are added from template regardless of LLM output."""
    response = json.dumps(
        {
            "reasoning": "Need analysis and conclusion",
            "queries": {
                "analysis": ["AI trends"],
                "conclusion": ["summary"],
            },
        }
    )
    llm = FakeLLM(response)
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
    """Conclusion section must have synthesis_only=True after enrichment."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {
                "analysis": ["q"],
                "conclusion": ["q"],
            },
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    assert conclusion["synthesis_only"] is True


@pytest.mark.asyncio
async def test_planner_conclusion_novelty_against_contains_analysis_keys() -> None:
    """Conclusion's novelty_against must contain all analysis section keys."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {
                "executive_summary": ["q"],
                "analysis": ["q"],
                "conclusion": ["q"],
            },
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    conclusion = next(s for s in result["outline"] if s["key"] == "conclusion")
    assert "analysis" in conclusion["novelty_against"]


@pytest.mark.asyncio
async def test_planner_es_novelty_against_contains_all_non_es_keys() -> None:
    """ES's novelty_against must contain all non-ES section keys."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {
                "executive_summary": ["q"],
                "analysis": ["q"],
                "conclusion": ["q"],
            },
        }
    )
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
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {"analysis": ["q"]},
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert "statistic" in analysis["must_include_data_types"]
    assert "quote" in analysis["must_include_data_types"]


@pytest.mark.asyncio
async def test_planner_uses_generate_validated() -> None:
    """Planner must use generate_validated (format parameter is QueryExpansionOutput schema)."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {"analysis": ["q"]},
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
    assert fmt.get("type") == "object"
    # Must include reasoning (ADR-632)
    assert "reasoning" in fmt.get("properties", {})


# --- Issue 6: Query facet enrichment tests ---


@pytest.mark.asyncio
async def test_planner_enrichment_populates_query_facets() -> None:
    """Analysis section gets non-empty query_facets after enrichment."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {"analysis": ["AI trends"]},
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI trends"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert "query_facets" in analysis
    assert len(analysis["query_facets"]) >= 1


@pytest.mark.asyncio
async def test_planner_synthesis_only_sections_get_empty_facets() -> None:
    """Conclusion and ES sections get empty query_facets."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {
                "analysis": ["q"],
                "conclusion": ["q"],
                "executive_summary": ["q"],
            },
        }
    )
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
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {"analysis": ["NVIDIA GPU market"]},
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "GPU market", "entities": ["NVIDIA", "AMD"]}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    facets = analysis["query_facets"]
    assert len(facets) >= 1
    assert "NVIDIA" in facets[0]["entities"]


@pytest.mark.asyncio
async def test_planner_skeleton_used_for_known_report_type() -> None:
    """Known report_type uses fixed skeleton; LLM expands queries only."""
    response = json.dumps(
        {
            "reasoning": "Expanding queries for skeleton sections",
            "queries": {
                "executive_summary": ["AI chip market overview 2026"],
                "analysis": ["NVIDIA Blackwell GPU", "AMD MI400"],
                "conclusion": ["AI chip market outlook"],
            },
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chip trends", "report_type": "market_analysis"}})

    outline = result["outline"]
    # Must have ES, analysis, conclusion (skeleton structure)
    keys = [s["key"] for s in outline]
    assert "executive_summary" in keys
    assert "analysis" in keys
    assert "conclusion" in keys


@pytest.mark.asyncio
async def test_planner_skeleton_fallback_on_llm_failure() -> None:
    """When LLM fails, skeleton sections with topic-based queries are used."""
    llm = FakeLLM("invalid json garbage")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "quantum computing", "report_type": "weekly_briefing"}})

    outline = result["outline"]
    keys = [s["key"] for s in outline]
    assert "executive_summary" in keys
    assert "analysis" in keys
    assert "conclusion" in keys


@pytest.mark.asyncio
async def test_planner_num_predict_is_1024() -> None:
    """Planner must use num_predict=1024 (skeleton only needs query expansion)."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {"analysis": ["q"]},
        }
    )

    class CaptureLLM:
        def __init__(self) -> None:
            self.calls: list[dict] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.calls.append(kwargs)
            return LLMResponse(text=response, model="fake")

    llm = CaptureLLM()
    node = PlannerNode(llm)
    await node({"brief": {"topic": "test"}})

    assert llm.calls[0]["num_predict"] == 1024


@pytest.mark.asyncio
async def test_planner_facets_include_brief_time_range() -> None:
    """When brief has time_range, it propagates to facets."""
    response = json.dumps(
        {
            "reasoning": "plan",
            "queries": {"analysis": ["AI adoption"]},
        }
    )
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI adoption", "time_range": "2026-Q1"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    facets = analysis["query_facets"]
    assert len(facets) >= 1
    assert facets[0]["time_range"] == "2026-Q1"
