"""HTML cleaning utilities for article content using bleach."""

import logging
import re
from typing import Tuple

import bleach

logger = logging.getLogger(__name__)


def clean_html_content(content: str, article_id: str = "") -> Tuple[str, bool]:
    """
    Remove HTML tags and extract text content from HTML using bleach.

    Args:
        content: Content that may contain HTML
        article_id: Article ID for logging (optional)

    Returns:
        Tuple of (cleaned_text, was_html)
        - cleaned_text: Text content with HTML removed
        - was_html: True if HTML was detected and removed
    """
    if not content:
        return "", False

    original_length = len(content)

    # Check if content appears to be HTML
    # Look for HTML doctype, html tags, or high ratio of HTML tags
    html_indicators = [
        content.strip().startswith('<!doctype'),
        content.strip().startswith('<!DOCTYPE'),
        content.strip().startswith('<html'),
        content.strip().startswith('<HTML'),
    ]

    # Count HTML tags
    html_tags = re.findall(r'<[^>]+>', content)
    html_tag_count = len(html_tags)
    html_ratio = (len(''.join(html_tags)) / len(content)) if content else 0.0

    is_html = any(html_indicators) or (html_ratio > 0.3 and html_tag_count > 50)

    if not is_html:
        # Not HTML, return as-is
        return content, False

    # Log HTML detection
    logger.warning(
        "HTML content detected, cleaning with bleach",
        extra={
            "article_id": article_id,
            "original_length": original_length,
            "html_tag_count": html_tag_count,
            "html_ratio": round(html_ratio * 100, 2),
        }
    )

    try:
        # Use bleach to strip all HTML tags and get text content
        # bleach.clean with tags=[] removes all tags
        cleaned = bleach.clean(content, tags=[], strip=True)

        # Remove HTML entities that might remain
        cleaned = bleach.clean(cleaned, tags=[], strip=True)

        # Remove excessive whitespace
        cleaned = re.sub(r'\s+', ' ', cleaned)

        # Remove leading/trailing whitespace
        cleaned = cleaned.strip()

        # Remove common HTML artifacts that might remain
        # Remove CSS-like patterns (property:value)
        cleaned = re.sub(r'\b[a-zA-Z-]+:\s*[^;]+;?', ' ', cleaned)

        # Remove URL-like patterns that are likely CSS/JS references
        cleaned = re.sub(r'https?://[^\s]+', ' ', cleaned)

        # Remove repeated punctuation or special characters
        cleaned = re.sub(r'[^\w\s\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]{3,}', ' ', cleaned)

        # Final whitespace cleanup
        cleaned = re.sub(r'\s+', ' ', cleaned).strip()

        cleaned_length = len(cleaned)
        reduction_ratio = (1 - (cleaned_length / original_length)) * 100 if original_length > 0 else 0

        logger.info(
            "HTML content cleaned with bleach",
            extra={
                "article_id": article_id,
                "original_length": original_length,
                "cleaned_length": cleaned_length,
                "reduction_ratio": round(reduction_ratio, 2),
            }
        )

        # If cleaned content is too short (less than 10% of original), it might be mostly HTML
        if cleaned_length < original_length * 0.1:
            logger.warning(
                "Cleaned content is very short, may indicate mostly HTML",
                extra={
                    "article_id": article_id,
                    "original_length": original_length,
                    "cleaned_length": cleaned_length,
                    "cleaned_preview": cleaned[:200],
                }
            )

        return cleaned, True

    except Exception as e:
        logger.error(
            "Failed to clean HTML with bleach, falling back to regex",
            extra={
                "article_id": article_id,
                "error": str(e),
            }
        )
        # Fallback to regex-based cleaning
        cleaned = re.sub(r'<[^>]+>', ' ', content)
        cleaned = re.sub(r'&[a-zA-Z0-9#]+;', ' ', cleaned)
        cleaned = re.sub(r'\s+', ' ', cleaned).strip()
        return cleaned, True

