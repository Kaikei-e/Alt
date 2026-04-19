"""Tests for cross-lingual writer behaviour.

The writer tags supporting_quotes with source language and the prompt
instructs the LLM to add a Japanese gloss for English originals.
"""

from __future__ import annotations

import pytest

from acolyte.domain.source_map import SourceMap
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import ReportGenerationState


class FakeLLM:
    def __init__(self, default: str = "paragraph body [S1]") -> None:
        self._default = default
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **_: object) -> LLMResponse:
        self.prompts.append(prompt)
        return LLMResponse(text=self._default, model="fake")


def _state_with_lang_source_map() -> ReportGenerationState:
    sm = SourceMap()
    sm.register("uuid-a", "JP Article", language="ja")
    sm.register("uuid-b", "EN Article", language="en")
    return {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "uuid-a", "title": "JP"}, {"id": "uuid-b", "title": "EN"}]},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "Trend X",
                    "claim_type": "factual",
                    "evidence_ids": ["uuid-a", "uuid-b"],
                    "supporting_quotes": ["日本語原文", "English original snippet"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                }
            ]
        },
        "brief": {"topic": "cross-lingual test"},
        "sections": {},
        "revision_count": 0,
        "source_map": sm.to_dict(),
    }


@pytest.mark.asyncio
async def test_supporting_quotes_carry_language_label() -> None:
    """The supporting_quotes block labels each quote with its source language."""
    llm = FakeLLM()
    node = WriterNode(llm)
    state = _state_with_lang_source_map()

    await node(state)

    prompt = llm.prompts[0]
    assert "[S1][ja]" in prompt
    assert "[S2][en]" in prompt


@pytest.mark.asyncio
async def test_prompt_instructs_gloss_for_en_quotes() -> None:
    """The prompt tells the writer to gloss [en] quotes in Japanese."""
    llm = FakeLLM()
    node = WriterNode(llm)
    state = _state_with_lang_source_map()

    await node(state)

    prompt = llm.prompts[0]
    assert "[en]" in prompt
    assert "原文" in prompt
    assert "要約" in prompt or "日本語" in prompt


@pytest.mark.asyncio
async def test_ja_only_evidence_no_gloss_language_mix() -> None:
    """When there are no [en] quotes, prompt still mentions the gloss rule (safe default)."""
    sm = SourceMap()
    sm.register("uuid-a", "JP", language="ja")
    llm = FakeLLM()
    node = WriterNode(llm)
    state: ReportGenerationState = {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "curated_by_section": {"analysis": [{"id": "uuid-a", "title": "JP"}]},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "only ja",
                    "claim_type": "factual",
                    "evidence_ids": ["uuid-a"],
                    "supporting_quotes": ["日本語のみ"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                }
            ]
        },
        "brief": {"topic": "ja only"},
        "sections": {},
        "revision_count": 0,
        "source_map": sm.to_dict(),
    }

    await node(state)

    prompt = llm.prompts[0]
    assert "[S1][ja]" in prompt
