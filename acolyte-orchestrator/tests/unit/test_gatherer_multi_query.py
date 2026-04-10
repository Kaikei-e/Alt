"""Unit tests for multi-query gatherer with section tagging + dedup (Phase 2)."""

from __future__ import annotations

import pytest

from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.port.evidence_provider import ArticleHit, RecapHit
from acolyte.usecase.graph.nodes.gatherer_node import GathererNode


class FakeEvidence:
    """Returns pre-configured articles per query keyword."""

    def __init__(self, articles_by_query: dict[str, list[ArticleHit]] | None = None) -> None:
        self._articles_by_query = articles_by_query or {}
        self.search_calls: list[str] = []

    async def search_articles(self, query: str, *, limit: int = 20) -> list[ArticleHit]:
        self.search_calls.append(query)
        # Match on substring
        for keyword, hits in self._articles_by_query.items():
            if keyword in query:
                return hits
        return []

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return "Body."

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        return []


@pytest.mark.asyncio
async def test_gatherer_uses_per_section_queries() -> None:
    """Gatherer should call search for each section's search_queries."""
    articles_by_query = {
        "market": [ArticleHit(article_id="art-1", title="Market Report", tags=["market"], score=0.9)],
        "chip": [ArticleHit(article_id="art-2", title="Chip Design", tags=["tech"], score=0.8)],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI semiconductor"},
        "outline": [
            {"key": "market", "title": "Market", "search_queries": ["market trends"]},
            {"key": "tech", "title": "Technology", "search_queries": ["chip architectures"]},
        ],
    }
    result = await node(state)

    # Should have called search for each query
    assert len(evidence.search_calls) >= 2
    assert any("market" in q for q in evidence.search_calls)
    assert any("chip" in q for q in evidence.search_calls)

    # Evidence should be present
    assert len(result["evidence"]) >= 2


@pytest.mark.asyncio
async def test_gatherer_tags_evidence_with_section_keys() -> None:
    """Each evidence item should be tagged with the section_keys that produced it."""
    articles_by_query = {
        "AI": [ArticleHit(article_id="art-1", title="AI Overview", tags=["AI"], score=0.9)],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI"},
        "outline": [
            {"key": "intro", "title": "Intro", "search_queries": ["AI overview"]},
        ],
    }
    result = await node(state)

    for item in result["evidence"]:
        if item["id"] == "art-1":
            assert "section_keys" in item
            assert "intro" in item["section_keys"]
            break
    else:
        pytest.fail("art-1 not found in evidence")


@pytest.mark.asyncio
async def test_gatherer_deduplicates_by_article_id() -> None:
    """Same article_id from different queries should be merged (section_keys combined)."""
    # Same article returned by two different queries
    shared = ArticleHit(article_id="art-shared", title="Shared Article", tags=["AI"], score=0.9)
    articles_by_query = {
        "market": [shared],
        "tech": [shared],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI"},
        "outline": [
            {"key": "market", "title": "Market", "search_queries": ["market trends"]},
            {"key": "tech", "title": "Tech", "search_queries": ["tech advances"]},
        ],
    }
    result = await node(state)

    # Should be only 1 evidence item (deduplicated)
    art_shared_items = [e for e in result["evidence"] if e["id"] == "art-shared"]
    assert len(art_shared_items) == 1
    # Should have both section keys merged
    assert set(art_shared_items[0]["section_keys"]) == {"market", "tech"}


@pytest.mark.asyncio
async def test_gatherer_falls_back_to_topic_when_no_outline() -> None:
    """When outline has no search_queries, gatherer falls back to topic-based search."""
    articles_by_query = {
        "AI": [ArticleHit(article_id="art-1", title="AI Article", tags=["AI"], score=0.9)],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI trends"},
        "outline": [
            {"key": "summary", "title": "Summary"},  # no search_queries
        ],
    }
    result = await node(state)

    # Should still produce evidence via topic fallback
    assert len(result["evidence"]) >= 1


@pytest.mark.asyncio
async def test_gatherer_empty_topic_returns_error() -> None:
    """No topic in brief should return error."""
    evidence = FakeEvidence()
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    result = await node({"brief": {}})

    assert result.get("error") is not None
    assert "topic" in result["error"].lower()


# --- Issue 6: Faceted search tests ---


