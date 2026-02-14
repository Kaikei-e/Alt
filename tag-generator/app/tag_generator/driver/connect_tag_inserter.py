"""Connect-RPC implementation of TagInserterPort.

Replaces BackendAPITagInserter with typed protobuf messages instead of
manual dict[str, Any] construction.
"""

from __future__ import annotations

from typing import Any

import structlog
from connectrpc.errors import ConnectError

from tag_generator.gen.proto.services.backend.v1 import internal_pb2
from tag_generator.gen.proto.services.backend.v1.internal_connect import (
    BackendInternalServiceClientSync,
)
from tag_inserter.upsert_tags import BatchResult

logger = structlog.get_logger(__name__)

_TIMEOUT_MS = 30000


class ConnectTagInserter:
    """Upserts tags via typed Connect-RPC client."""

    def __init__(
        self,
        client: BackendInternalServiceClientSync,
        auth_headers: dict[str, str],
    ) -> None:
        self.client = client
        self.auth_headers = auth_headers

    def upsert_tags(
        self,
        conn: Any,
        article_id: str,
        tags: list[str],
        feed_id: str,
        tag_confidences: dict[str, float] | None = None,
    ) -> dict[str, Any]:
        """Upsert tags for a single article via UpsertArticleTags RPC."""
        tag_items = [
            internal_pb2.TagItem(
                name=tag,
                confidence=tag_confidences.get(tag, 0.5) if tag_confidences else 0.5,
            )
            for tag in tags
        ]

        req = internal_pb2.UpsertArticleTagsRequest(
            article_id=article_id,
            feed_id=feed_id,
            tags=tag_items,
        )

        try:
            resp = self.client.upsert_article_tags(
                req,
                headers=self.auth_headers,
                timeout_ms=_TIMEOUT_MS,
            )
            return {
                "success": resp.success,
                "upserted_count": resp.upserted_count,
            }
        except ConnectError as e:
            logger.error("UpsertArticleTags failed", article_id=article_id, error=str(e))
            return {"success": False, "error": str(e)}

    def batch_upsert_tags_no_commit(self, conn: Any, article_tags: list[dict[str, Any]]) -> BatchResult:
        """Batch upsert tags via BatchUpsertArticleTags RPC."""
        items: list[internal_pb2.UpsertArticleTagsRequest] = []
        for entry in article_tags:
            article_id = entry.get("article_id", "")
            feed_id = entry.get("feed_id", "")
            tags = entry.get("tags", [])
            tag_confidences = entry.get("tag_confidences", {})

            tag_items = [
                internal_pb2.TagItem(
                    name=tag,
                    confidence=tag_confidences.get(tag, 0.5) if tag_confidences else 0.5,
                )
                for tag in tags
            ]

            items.append(
                internal_pb2.UpsertArticleTagsRequest(
                    article_id=article_id,
                    feed_id=feed_id,
                    tags=tag_items,
                )
            )

        req = internal_pb2.BatchUpsertArticleTagsRequest(items=items)

        try:
            resp = self.client.batch_upsert_article_tags(
                req,
                headers=self.auth_headers,
                timeout_ms=_TIMEOUT_MS,
            )
            return BatchResult(
                success=resp.success,
                processed_articles=len(article_tags),
                failed_articles=0,
                errors=[],
                message=None,
            )
        except ConnectError as e:
            logger.error("BatchUpsertArticleTags failed", error=str(e))
            return BatchResult(
                success=False,
                processed_articles=0,
                failed_articles=len(article_tags),
                errors=[str(e)],
                message=str(e),
            )

    def batch_upsert_tags_with_comparison(self, conn: Any, article_tags: list[dict[str, Any]]) -> BatchResult:
        """Batch upsert with comparison — delegates to batch_upsert_tags_no_commit in API mode."""
        return self.batch_upsert_tags_no_commit(conn, article_tags)
