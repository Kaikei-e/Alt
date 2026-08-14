"""The LLM curation path must honour max_evidence (finding 044).

A small model happily echoes back every candidate ID it was shown. Without a
cap the whole section pool flows into hydrate/compress/quote_selector, so the
success path needs the same ``[:max_evidence]`` guard the parse-failure path
already has.
"""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.curator_node import CuratorNode


class EchoAllLLM:
    """Returns every candidate ID it was shown, in the given relevance order."""

    def __init__(self, selected_ids: list[str]) -> None:
        self._selected_ids = selected_ids

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        return LLMResponse(text=json.dumps(self._selected_ids), model="fake")


def _evidence(count: int, section_key: str = "market") -> list[dict]:
    return [
        {
            "type": "article",
            "id": f"art-{i}",
            "title": f"A{i}",
            "score": 1.0 - i / 100,
            "section_keys": [section_key],
        }
        for i in range(count)
    ]


@pytest.mark.asyncio
async def test_curator_caps_llm_selection_at_max_evidence() -> None:
    """A model that selects everything must still yield at most max_evidence items."""
    evidence = _evidence(12)
    llm = EchoAllLLM([e["id"] for e in evidence])
    node = CuratorNode(llm, max_evidence=3)

    result = await node(
        {
            "evidence": evidence,
            "outline": [{"key": "market", "title": "Market"}],
            "brief": {"topic": "AI"},
        }
    )

    assert len(result["curated_by_section"]["market"]) == 3
    assert len(result["curated"]) == 3


@pytest.mark.asyncio
async def test_curator_keeps_llm_relevance_order_when_truncating() -> None:
    """Truncation must keep the model's ranking, not the pool order."""
    evidence = _evidence(6)
    # Model ranks the tail of the pool as the most relevant.
    llm = EchoAllLLM(["art-5", "art-4", "art-0", "art-1", "art-2", "art-3"])
    node = CuratorNode(llm, max_evidence=2)

    result = await node(
        {
            "evidence": evidence,
            "outline": [{"key": "market", "title": "Market"}],
            "brief": {"topic": "AI"},
        }
    )

    assert [e["id"] for e in result["curated_by_section"]["market"]] == ["art-5", "art-4"]


@pytest.mark.asyncio
async def test_curator_ignores_hallucinated_and_duplicate_ids() -> None:
    """IDs the model invented or repeated must not consume a slot twice."""
    evidence = _evidence(4)
    llm = EchoAllLLM(["art-9000", "art-2", "art-2", "art-0"])
    node = CuratorNode(llm, max_evidence=3)

    result = await node(
        {
            "evidence": evidence,
            "outline": [{"key": "market", "title": "Market"}],
            "brief": {"topic": "AI"},
        }
    )

    assert [e["id"] for e in result["curated_by_section"]["market"]] == ["art-2", "art-0"]
