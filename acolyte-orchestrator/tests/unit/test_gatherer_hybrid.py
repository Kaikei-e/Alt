"""Unit tests for hybrid retrieval (multi-query + RRF fusion) in GathererNode.

TDD: Tests for multi-query variant retrieval and fusion integration.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest

from acolyte.domain.fusion import RRFFusion, ScoredHit
from acolyte.port.evidence_provider import ArticleHit, RecapHit
from acolyte.usecase.graph.nodes.gatherer_node import GathererNode


def _article_hit(article_id: str, title: str = "", score: float = 0.0) -> ArticleHit:
    return ArticleHit(article_id=article_id, title=title or f"Article {article_id}", score=score)


def _make_evidence(*, fusion: RRFFusion | None = None) -> tuple[AsyncMock, GathererNode]:
    evidence = AsyncMock()
    evidence.search_articles = AsyncMock(return_value=[])
    evidence.search_recaps = AsyncMock(return_value=[])
    node = GathererNode(evidence, fusion=fusion or RRFFusion())
    return evidence, node


@pytest.mark.asyncio
async def test_multi_query_calls_variants() -> None:
    """Faceted search should call search_articles multiple times per facet (once per variant)."""
    evidence, node = _make_evidence()
    evidence.search_articles = AsyncMock(
        return_value=[_article_hit("a1", score=0.8)]
    )

    state = {
        "brief": {"topic": "AI market", "entities": ["OpenAI"]},
        "outline": [
            {
                "key": "analysis",
                "title": "Analysis",
                "section_role": "analysis",
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "AI market trends",
                        "entities": [],
                        "must_have_terms": ["AI", "market", "trends"],
                        "optional_terms": [],
                        "time_range": None,
                        "source_bias": "article",
                    }
                ],
            }
        ],
    }
    result = await node(state)

    # Should call search_articles more than once (primary + broad + possibly narrow)
    assert evidence.search_articles.call_count >= 2


@pytest.mark.asyncio
async def test_fusion_applied_to_variant_results() -> None:
    """Results from multiple query variants should be fused via RRF."""
    evidence, node = _make_evidence()

    # Different calls return different articles
    call_count = 0

    async def mock_search(query: str, *, limit: int = 20) -> list[ArticleHit]:
        nonlocal call_count
        call_count += 1
        if call_count == 1:
            return [_article_hit("a1", score=0.9), _article_hit("a2", score=0.7)]
        return [_article_hit("a2", score=0.8), _article_hit("a3", score=0.6)]

    evidence.search_articles = AsyncMock(side_effect=mock_search)

    state = {
        "brief": {"topic": "AI market", "entities": ["OpenAI"]},
        "outline": [
            {
                "key": "analysis",
                "section_role": "analysis",
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "AI market trends",
                        "entities": [],
                        "must_have_terms": ["AI", "market", "trends"],
                        "optional_terms": [],
                    }
                ],
            }
        ],
    }
    result = await node(state)

    evidence_list = result.get("evidence", [])
    article_ids = {e["id"] for e in evidence_list if e["type"] == "article"}
    # a2 should appear (present in both lists, boosted by RRF)
    assert "a2" in article_ids


@pytest.mark.asyncio
async def test_variant_failure_degrades_to_primary() -> None:
    """If a variant search fails, should still return results from successful variants."""
    evidence, node = _make_evidence()

    call_count = 0

    async def mock_search(query: str, *, limit: int = 20) -> list[ArticleHit]:
        nonlocal call_count
        call_count += 1
        if call_count == 1:
            return [_article_hit("a1", score=0.9)]
        raise ConnectionError("Variant search failed")

    evidence.search_articles = AsyncMock(side_effect=mock_search)

    state = {
        "brief": {"topic": "AI market", "entities": ["OpenAI"]},
        "outline": [
            {
                "key": "analysis",
                "section_role": "analysis",
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "AI market trends",
                        "entities": [],
                        "must_have_terms": ["AI", "market", "trends"],
                        "optional_terms": [],
                    }
                ],
            }
        ],
    }
    result = await node(state)

    evidence_list = result.get("evidence", [])
    article_ids = {e["id"] for e in evidence_list if e["type"] == "article"}
    assert "a1" in article_ids


@pytest.mark.asyncio
async def test_legacy_path_unchanged() -> None:
    """Without query_facets, should use legacy search_queries path (no fusion)."""
    evidence, node = _make_evidence()
    evidence.search_articles = AsyncMock(
        return_value=[_article_hit("a1", score=0.5)]
    )

    state = {
        "brief": {"topic": "AI market"},
        "outline": [
            {
                "key": "analysis",
                "section_role": "analysis",
                "search_queries": ["AI market trends"],
            }
        ],
    }
    result = await node(state)

    evidence_list = result.get("evidence", [])
    assert len(evidence_list) >= 1


@pytest.mark.asyncio
async def test_fusion_strategy_injectable() -> None:
    """FusionStrategy should be injectable via constructor DI."""

    class MockFusion:
        def __init__(self) -> None:
            self.called = False

        def fuse(self, ranked_lists: list[list[ScoredHit]]) -> list[ScoredHit]:
            self.called = True
            # Flatten all lists
            all_hits: list[ScoredHit] = []
            for rl in ranked_lists:
                all_hits.extend(rl)
            return all_hits

    mock_fusion = MockFusion()
    evidence = AsyncMock()
    evidence.search_articles = AsyncMock(
        return_value=[_article_hit("a1", score=0.8)]
    )
    evidence.search_recaps = AsyncMock(return_value=[])

    node = GathererNode(evidence, fusion=mock_fusion)

    state = {
        "brief": {"topic": "AI market", "entities": ["OpenAI"]},
        "outline": [
            {
                "key": "analysis",
                "section_role": "analysis",
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "AI market trends",
                        "entities": [],
                        "must_have_terms": ["AI", "market", "trends"],
                        "optional_terms": [],
                    }
                ],
            }
        ],
    }
    await node(state)
    assert mock_fusion.called
