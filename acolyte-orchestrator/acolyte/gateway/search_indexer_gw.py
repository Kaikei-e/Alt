"""Search-indexer gateway — EvidenceProviderPort implementation via search-indexer REST API.

Uses GET /v1/search?q={query}&limit={limit} — the search-indexer's own REST endpoint,
NOT Meilisearch's /indexes/{index}/search directly.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.port.evidence_provider import ArticleHit, ArticleMetadata, RecapHit

if TYPE_CHECKING:
    import httpx

    from acolyte.config.settings import Settings

logger = structlog.get_logger(__name__)


class SearchIndexerGateway:
    """Evidence retrieval via search-indexer REST API."""

    def __init__(self, http_client: httpx.AsyncClient, settings: Settings) -> None:
        self._client = http_client
        self._base_url = settings.search_indexer_url

    async def search_articles(self, query: str, *, limit: int = 20) -> list[ArticleHit]:
        """Search articles via GET /v1/search."""
        resp = await self._client.get(
            f"{self._base_url}/v1/search",
            params={"q": query, "limit": limit},
        )
        resp.raise_for_status()
        data = resp.json()

        hits = []
        for hit in data.get("hits", []):
            # Truncate content to excerpt (first 200 chars)
            content = hit.get("content", "")
            excerpt = content[:200] + "..." if len(content) > 200 else content

            hits.append(
                ArticleHit(
                    article_id=str(hit.get("id", "")),
                    title=hit.get("title", ""),
                    url=hit.get("url", ""),
                    score=float(hit.get("_rankingScore", 0.0)),
                    published_at=hit.get("published_at"),
                    excerpt=excerpt,
                )
            )

        logger.info("search_articles", query=query, hits=len(hits))
        return hits

    async def fetch_article_metadata(self, article_ids: list[str]) -> list[ArticleMetadata]:
        """Fetch metadata — uses search with ID filter."""
        return []  # Not needed for current pipeline

    async def fetch_article_body(self, article_id: str) -> str:
        """Fetch full article body — not available via search-indexer REST API."""
        return ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        """Recap search — not available in search-indexer, returns empty."""
        return []
