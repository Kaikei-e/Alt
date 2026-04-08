"""Content Validator for summarization (Phase 4 refactoring).

This module extracts content validation logic from SummarizeUsecase.generate_summary()
following SOLID principles (Single Responsibility Principle).

Following Python 3.14 best practices:
- Frozen dataclass for validation results
- Factory method for configuration-based instantiation
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from news_creator.utils.html_cleaner import clean_html_content

if TYPE_CHECKING:
    from news_creator.config.config import NewsCreatorConfig

logger = logging.getLogger(__name__)

# Default thresholds matching CDC contracts and context window limits
DEFAULT_MIN_LENGTH = 100  # CDC contract minimum content length
DEFAULT_MAX_LENGTH = 60_000  # ~15K tokens for 16K context window
DEFAULT_ABNORMAL_THRESHOLD = 100_000  # Warning threshold for very large content


@dataclass(frozen=True)
class ValidationResult:
    """Immutable result of content validation.

    Attributes:
        cleaned_content: Content after HTML cleaning and truncation
        was_html: Whether HTML was detected and removed
        was_truncated: Whether content was truncated to fit limits
        original_length: Length of original content before processing
        warnings: List of warning messages (e.g., abnormal size)
    """

    cleaned_content: str
    was_html: bool
    was_truncated: bool
    original_length: int
    warnings: list[str] = field(default_factory=list)


class ContentValidator:
    """Validates and cleans content for summarization.

    Responsibilities:
    - Clean HTML from content (Zero Trust validation)
    - Validate minimum content length
    - Truncate content to fit context window
    - Report abnormal content sizes

    This class extracts lines 49-141 from SummarizeUsecase.generate_summary().
    """

    def __init__(
        self,
        min_length: int = DEFAULT_MIN_LENGTH,
        max_length: int = DEFAULT_MAX_LENGTH,
        abnormal_size_threshold: int = DEFAULT_ABNORMAL_THRESHOLD,
    ):
        """Initialize content validator.

        Args:
            min_length: Minimum content length after cleaning
            max_length: Maximum content length (will truncate if exceeded)
            abnormal_size_threshold: Threshold for abnormal size warning
        """
        self.min_length = min_length
        self.max_length = max_length
        self.abnormal_size_threshold = abnormal_size_threshold

    @classmethod
    def from_config(cls, config: NewsCreatorConfig) -> ContentValidator:
        """Create ContentValidator from NewsCreatorConfig.

        Args:
            config: News creator configuration

        Returns:
            Configured ContentValidator instance
        """
        return cls(
            min_length=DEFAULT_MIN_LENGTH,
            max_length=DEFAULT_MAX_LENGTH,
            abnormal_size_threshold=DEFAULT_ABNORMAL_THRESHOLD,
        )

    def validate_and_clean(
        self,
        content: str,
        article_id: str = "",
    ) -> ValidationResult:
        """Validate and clean content for summarization.

        Args:
            content: Raw content to validate
            article_id: Article ID for logging

        Returns:
            ValidationResult with cleaned content and metadata

        Raises:
            ValueError: If content is empty or too short after cleaning
        """
        warnings: list[str] = []
        original_length = len(content)

        # Zero Trust: Always clean HTML from content
        logger.info(
            "Cleaning content (Zero Trust validation)",
            extra={
                "article_id": article_id,
                "original_length": original_length,
            }
        )

        cleaned_content, was_html = clean_html_content(content, article_id)
        cleaned_length = len(cleaned_content)

        if was_html:
            reduction_ratio = (
                (1.0 - (cleaned_length / original_length)) * 100.0
                if original_length > 0
                else 0.0
            )
            logger.warning(
                "HTML detected and removed from article content",
                extra={
                    "article_id": article_id,
                    "original_length": original_length,
                    "cleaned_length": cleaned_length,
                    "reduction_ratio": round(reduction_ratio, 2),
                }
            )
        else:
            logger.info(
                "Content appears to be plain text (no HTML detected)",
                extra={
                    "article_id": article_id,
                    "content_length": cleaned_length,
                }
            )

        # Strip whitespace
        cleaned_content = cleaned_content.strip()

        # Validate minimum length
        if not cleaned_content or len(cleaned_content) < self.min_length:
            error_msg = (
                f"Content is empty or too short after HTML cleaning. "
                f"Original length: {original_length}, "
                f"Cleaned length: {len(cleaned_content)}, "
                f"Minimum required: {self.min_length} characters. "
                f"This article may not have enough content to generate a meaningful summary."
            )
            logger.warning(
                "Article content too short for summarization",
                extra={
                    "article_id": article_id,
                    "was_html": was_html,
                    "original_length": original_length,
                    "cleaned_length": len(cleaned_content),
                    "min_required": self.min_length,
                    "content_preview": cleaned_content[:100] if cleaned_content else "",
                }
            )
            raise ValueError(error_msg)

        # Check for abnormal size
        if len(cleaned_content) > self.abnormal_size_threshold:
            warning_msg = f"Abnormally large content detected: {len(cleaned_content)} characters"
            warnings.append(warning_msg)
            logger.warning(
                "ABNORMAL CONTENT SIZE detected",
                extra={
                    "article_id": article_id,
                    "content_length": len(cleaned_content),
                    "threshold": self.abnormal_size_threshold,
                }
            )

        # Truncate if necessary
        was_truncated = len(cleaned_content) > self.max_length
        if was_truncated:
            cleaned_content = cleaned_content[:self.max_length]
            logger.warning(
                "Input content truncated to fit context window",
                extra={
                    "article_id": article_id,
                    "original_length": len(cleaned_content),
                    "truncated_length": self.max_length,
                    "max_length": self.max_length,
                }
            )

        return ValidationResult(
            cleaned_content=cleaned_content,
            was_html=was_html,
            was_truncated=was_truncated,
            original_length=original_length,
            warnings=warnings,
        )
