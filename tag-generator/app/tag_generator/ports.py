"""Port interfaces for Clean Architecture dependency inversion.

These Protocol classes define the contracts that the tag_generator layer
expects from its dependencies (article_fetcher, tag_extractor, tag_inserter).
Concrete implementations in those packages satisfy these protocols structurally
(no explicit inheritance required).
"""

from __future__ import annotations

from typing import Any, Protocol

from tag_extractor.extract import TagExtractionOutcome
from tag_inserter.upsert_tags import BatchResult


class ArticleFetcherPort(Protocol):
    """Port for fetching articles from the backend API."""

    def fetch_articles(
        self,
        conn: object,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
    ) -> list[dict[str, Any]]: ...

    def fetch_new_articles(
        self,
        conn: object,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
    ) -> list[dict[str, Any]]: ...

    def count_untagged_articles(self, conn: object) -> int: ...

    def fetch_articles_by_status(
        self, conn: object, has_tags: bool = False, limit: int | None = None
    ) -> list[dict[str, Any]]: ...

    def fetch_low_confidence_articles(
        self,
        conn: object,
        confidence_threshold: float = 0.5,
        limit: int | None = None,
    ) -> list[dict[str, Any]]: ...

    def fetch_article_by_id(self, conn: object, article_id: str) -> dict[str, Any] | None: ...


class TagExtractorPort(Protocol):
    """Port for extracting tags from article text."""

    def extract_tags_with_metrics(self, title: str, content: str) -> TagExtractionOutcome: ...


class TagInserterPort(Protocol):
    """Port for persisting extracted tags via the backend API."""

    def upsert_tags(
        self,
        conn: object,
        article_id: str,
        tags: list[str],
        feed_id: str,
        tag_confidences: dict[str, float] | None = None,
    ) -> dict[str, Any]: ...

    def batch_upsert_tags_no_commit(self, conn: object, article_tags: list[dict[str, Any]]) -> BatchResult: ...

    def batch_upsert_tags_with_comparison(self, conn: object, article_tags: list[dict[str, Any]]) -> BatchResult: ...
