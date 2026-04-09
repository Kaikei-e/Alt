"""Consumer contract tests for acolyte-orchestrator → search-indexer.

Verifies that acolyte-orchestrator's expectations of the search-indexer
REST API are documented as Pact contracts.

search-indexer actual API:
  GET /v1/search?q={query}&limit={limit}
  Response: {query: str, hits: [{id, title, content, tags, score}]}

Note: search-indexer does NOT return url or published_at.
score is Meilisearch _rankingScore (0.0-1.0).
Recap search is available via Connect v2 SearchRecaps (not REST).

Run with:
    cd acolyte-orchestrator && uv run pytest tests/contract/ -v --no-cov
"""

import json
from pathlib import Path

import httpx
from pact import Pact

PACT_DIR = Path(__file__).resolve().parent.parent.parent.parent / "pacts"


def _new_pact() -> Pact:
    return Pact("acolyte-orchestrator", "search-indexer")


def test_search_articles():
    """Verify contract for GET /v1/search (article search)."""
    pact = _new_pact()
    (
        pact.upon_receiving("an article search request for evidence gathering")
        .given("search-indexer has indexed articles")
        .with_request("GET", "/v1/search")
        .with_query_parameters({"q": "AI market trends 2026", "limit": "20"})
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {
                    "query": "AI market trends 2026",
                    "hits": [
                        {
                            "id": "article-001",
                            "title": "AI Market Overview 2026",
                            "content": "The artificial intelligence market continues to expand...",
                            "tags": ["AI", "market", "2026"],
                            "score": 0.85,
                        }
                    ],
                }
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.get(
            f"{srv.url}/v1/search",
            params={"q": "AI market trends 2026", "limit": "20"},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "hits" in data
        assert len(data["hits"]) > 0
        hit = data["hits"][0]
        assert "id" in hit
        assert "title" in hit
        assert "content" in hit
        assert "tags" in hit
        assert "score" in hit
        assert isinstance(hit["score"], (int, float))

    pact.write_file(str(PACT_DIR), overwrite=True)


def test_search_articles_empty_results():
    """Verify contract for GET /v1/search with no matches."""
    pact = _new_pact()
    (
        pact.upon_receiving("an article search request with no matches")
        .given("search-indexer has no matching articles")
        .with_request("GET", "/v1/search")
        .with_query_parameters({"q": "nonexistent topic xyz", "limit": "20"})
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {
                    "query": "nonexistent topic xyz",
                    "hits": [],
                }
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.get(
            f"{srv.url}/v1/search",
            params={"q": "nonexistent topic xyz", "limit": "20"},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["hits"] == []

    pact.write_file(str(PACT_DIR), overwrite=True)
