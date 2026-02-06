"""Port for extracting tags from text."""

from __future__ import annotations

from typing import Protocol

from tag_extractor.extract import TagExtractionOutcome


class TagExtractorPort(Protocol):
    """Port for extracting tags from article text."""

    def extract_tags_with_metrics(self, title: str, content: str) -> TagExtractionOutcome: ...
