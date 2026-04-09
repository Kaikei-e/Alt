"""Evidence provider port — interface for article/recap search and retrieval."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol


@dataclass(frozen=True)
class ArticleHit:
    """Metadata-only search hit. Content is stored in ContentStore separately.

    Fields match search-indexer REST GET /v1/search response:
    id, title, content, tags. Note: url, published_at, _rankingScore
    are NOT returned by search-indexer.
    """

    article_id: str
    title: str
    tags: list[str] | None = None
    score: float = 0.0


@dataclass(frozen=True)
class ArticleMetadata:
    article_id: str
    title: str
    url: str
    source_name: str | None = None
    tags: list[str] | None = None
    published_at: str | None = None


@dataclass(frozen=True)
class RecapHit:
    recap_id: str
    title: str
    score: float
    summary: str | None = None


class EvidenceProviderPort(Protocol):
    async def search_articles(self, query: str, *, limit: int = 20) -> list[ArticleHit]: ...

    async def fetch_article_metadata(self, article_ids: list[str]) -> list[ArticleMetadata]: ...

    async def fetch_article_body(self, article_id: str) -> str: ...

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]: ...
