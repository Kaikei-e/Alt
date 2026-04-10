"""Unit tests for per-section curator (Phase 2)."""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.curator_node import CuratorNode


class FakeLLM:
    """Returns curator selection response."""

    def __init__(self, selected_ids_per_call: list[list[str]] | None = None) -> None:
        self._selections = selected_ids_per_call or []
        self._call_idx = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        if self._call_idx < len(self._selections):
            ids = self._selections[self._call_idx]
        else:
            ids = []
        self._call_idx += 1
        return LLMResponse(text=json.dumps(ids), model="fake")


@pytest.mark.asyncio
async def test_curator_produces_curated_by_section() -> None:
    """Curator should output curated_by_section dict keyed by section key."""
    llm = FakeLLM()
    node = CuratorNode(llm, max_evidence=5)

    evidence = [
        {"type": "article", "id": "art-1", "title": "A1", "score": 0.9, "section_keys": ["market"]},
        {"type": "article", "id": "art-2", "title": "A2", "score": 0.8, "section_keys": ["tech"]},
        {"type": "article", "id": "art-3", "title": "A3", "score": 0.7, "section_keys": ["market", "tech"]},
    ]
    outline = [
        {"key": "market", "title": "Market"},
        {"key": "tech", "title": "Tech"},
    ]

    result = await node(
        {
            "evidence": evidence,
            "outline": outline,
            "brief": {"topic": "AI"},
        }
    )

    assert "curated_by_section" in result
    assert "market" in result["curated_by_section"]
    assert "tech" in result["curated_by_section"]


@pytest.mark.asyncio
async def test_curator_per_section_filters_by_section_key() -> None:
    """Each section's curated list should only contain evidence tagged with that section."""
    llm = FakeLLM()
    node = CuratorNode(llm, max_evidence=10)

    evidence = [
        {"type": "article", "id": "art-1", "title": "Market Only", "score": 0.9, "section_keys": ["market"]},
        {"type": "article", "id": "art-2", "title": "Tech Only", "score": 0.8, "section_keys": ["tech"]},
        {"type": "article", "id": "art-3", "title": "Both", "score": 0.7, "section_keys": ["market", "tech"]},
    ]
    outline = [
        {"key": "market", "title": "Market"},
        {"key": "tech", "title": "Tech"},
    ]

    result = await node(
        {
            "evidence": evidence,
            "outline": outline,
            "brief": {"topic": "AI"},
        }
    )

    market_ids = {e["id"] for e in result["curated_by_section"]["market"]}
    tech_ids = {e["id"] for e in result["curated_by_section"]["tech"]}

    assert "art-1" in market_ids  # market only
    assert "art-2" not in market_ids  # tech only
    assert "art-3" in market_ids  # both
    assert "art-2" in tech_ids
    assert "art-3" in tech_ids


@pytest.mark.asyncio
async def test_curator_llm_curation_for_large_sections() -> None:
    """When a section has more evidence than max_evidence, LLM should curate."""
    # LLM returns selected IDs for the section that exceeds limit
    llm = FakeLLM(selected_ids_per_call=[["art-1", "art-3"]])
    node = CuratorNode(llm, max_evidence=2)

    evidence = [
        {"type": "article", "id": "art-1", "title": "A1", "score": 0.9, "section_keys": ["market"]},
        {"type": "article", "id": "art-2", "title": "A2", "score": 0.8, "section_keys": ["market"]},
        {"type": "article", "id": "art-3", "title": "A3", "score": 0.7, "section_keys": ["market"]},
    ]
    outline = [{"key": "market", "title": "Market"}]

    result = await node(
        {
            "evidence": evidence,
            "outline": outline,
            "brief": {"topic": "AI"},
        }
    )

    market_curated = result["curated_by_section"]["market"]
    market_ids = {e["id"] for e in market_curated}
    assert market_ids == {"art-1", "art-3"}


@pytest.mark.asyncio
async def test_curator_backward_compat_curated_key() -> None:
    """Curator should still populate the `curated` key for backward compatibility."""
    llm = FakeLLM()
    node = CuratorNode(llm, max_evidence=10)

    evidence = [
        {"type": "article", "id": "art-1", "title": "A1", "score": 0.9, "section_keys": ["market"]},
    ]
    outline = [{"key": "market", "title": "Market"}]

    result = await node(
        {
            "evidence": evidence,
            "outline": outline,
            "brief": {"topic": "AI"},
        }
    )

    # Backward compat: curated should still be present
    assert "curated" in result
    assert len(result["curated"]) >= 1