@pytest.mark.asyncio
async def test_gatherer_uses_faceted_queries_when_available() -> None:
    """Outline with query_facets triggers faceted search path."""
    articles_by_query = {
        "AI": [ArticleHit(article_id="art-1", title="AI Article", tags=["AI"], score=0.9)],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI trends"},
        "outline": [
            {
                "key": "analysis",
                "title": "Analysis",
                "search_queries": ["AI trends"],
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "AI trends",
                        "must_have_terms": ["AI", "trends"],
                        "entities": [],
                        "optional_terms": [],
                        "time_range": None,
                        "source_bias": "article",
                    }
                ],
            },
        ],
    }
    result = await node(state)

    assert len(result["evidence"]) >= 1
    # Faceted path should have made search calls
    assert len(evidence.search_calls) >= 1


@pytest.mark.asyncio
async def test_gatherer_falls_back_to_search_queries_without_facets() -> None:
    """Outline without query_facets uses legacy search_queries path."""
    articles_by_query = {
        "market": [ArticleHit(article_id="art-1", title="Market Report", tags=["market"], score=0.9)],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI semiconductor"},
        "outline": [
            {"key": "market", "title": "Market", "search_queries": ["market trends"]},
        ],
    }
    result = await node(state)

    # Legacy path should still work
    assert len(result["evidence"]) >= 1
    assert any("market" in q for q in evidence.search_calls)


@pytest.mark.asyncio
async def test_gatherer_flags_weak_facets() -> None:
    """Facet returning fewer than WEAK_FACET_THRESHOLD hits appears in weak_facets."""
    # Only one hit for "niche" — below threshold of 2
    articles_by_query = {
        "niche": [ArticleHit(article_id="art-1", title="Niche Topic", tags=[], score=0.5)],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "niche topic"},
        "outline": [
            {
                "key": "analysis",
                "title": "Analysis",
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "niche topic analysis",
                        "must_have_terms": ["niche", "topic"],
                        "entities": [],
                        "optional_terms": [],
                        "time_range": None,
                        "source_bias": "article",
                    }
                ],
            },
        ],
    }
    result = await node(state)

    assert "weak_facets" in result
    assert len(result["weak_facets"]) >= 1
    assert result["weak_facets"][0]["section_key"] == "analysis"


@pytest.mark.asyncio
async def test_gatherer_deduplicates_across_facets() -> None:
    """Same article from two facets in one section is deduplicated."""
    shared = ArticleHit(article_id="art-shared", title="Shared", tags=[], score=0.9)
    articles_by_query = {
        "AI": [shared],
        "GPU": [shared],
    }
    evidence = FakeEvidence(articles_by_query)
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI GPU"},
        "outline": [
            {
                "key": "analysis",
                "title": "Analysis",
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "AI market",
                        "must_have_terms": ["AI"],
                        "entities": [],
                        "optional_terms": [],
                        "time_range": None,
                        "source_bias": "article",
                    },
                    {
                        "intent": "investigate",
                        "raw_query": "GPU demand",
                        "must_have_terms": ["GPU"],
                        "entities": [],
                        "optional_terms": [],
                        "time_range": None,
                        "source_bias": "article",
                    },
                ],
            },
        ],
    }
    result = await node(state)

    shared_items = [e for e in result["evidence"] if e["id"] == "art-shared"]
    assert len(shared_items) == 1
    assert "analysis" in shared_items[0]["section_keys"]


@pytest.mark.asyncio
async def test_gatherer_synthesis_only_sections_skip_search() -> None:
    """Sections with empty query_facets (synthesis_only) do not trigger search."""
    evidence = FakeEvidence()
    content_store = MemoryContentStore()
    node = GathererNode(evidence, content_store=content_store)

    state = {
        "brief": {"topic": "AI"},
        "outline": [
            {
                "key": "analysis",
                "title": "Analysis",
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "AI analysis",
                        "must_have_terms": ["AI"],
                        "entities": [],
                        "optional_terms": [],
                        "time_range": None,
                        "source_bias": "article",
                    },
                ],
            },
            {
                "key": "conclusion",
                "title": "Conclusion",
                "query_facets": [],  # synthesis_only
            },
        ],
    }
    await node(state)

    # Should only have called search for analysis, not conclusion
    for call in evidence.search_calls:
        assert "conclusion" not in call.lower()
