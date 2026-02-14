"""Backend API implementation of TagInserterPort / TagRepositoryPort.

Replaces direct PostgreSQL access with Connect-RPC calls to alt-backend.
Accepts `conn` parameter for interface compatibility but ignores it.
"""

from __future__ import annotations

from typing import Any

import structlog

from tag_inserter.upsert_tags import BatchResult
from tag_generator.driver.backend_api_client import BackendAPIClient

logger = structlog.get_logger(__name__)


class BackendAPITagInserter:
    """Upserts tags via alt-backend's internal API instead of direct DB access."""

    def __init__(self, client: BackendAPIClient) -> None:
        self.client = client

    def upsert_tags(
        self,
        conn: Any,
        article_id: str,
        tags: list[str],
        feed_id: str,
        tag_confidences: dict[str, float] | None = None,
    ) -> dict[str, Any]:
        """Upsert tags for a single article via UpsertArticleTags RPC."""
        tag_items = []
        for tag in tags:
            confidence = 0.5
            if tag_confidences and tag in tag_confidences:
                confidence = tag_confidences[tag]
            tag_items.append({
                "name": tag,
                "confidence": confidence,
            })

        try:
            resp = self.client.call("UpsertArticleTags", {
                "articleId": article_id,
                "feedId": feed_id,
                "tags": tag_items,
            })
            return {
                "success": resp.get("success", False),
                "upserted_count": resp.get("upsertedCount", 0),
            }
        except Exception as e:
            logger.error("UpsertArticleTags failed", article_id=article_id, error=str(e))
            return {"success": False, "error": str(e)}

    def batch_upsert_tags_no_commit(
        self, conn: Any, article_tags: list[dict[str, Any]]
    ) -> BatchResult:
        """Batch upsert tags via BatchUpsertArticleTags RPC.

        Note: In API mode, the backend handles transactions internally,
        so "no_commit" is effectively a full commit.
        """
        items = []
        for entry in article_tags:
            article_id = entry.get("article_id", "")
            feed_id = entry.get("feed_id", "")
            tags = entry.get("tags", [])
            tag_confidences = entry.get("tag_confidences", {})

            tag_items = []
            for tag in tags:
                confidence = 0.5
                if tag_confidences and tag in tag_confidences:
                    confidence = tag_confidences[tag]
                tag_items.append({
                    "name": tag,
                    "confidence": confidence,
                })

            items.append({
                "articleId": article_id,
                "feedId": feed_id,
                "tags": tag_items,
            })

        try:
            resp = self.client.call("BatchUpsertArticleTags", {"items": items})
            return BatchResult(
                success=resp.get("success", False),
                processed_articles=len(article_tags),
                failed_articles=0,
                errors=[],
                message=None,
            )
        except Exception as e:
            logger.error("BatchUpsertArticleTags failed", error=str(e))
            return BatchResult(
                success=False,
                processed_articles=0,
                failed_articles=len(article_tags),
                errors=[str(e)],
                message=str(e),
            )

    def batch_upsert_tags_with_comparison(
        self, conn: Any, article_tags: list[dict[str, Any]]
    ) -> BatchResult:
        """Batch upsert with comparison — delegates to batch_upsert_tags_no_commit in API mode."""
        return self.batch_upsert_tags_no_commit(conn, article_tags)
