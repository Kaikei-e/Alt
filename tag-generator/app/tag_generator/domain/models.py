"""Domain models for the tag-generator service.

Pure value objects with no infrastructure dependencies.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from tag_extractor.extract import TagExtractionOutcome


@dataclass(frozen=True)
class Article:
    """Immutable representation of an article to be tagged."""

    id: str
    title: str
    content: str
    created_at: str
    feed_id: str | None = None
    url: str | None = None

    @classmethod
    def from_dict(cls, raw: dict[str, Any]) -> Article:
        """Create an Article from a raw dictionary (e.g. DB row)."""
        created_at = raw["created_at"]
        if isinstance(created_at, datetime):
            created_at = created_at.isoformat()
        return cls(
            id=raw["id"],
            title=raw["title"],
            content=raw["content"],
            created_at=created_at,
            feed_id=raw.get("feed_id"),
            url=raw.get("url"),
        )

    def to_dict(self) -> dict[str, Any]:
        """Convert to a plain dictionary."""
        return {
            "id": self.id,
            "title": self.title,
            "content": self.content,
            "created_at": self.created_at,
            "feed_id": self.feed_id,
            "url": self.url,
        }


@dataclass(frozen=True)
class Tag:
    """A single extracted tag with its confidence score."""

    name: str
    confidence: float


@dataclass(frozen=True)
class TagExtractionResult:
    """Result of tag extraction for a single article."""

    article_id: str
    tags: list[Tag]
    language: str
    inference_ms: float
    overall_confidence: float

    @property
    def tag_names(self) -> list[str]:
        """Return tag names as a list of strings."""
        return [tag.name for tag in self.tags]

    @property
    def tag_confidences(self) -> dict[str, float]:
        """Return a mapping of tag name to confidence."""
        return {tag.name: tag.confidence for tag in self.tags}

    @property
    def is_empty(self) -> bool:
        """Return True if no tags were extracted."""
        return len(self.tags) == 0

    @classmethod
    def from_outcome(cls, article_id: str, outcome: TagExtractionOutcome) -> TagExtractionResult:
        """Convert a TagExtractionOutcome to the domain model."""
        tags = [Tag(name=name, confidence=outcome.tag_confidences.get(name, 0.5)) for name in outcome.tags]
        return cls(
            article_id=article_id,
            tags=tags,
            language=outcome.language,
            inference_ms=outcome.inference_ms,
            overall_confidence=outcome.confidence,
        )


@dataclass
class BatchResult:
    """Mutable result of a batch processing operation."""

    total_processed: int = 0
    successful: int = 0
    failed: int = 0
    has_more_pending: bool = False

    @property
    def is_success(self) -> bool:
        """A batch is successful if there are no failures."""
        return self.failed == 0

    def to_dict(self) -> dict[str, Any]:
        """Convert to a plain dictionary."""
        return {
            "total_processed": self.total_processed,
            "successful": self.successful,
            "failed": self.failed,
            "has_more_pending": self.has_more_pending,
        }


@dataclass(frozen=True)
class CursorPosition:
    """Immutable pagination cursor for article fetching."""

    created_at: str
    article_id: str
