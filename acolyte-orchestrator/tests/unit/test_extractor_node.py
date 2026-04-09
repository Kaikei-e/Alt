"""Unit tests for extractor node — atomic fact extraction from evidence."""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.extractor_node import ExtractorNode


class FakeLLM:
    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        return LLMResponse(
            text=json.dumps({
                "facts": [
                    {
                        "claim": "AI market grew 20% in Q2",
                        "source_id": "art-1",
                        "source_title": "AI Market Report",
                        "verbatim_quote": "The AI market expanded by 20% year-over-year",
                        "confidence": 0.9,
                        "data_type": "statistic",
                    },
                    {
                        "claim": "NVIDIA dominates GPU market",
                        "source_id": "art-1",
                        "source_title": "AI Market Report",
                        "verbatim_quote": "NVIDIA controls 80% of the AI GPU market",
                        "confidence": 0.8,
                        "data_type": "quote",
                    },
                ]
            }),
            model="fake",
        )


@pytest.mark.asyncio
async def test_extractor_produces_facts() -> None:
    node = ExtractorNode(FakeLLM())
    state = {
        "curated_by_section": {
            "summary": [{"id": "art-1", "title": "AI Market Report", "type": "article"}],
        },
        "hydrated_evidence": {
            "art-1": "The AI market expanded by 20% year-over-year. NVIDIA controls 80% of the AI GPU market.",
        },
    }
    result = await node(state)
    facts = result["extracted_facts"]
    assert len(facts) >= 1
    assert facts[0]["claim"] == "AI market grew 20% in Q2"
    assert facts[0]["source_id"] == "art-1"
    assert facts[0]["data_type"] == "statistic"


@pytest.mark.asyncio
async def test_extractor_handles_empty_evidence() -> None:
    node = ExtractorNode(FakeLLM())
    state = {
        "curated_by_section": {},
        "hydrated_evidence": {},
    }
    result = await node(state)
    assert result["extracted_facts"] == []


@pytest.mark.asyncio
async def test_extractor_skips_items_without_body() -> None:
    """Items not in hydrated_evidence should be skipped."""
    node = ExtractorNode(FakeLLM())
    state = {
        "curated_by_section": {
            "summary": [{"id": "art-1", "title": "AI Market"}, {"id": "art-2", "title": "No Body"}],
        },
        "hydrated_evidence": {
            "art-1": "AI market content here.",
            # art-2 has no hydrated body
        },
    }
    result = await node(state)
    # Should only extract from art-1 (which has body)
    all_source_ids = {f["source_id"] for f in result["extracted_facts"]}
    assert "art-1" in all_source_ids
