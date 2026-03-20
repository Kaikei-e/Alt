"""Connect-RPC implementation of ArticleFetcherPort.

Replaces BackendAPIArticleFetcher with typed protobuf messages instead of
manual dict[str, Any] construction.
"""

from __future__ import annotations

from typing import Any

import structlog
from connectrpc.errors import ConnectError
from google.protobuf.timestamp_pb2 import Timestamp

from tag_generator.gen.proto.services.backend.v1 import internal_pb2
from tag_generator.gen.proto.services.backend.v1.internal_connect import (
    BackendInternalServiceClientSync,
)

logger = structlog.get_logger(__name__)

_TIMEOUT_MS = 30000


class ConnectArticleFetcher:
    """Fetches articles via typed Connect-RPC client."""

    def __init__(
        self,
        client: BackendInternalServiceClientSync,
        auth_headers: dict[str, str],
    ) -> None:
        self.client = client
        self.auth_headers = auth_headers

    # Sentinel value indicating "first page" (no cursor yet).
    _FIRST_PAGE_SENTINEL = "9999-12-31T23:59:59Z"

    def fetch_articles(
        self,
        conn: Any,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
        untagged_only: bool = True,
    ) -> list[dict[str, Any]]:
        """Fetch articles using ListUntaggedArticles RPC with keyset pagination."""
        limit = custom_batch_size or 75

        req = internal_pb2.ListUntaggedArticlesRequest(limit=limit)

        # Pass cursor to backend for keyset pagination (skip on first page)
        if last_created_at and last_created_at != self._FIRST_PAGE_SENTINEL:
            ts = Timestamp()
            ts.FromJsonString(last_created_at)
            req.last_created_at.CopyFrom(ts)
            req.last_id = last_id

        resp = self.client.list_untagged_articles(
            req,
            headers=self.auth_headers,
            timeout_ms=_TIMEOUT_MS,
        )

        articles: list[dict[str, Any]] = []
        for a in resp.articles:
            created_at = last_created_at
            if a.HasField("created_at"):
                created_at = a.created_at.ToJsonString()

            articles.append(
                {
                    "id": a.id,
                    "title": a.title,
                    "content": a.content,
                    "created_at": created_at,
                    "feed_id": a.feed_id if a.feed_id else None,
                    "url": "",
                    "user_id": a.user_id,
                }
            )

        return articles

    def fetch_new_articles(
        self,
        conn: Any,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
    ) -> list[dict[str, Any]]:
        """Fetch new untagged articles — always requests first page (newest).

        The forward processing path needs the newest untagged articles,
        not articles older than the cursor. With backward keyset pagination,
        the first page (no cursor) returns the newest results.
        """
        return self.fetch_articles(conn, self._FIRST_PAGE_SENTINEL, "", custom_batch_size, untagged_only=False)

    def count_untagged_articles(self, conn: Any) -> int:
        """Count untagged articles via ListUntaggedArticles RPC."""
        req = internal_pb2.ListUntaggedArticlesRequest(limit=1)
        resp = self.client.list_untagged_articles(
            req,
            headers=self.auth_headers,
            timeout_ms=_TIMEOUT_MS,
        )
        return resp.total_count

    def fetch_articles_by_status(
        self, conn: Any, has_tags: bool = False, limit: int | None = None
    ) -> list[dict[str, Any]]:
        """Fetch articles by tag status."""
        if has_tags:
            return []
        return self.fetch_articles(conn, "9999-12-31T23:59:59Z", "zzzzz", limit)

    def fetch_low_confidence_articles(
        self,
        conn: Any,
        confidence_threshold: float = 0.5,
        limit: int | None = None,
    ) -> list[dict[str, Any]]:
        """Not available via API — returns empty list."""
        return []

    def fetch_article_by_id(self, conn: Any, article_id: str) -> dict[str, Any] | None:
        """Fetch article by ID via GetArticleContent RPC."""
        try:
            req = internal_pb2.GetArticleContentRequest(article_id=article_id)
            resp = self.client.get_article_content(
                req,
                headers=self.auth_headers,
                timeout_ms=_TIMEOUT_MS,
            )
            return {
                "id": resp.article_id,
                "title": resp.title,
                "content": resp.content,
                "url": resp.url,
                "created_at": "",
                "feed_id": None,
            }
        except ConnectError:
            logger.warning("Failed to fetch article by ID via API", article_id=article_id)
            return None
