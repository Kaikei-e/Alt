"""Port interfaces for Clean Architecture dependency inversion.

These Protocol classes define the contracts that the tag_generator layer
expects from its dependencies (article_fetcher, tag_extractor, tag_inserter).
Concrete implementations in those packages satisfy these protocols structurally
(no explicit inheritance required).
"""

from __future__ import annotations

from typing import Any, Protocol

from psycopg2.extensions import connection as Connection

from tag_extractor.extract import TagExtractionOutcome
from tag_inserter.upsert_tags import BatchResult


class ArticleFetcherPort(Protocol):
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


class TagExtractorPort(Protocol):
    """Port for extracting tags from article text."""

    def extract_tags_with_metrics(self, title: str, content: str) -> TagExtractionOutcome: ...


class TagInserterPort(Protocol):
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
