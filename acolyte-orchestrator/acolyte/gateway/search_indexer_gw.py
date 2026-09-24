"""Search-indexer gateway — EvidenceProviderPort implementation via search-indexer REST API.

Uses GET /v1/search?q={query}&limit={limit} — the search-indexer's own REST endpoint.

Response schema: {query: str, hits: [{id, title, content, tags, score, published_at}]}
Note: search-indexer does NOT return url.
score is Meilisearch _rankingScore (0.0-1.0).

Content from search results is stored in ContentStore (not in ArticleHit)
to follow the 'Fetch metadata first, body only for top-N' rule.
"""

from __future__ import annotations

from typing import TYPE_CHECKING
from uuid import UUID

import httpx
import structlog

from acolyte.port.evidence_provider import (
    ArticleHit,
    ArticleMetadata,
    EvidenceProviderError,
    EvidenceStatusError,
    RecapHit,
)

if TYPE_CHECKING:
    from datetime import datetime

    from acolyte.config.settings import Settings
    from acolyte.port.content_store import ContentStorePort

logger = structlog.get_logger(__name__)

# Cap the query string logged at INFO so HyDE-generated hypothetical passages
# (hundreds of chars of synthetic topic content) do not flood observability
# pipelines. Full query is still sent to the search-indexer over mTLS.
_LOG_QUERY_MAX_CHARS = 120

# Stub methods that return [] — warn once so "0 hits" is distinguishable from
# "not implemented via REST" without spamming every gatherer call.
_stub_warned: set[str] = set()


def _warn_stub_once(method: str, detail: str) -> None:
    if method in _stub_warned:
        return
    _stub_warned.add(method)
    logger.warning("search_indexer_stub_empty", method=method, detail=detail)


class SearchIndexerGateway:
    """Evidence retrieval via search-indexer REST API."""

    def __init__(self, http_client: httpx.AsyncClient, settings: Settings, content_store: ContentStorePort) -> None:
        self._client = http_client
        self._base_url = settings.search_indexer_url
        self._content_store = content_store

    async def search_articles(
        self,
        query: str,
        *,
        user_id: UUID,
        limit: int = 20,
        published_after: datetime | None = None,
        published_before: datetime | None = None,
    ) -> list[ArticleHit]:
        """Search articles via GET /v1/search.

        Stores content in ContentStore; returns metadata-only ArticleHit.
        Authentication is established at the TLS transport layer (mTLS).
        """
        if not isinstance(user_id, UUID):
            msg = "user_id must be a UUID for article search"
            raise TypeError(msg)

        params: dict[str, str | int] = {
            "q": query,
            "limit": limit,
            "user_id": str(user_id),
        }
        if published_after is not None:
            params["published_after"] = published_after.isoformat()
        if published_before is not None:
            params["published_before"] = published_before.isoformat()

        try:
            resp = await self._client.get(
                f"{self._base_url}/v1/search",
                params=params,
            )
            resp.raise_for_status()
        except httpx.HTTPStatusError as exc:
            raise EvidenceStatusError(exc.response.status_code, exc.response.text) from exc
        except httpx.HTTPError as exc:
            raise EvidenceProviderError(exc) from exc

        data = resp.json()

        hits = []
        for hit in data.get("hits", []):
            article_id = str(hit.get("id", ""))

            # Store content in ContentStore for later top-N hydration
            content = hit.get("content", "")
            if content:
                await self._content_store.store(article_id, content)
            else:
                logger.warning("search_articles_empty_content", article_id=article_id)

            hits.append(
                ArticleHit(
                    article_id=article_id,
                    title=hit.get("title", ""),
                    tags=hit.get("tags"),
                    score=float(hit.get("score", 0.0)),
                    language=str(hit.get("language") or "und"),
                    published_at=hit.get("published_at"),
                )
            )

        logger.info(
            "search_articles",
            query=query[:_LOG_QUERY_MAX_CHARS],
            query_full_len=len(query),
            hits=len(hits),
        )
        return hits

    async def fetch_article_metadata(self, article_ids: list[str]) -> list[ArticleMetadata]:
        """Fetch metadata — not available via search-indexer REST API."""
        _warn_stub_once(
            "fetch_article_metadata",
            "not available via search-indexer REST; returning empty list",
        )
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        """Fetch full article body from ContentStore."""
        body = await self._content_store.fetch(article_id)
        return body or ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        """Recap search — not available via REST. Use Connect v2 SearchRecaps for recap evidence."""
        _warn_stub_once(
            "search_recaps",
            "not available via REST; use Connect v2 SearchRecaps; returning empty list",
        )
        return []
