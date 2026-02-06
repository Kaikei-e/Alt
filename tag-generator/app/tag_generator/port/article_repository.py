"""Port for fetching articles from storage."""

from __future__ import annotations

from typing import Any, Protocol

from psycopg2.extensions import connection as Connection


class ArticleRepositoryPort(Protocol):
    """Port for fetching articles from the database."""

    def fetch_articles(
        self,
        conn: Connection,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
        untagged_only: bool = True,
    ) -> list[dict[str, Any]]: ...

    def fetch_new_articles(
        self,
        conn: Connection,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
    ) -> list[dict[str, Any]]: ...

    def count_untagged_articles(self, conn: Connection) -> int: ...

    def fetch_articles_by_status(
        self, conn: Connection, has_tags: bool = False, limit: int | None = None
    ) -> list[dict[str, Any]]: ...

    def fetch_low_confidence_articles(
        self,
        conn: Connection,
        confidence_threshold: float = 0.5,
        limit: int | None = None,
    ) -> list[dict[str, Any]]: ...

    def fetch_article_by_id(self, conn: Connection, article_id: str) -> dict[str, Any] | None: ...
