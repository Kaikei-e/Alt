"""Consumer contract tests for acolyte-orchestrator → search-indexer.

Pins the REST /v1/search response shape that the Gatherer pipeline relies on.
Authentication is now established at the transport layer (mTLS client cert
verified by the nginx sidecar); the consumer no longer sends application-
level auth headers.

search-indexer actual API:
  GET /v1/search?q={query}&limit={limit}
  Response: {query: str, hits: [{id, title, content, tags, score, language}]}

  ``language`` is BCP-47 short ("ja", "en") or "und" when unknown; consumers
  treat missing values as "und" for language-quota rebalancing.

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
    """GET /v1/search returns hits with id/title/content/tags/score/language."""
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
                            "language": "en",
                        },
                        {
                            "id": "article-002",
                            "title": "AI市場 2026年展望",
                            "content": "人工知能市場は拡大を続けている...",
                            "tags": ["AI", "市場"],
                            "score": 0.78,
                            "language": "ja",
                        },
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
        assert len(data["hits"]) >= 2
        languages = {hit["language"] for hit in data["hits"]}
        assert "en" in languages and "ja" in languages
        for hit in data["hits"]:
            assert "id" in hit
            assert "title" in hit
            assert "content" in hit
            assert "tags" in hit
            assert "score" in hit
            assert "language" in hit
            assert isinstance(hit["score"], (int, float))

    pact.write_file(str(PACT_DIR), overwrite=True)


def test_search_articles_empty_results():
    """GET /v1/search returns empty hits array when no matches."""
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
