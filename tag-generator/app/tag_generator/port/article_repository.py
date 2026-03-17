"""Port for fetching articles from storage."""

from __future__ import annotations

from typing import Any, Protocol


class ArticleRepositoryPort(Protocol):
    """Port for fetching articles from the backend API."""

    def fetch_articles(
        self,
        conn: Any,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
        untagged_only: bool = True,
    ) -> list[dict[str, Any]]: ...

    def fetch_new_articles(
        self,
        conn: Any,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
    ) -> list[dict[str, Any]]: ...

    def count_untagged_articles(self, conn: Any) -> int: ...

    def fetch_articles_by_status(
        self, conn: Any, has_tags: bool = False, limit: int | None = None
    ) -> list[dict[str, Any]]: ...

    def fetch_low_confidence_articles(
        self,
        conn: Any,
        confidence_threshold: float = 0.5,
        limit: int | None = None,
    ) -> list[dict[str, Any]]: ...

    def fetch_article_by_id(self, conn: Any, article_id: str) -> dict[str, Any] | None: ...
