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
