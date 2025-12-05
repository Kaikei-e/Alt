import trafilatura
from typing import Optional

class ContentExtractor:
    """Service for extracting main content from HTML using trafilatura."""

    def extract_content(self, html: str, include_comments: bool = False) -> str:
        """
        Extract main content from HTML string.

        Args:
            html: Raw HTML string
            include_comments: Whether to include comments in extraction

        Returns:
            Extracted text content or empty string if extraction fails
        """
        if not html:
            return ""

        try:
            # Trafilatura handles boilerplate removal and main content extraction
            text = trafilatura.extract(
                html,
                include_comments=include_comments,
                include_tables=False,
                no_fallback=False
            )
            return text if text else ""
        except Exception as e:
            # Log error if logging is available, otherwise just return empty
            # In production, we'd want proper logging here
            print(f"Trafilatura extraction failed: {e}")
            return ""
