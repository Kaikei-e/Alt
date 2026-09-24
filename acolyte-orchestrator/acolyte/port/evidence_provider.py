"""Evidence provider port — interface for article/recap search and retrieval."""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    from datetime import datetime
    from uuid import UUID


class EvidenceProviderError(Exception):
    """Base error for evidence provider operations."""

    def __init__(self, exc: Exception | str | None = None) -> None:
        detail = f": {exc}" if exc is not None else ""
        super().__init__(f"evidence provider request failed{detail}")


_MAX_RESPONSE_CHARS_IN_ERROR = 200


class EvidenceStatusError(EvidenceProviderError):
    """Raised when upstream evidence provider returns an HTTP error status."""

    def __init__(
        self,
        status_code: int,
        response_text: str = "",
    ) -> None:
        self.status_code = status_code
        self.response_text = response_text
        truncated = response_text[:_MAX_RESPONSE_CHARS_IN_ERROR]
        suffix = "..." if len(response_text) > _MAX_RESPONSE_CHARS_IN_ERROR else ""
        detail = f": {truncated}{suffix}" if truncated else ""
        Exception.__init__(self, f"evidence provider query failed with status {status_code}{detail}")


@dataclass(frozen=True)
class ArticleHit:
    """Metadata-only search hit. Content is stored in ContentStore separately.

    Fields match search-indexer REST GET /v1/search response:
    id, title, content, tags, score, language, published_at.

    ``language`` is a BCP-47 short code (``ja``, ``en``) or ``und`` when the
    upstream does not yet populate it.
    """

    article_id: str
    title: str
    tags: list[str] | None = None
    score: float = 0.0
    language: str = "und"
    published_at: str | None = None


@dataclass(frozen=True)
class ArticleMetadata:
    article_id: str
    title: str
    url: str
    source_name: str | None = None
    tags: list[str] | None = None
    published_at: str | None = None
    language: str = "und"


@dataclass(frozen=True)
class RecapHit:
    recap_id: str
    title: str
    score: float
    summary: str | None = None


class EvidenceProviderPort(Protocol):
    async def search_articles(
        self,
        query: str,
        *,
        user_id: UUID,
        limit: int = 20,
        published_after: datetime | None = None,
        published_before: datetime | None = None,
    ) -> list[ArticleHit]: ...

    async def fetch_article_metadata(self, article_ids: list[str]) -> list[ArticleMetadata]: ...

    async def fetch_article_body(self, article_id: str) -> str: ...

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]: ...
