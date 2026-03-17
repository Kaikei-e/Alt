"""Port for persisting tags to storage."""

from __future__ import annotations

from typing import Any, Protocol

from tag_inserter.upsert_tags import BatchResult


class TagRepositoryPort(Protocol):
    """Port for persisting extracted tags via the backend API."""

    def upsert_tags(
        self,
        conn: Any,
        article_id: str,
        tags: list[str],
        feed_id: str,
        tag_confidences: dict[str, float] | None = None,
    ) -> dict[str, Any]: ...

    def batch_upsert_tags_no_commit(self, conn: Any, article_tags: list[dict[str, Any]]) -> BatchResult: ...

    def batch_upsert_tags_with_comparison(self, conn: Any, article_tags: list[dict[str, Any]]) -> BatchResult: ...
