"""Usecase: Extract tags from a single article."""

from __future__ import annotations

from typing import TYPE_CHECKING

from tag_generator.domain.models import TagExtractionResult

if TYPE_CHECKING:
    from tag_generator.port.tag_extractor import TagExtractorPort


class ExtractTagsUsecase:
    """Extract tags from an article's title and content."""

    def __init__(self, tag_extractor: TagExtractorPort) -> None:
        self._tag_extractor = tag_extractor

    def execute(self, article_id: str, title: str, content: str) -> TagExtractionResult:
        """Run tag extraction and return a domain-typed result.

        Raises:
            TagExtractionError: If the ML model fails.
        """
        outcome = self._tag_extractor.extract_tags_with_metrics(title, content)
        return TagExtractionResult.from_outcome(article_id, outcome)
