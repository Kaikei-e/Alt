"""Unit tests for hydrator node — top-N body fetch from ContentStore (Phase 2)."""

from __future__ import annotations

import pytest

from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.usecase.graph.nodes.hydrator_node import HydratorNode


@pytest.mark.asyncio
async def test_hydrator_fetches_curated_bodies() -> None:
    """Hydrator should fetch body text for all curated article IDs."""
    content_store = MemoryContentStore()
    await content_store.store("art-1", "Full body of article 1.")
    await content_store.store("art-2", "Full body of article 2.")

    node = HydratorNode(content_store)
    result = await node(
        {
            "curated_by_section": {
                "market": [
                    {"type": "article", "id": "art-1", "title": "A1", "score": 0.9},
                    {"type": "article", "id": "art-2", "title": "A2", "score": 0.8},
                ],
            },
        }
    )

    assert "hydrated_evidence" in result
    assert result["hydrated_evidence"]["art-1"] == "Full body of article 1."
    assert result["hydrated_evidence"]["art-2"] == "Full body of article 2."


@pytest.mark.asyncio
async def test_hydrator_skips_missing_content() -> None:
    """Hydrator should skip articles with no content in store."""
    content_store = MemoryContentStore()
    await content_store.store("art-1", "Has body.")

    node = HydratorNode(content_store)
    result = await node(
        {
            "curated_by_section": {
                "market": [
                    {"type": "article", "id": "art-1", "title": "A1", "score": 0.9},
                    {"type": "article", "id": "art-missing", "title": "Missing", "score": 0.5},
                ],
            },
        }
    )

    assert "art-1" in result["hydrated_evidence"]
    assert "art-missing" not in result["hydrated_evidence"]


@pytest.mark.asyncio
async def test_hydrator_deduplicates_across_sections() -> None:
    """Same article_id in multiple sections should only be fetched once."""
    content_store = MemoryContentStore()
    await content_store.store("art-shared", "Shared body.")

    node = HydratorNode(content_store)
    result = await node(
        {
            "curated_by_section": {
                "market": [{"type": "article", "id": "art-shared", "title": "S", "score": 0.9}],
                "tech": [{"type": "article", "id": "art-shared", "title": "S", "score": 0.9}],
            },
        }
    )

    assert result["hydrated_evidence"]["art-shared"] == "Shared body."


@pytest.mark.asyncio
async def test_hydrator_skips_recap_type() -> None:
    """Hydrator should only fetch articles, not recaps."""
    content_store = MemoryContentStore()
    await content_store.store("art-1", "Article body.")

    node = HydratorNode(content_store)
    result = await node(
        {
            "curated_by_section": {
                "market": [
                    {"type": "article", "id": "art-1", "title": "A1", "score": 0.9},
                    {"type": "recap", "id": "recap-1", "title": "Recap", "score": 0.8},
                ],
            },
        }
    )

    assert "art-1" in result["hydrated_evidence"]
    assert "recap-1" not in result["hydrated_evidence"]


@pytest.mark.asyncio
async def test_hydrator_empty_curated_returns_empty() -> None:
    """When curated_by_section is empty, hydrated_evidence should be empty."""
    content_store = MemoryContentStore()
    node = HydratorNode(content_store)

    result = await node({"curated_by_section": {}})
    assert result["hydrated_evidence"] == {}


@pytest.mark.asyncio
async def test_hydrator_fallback_from_curated_key() -> None:
    """When curated_by_section is absent, hydrator should use curated as fallback."""
    content_store = MemoryContentStore()
    await content_store.store("art-1", "Body from curated fallback.")

    node = HydratorNode(content_store)
    result = await node(
        {
            "curated": [
                {"type": "article", "id": "art-1", "title": "A1", "score": 0.9},
            ],
        }
    )

    assert "art-1" in result["hydrated_evidence"]
    assert result["hydrated_evidence"]["art-1"] == "Body from curated fallback."
