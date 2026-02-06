"""Port for persisting tags to storage."""

from __future__ import annotations

from typing import Any, Protocol

from psycopg2.extensions import connection as Connection

from tag_inserter.upsert_tags import BatchResult


class TagRepositoryPort(Protocol):
    """Port for persisting extracted tags to the database."""

    def upsert_tags(
        self,
        conn: Connection,
        article_id: str,
        tags: list[str],
        feed_id: str,
        tag_confidences: dict[str, float] | None = None,
    ) -> dict[str, Any]: ...

    def batch_upsert_tags_no_commit(self, conn: Connection, article_tags: list[dict[str, Any]]) -> BatchResult: ...

    def batch_upsert_tags_with_comparison(
        self, conn: Connection, article_tags: list[dict[str, Any]]
    ) -> BatchResult: ...
