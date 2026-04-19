"""Search-indexer gateway — EvidenceProviderPort implementation via search-indexer REST API.

Uses GET /v1/search?q={query}&limit={limit} — the search-indexer's own REST endpoint.

Response schema: {query: str, hits: [{id, title, content, tags, score}]}
Note: search-indexer does NOT return url or published_at.
score is Meilisearch _rankingScore (0.0-1.0).

Content from search results is stored in ContentStore (not in ArticleHit)
to follow the 'Fetch metadata first, body only for top-N' rule.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.port.evidence_provider import ArticleHit, ArticleMetadata, RecapHit

if TYPE_CHECKING:
    import httpx

    from acolyte.config.settings import Settings
    from acolyte.port.content_store import ContentStorePort

logger = structlog.get_logger(__name__)


class SearchIndexerGateway:
    """Evidence retrieval via search-indexer REST API."""

    def __init__(self, http_client: httpx.AsyncClient, settings: Settings, content_store: ContentStorePort) -> None:
        self._client = http_client
        self._base_url = settings.search_indexer_url
        self._content_store = content_store

    async def search_articles(self, query: str, *, limit: int = 20) -> list[ArticleHit]:
        """Search articles via GET /v1/search.

        Stores content in ContentStore; returns metadata-only ArticleHit.
        Authentication is established at the TLS transport layer (mTLS).
        """
        resp = await self._client.get(
            f"{self._base_url}/v1/search",
            params={"q": query, "limit": limit},
        )
        resp.raise_for_status()
        data = resp.json()

        hits = []
        for hit in data.get("hits", []):
            article_id = str(hit.get("id", ""))

            # Store content in ContentStore for later top-N hydration
            content = hit.get("content", "")
            if content:
                await self._content_store.store(article_id, content)

            hits.append(
                ArticleHit(
                    article_id=article_id,
                    title=hit.get("title", ""),
                    tags=hit.get("tags"),
                    score=float(hit.get("score", 0.0)),
                    language=str(hit.get("language") or "und"),
                )
            )

        logger.info("search_articles", query=query, hits=len(hits))
        return hits

    async def fetch_article_metadata(self, article_ids: list[str]) -> list[ArticleMetadata]:
        """Fetch metadata — not available via search-indexer REST API."""
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        """Fetch full article body from ContentStore."""
        body = await self._content_store.fetch(article_id)
        return body or ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        """Recap search — not available via REST. Use Connect v2 SearchRecaps for recap evidence."""
        return []
