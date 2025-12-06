import logging
import trafilatura

# Suppress noisy trafilatura logs
logging.getLogger("trafilatura").setLevel(logging.CRITICAL)

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
        # Improved validation: check for empty, whitespace-only, or too short content
        if not html or not html.strip() or len(html.strip()) < 10:
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
        except Exception:
            # Squelch all errors to avoid log spam, return empty as fallback logic exists in worker
            return ""
