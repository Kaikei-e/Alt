"""Consumer contract tests for acolyte-orchestrator → search-indexer.

Verifies that acolyte-orchestrator's expectations of the search-indexer
search API are documented as Pact contracts.

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
    """Verify contract for POST /indexes/articles/search."""
    pact = _new_pact()
    (
        pact.upon_receiving("an article search request for evidence gathering")
        .given("search-indexer has indexed articles")
        .with_request("POST", "/indexes/articles/search")
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {
                    "hits": [
                        {
                            "id": "article-001",
                            "title": "AI Market Overview 2026",
                            "url": "https://example.com/ai-2026",
                            "published_at": "2026-04-01T00:00:00Z",
                        }
                    ],
                    "estimatedTotalHits": 1,
                    "limit": 20,
                    "offset": 0,
                }
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.post(
            f"{srv.url}/indexes/articles/search",
            json={"q": "AI market trends 2026", "limit": 20},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "hits" in data
        assert len(data["hits"]) > 0
        assert "title" in data["hits"][0]

    pact.write_file(str(PACT_DIR), overwrite=True)


def test_search_recaps():
    """Verify contract for POST /indexes/recaps/search."""
    pact = _new_pact()
    (
        pact.upon_receiving("a recap search request for evidence gathering")
        .given("search-indexer has indexed recaps")
        .with_request("POST", "/indexes/recaps/search")
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {
                    "hits": [
                        {
                            "id": "recap-001",
                            "title": "Tech Trends Weekly Recap",
                            "summary": "This week in technology...",
                        }
                    ],
                    "estimatedTotalHits": 1,
                    "limit": 10,
                    "offset": 0,
                }
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.post(
            f"{srv.url}/indexes/recaps/search",
            json={"q": "technology trends", "limit": 10},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "hits" in data
        assert len(data["hits"]) > 0

    pact.write_file(str(PACT_DIR), overwrite=True)
