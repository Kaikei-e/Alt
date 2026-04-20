"""Planner must emit bilingual (Japanese + English) search queries.

Mirrors the news-creator expand-query contract established by ADR-000695:
the prompt template has to instruct the LLM to produce queries in both
Japanese and English so that BM25 and vector retrieval can surface
cross-lingual evidence for a Japanese report topic.
"""

from __future__ import annotations

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.planner_node import PlannerNode


class FakeLLM:
    def __init__(self, response_text: str) -> None:
        self._response_text = response_text
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **_: object) -> LLMResponse:
        self.prompts.append(prompt)
        return LLMResponse(text=self._response_text, model="fake")


@pytest.mark.asyncio
async def test_planner_prompt_requires_bilingual_queries_for_japanese_topic() -> None:
    stub_xml = "<plan><reasoning>t</reasoning><section><key>analysis</key><query>q</query></section></plan>"
    llm = FakeLLM(stub_xml)
    node = PlannerNode(llm)

    await node({"brief": {"topic": "イラン情勢について", "report_type": "weekly_briefing"}})

    assert llm.prompts, "planner must invoke LLM at least once"
    prompt = llm.prompts[0]
    assert "English" in prompt, "prompt must name English as a required query language"
    assert "Japanese" in prompt or "日本語" in prompt, "prompt must name Japanese as a required query language"


@pytest.mark.asyncio
async def test_planner_preserves_bilingual_queries_from_llm() -> None:
    xml = (
        "<plan><reasoning>bilingual</reasoning>"
        "<section><key>analysis</key>"
        "<query>イラン情勢 軍事</query>"
        "<query>Iran military tensions 2026</query>"
        "</section>"
        "</plan>"
    )
    node = PlannerNode(FakeLLM(xml))

    result = await node({"brief": {"topic": "イラン情勢", "report_type": "weekly_briefing"}})

    analysis = next(s for s in result["outline"] if s["key"] == "analysis")
    assert "イラン情勢 軍事" in analysis["search_queries"]
    assert "Iran military tensions 2026" in analysis["search_queries"]


@pytest.mark.asyncio
async def test_planner_fallback_includes_english_variant_for_japanese_topic() -> None:
    """When the LLM fails, the deterministic fallback must still include at
    least one English-transliteration variant so the Gatherer has a chance to
    surface English evidence."""
    llm = FakeLLM("not valid xml")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "イラン情勢分析", "report_type": "weekly_briefing"}})

    for section in result["outline"]:
        queries = section["search_queries"]
        assert queries, f"section {section['key']} must have at least one fallback query"
        has_ascii_word = any(any(ch.isascii() and ch.isalpha() for ch in q) for q in queries)
        assert has_ascii_word, (
            f"section {section['key']} fallback queries must include at least one ASCII/English token, got {queries!r}"
        )
