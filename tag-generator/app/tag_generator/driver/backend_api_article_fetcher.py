"""Backend API implementation of ArticleFetcherPort / ArticleRepositoryPort.

Replaces direct PostgreSQL access with Connect-RPC calls to alt-backend.
Accepts `conn` parameter for interface compatibility but ignores it.
"""

from __future__ import annotations

from typing import Any

import structlog

from tag_generator.driver.backend_api_client import BackendAPIClient

logger = structlog.get_logger(__name__)


class BackendAPIArticleFetcher:
    """Fetches articles via alt-backend's internal API instead of direct DB access."""

    def __init__(self, client: BackendAPIClient) -> None:
        self.client = client

    def fetch_articles(
        self,
        conn: Any,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
        untagged_only: bool = True,
    ) -> list[dict[str, Any]]:
        """Fetch articles using ListUntaggedArticles RPC.

        Note: In API mode, we always fetch untagged articles (ignoring untagged_only flag)
        since the API provides exactly that endpoint.
        """
        limit = custom_batch_size or 75
        resp = self.client.call("ListUntaggedArticles", {
            "limit": limit,
            "offset": 0,
        })

        articles = []
        for a in resp.get("articles", []):
            articles.append({
                "id": a.get("id", ""),
                "title": a.get("title", ""),
                "content": a.get("content", ""),
                "created_at": a.get("createdAt", last_created_at),
                "feed_id": None,
                "url": "",
                "user_id": a.get("userId", ""),
            })

        return articles

    def fetch_new_articles(
        self,
        conn: Any,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
    ) -> list[dict[str, Any]]:
        """Fetch new articles — delegates to fetch_articles in API mode."""
        return self.fetch_articles(conn, last_created_at, last_id, custom_batch_size, untagged_only=False)

    def count_untagged_articles(self, conn: Any) -> int:
        """Count untagged articles via ListUntaggedArticles RPC."""
        resp = self.client.call("ListUntaggedArticles", {
            "limit": 1,
            "offset": 0,
        })
        return int(resp.get("totalCount", 0))

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
            resp = self.client.call("GetArticleContent", {
                "articleId": article_id,
            })
            return {
                "id": resp.get("articleId", ""),
                "title": resp.get("title", ""),
                "content": resp.get("content", ""),
                "url": resp.get("url", ""),
                "created_at": "",
                "feed_id": None,
            }
        except Exception:
            logger.warning("Failed to fetch article by ID via API", article_id=article_id)
            return None
