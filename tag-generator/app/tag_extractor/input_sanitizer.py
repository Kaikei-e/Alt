"""Input sanitization utilities for tag extraction."""

import re
import unicodedata
from urllib.parse import urlparse

import nh3
import structlog
from pydantic import BaseModel, Field, field_validator

WHITESPACE_PATTERN = re.compile(r"\s+")
DANGEROUS_ELEMENT_PATTERN = re.compile(
    r"<(script|style|iframe|object|embed)\b[^>]*>.*?(?:</\1\s*>|$)",
    re.IGNORECASE | re.DOTALL,
)
URL_PATTERN = re.compile(
    r"^https?://"
    r"(?:(?:[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?\.)+[A-Z]{2,6}\.?|"
    r"localhost|"
    r"\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})"
    r"(?::\d+)?"
    r"(?:/?|[/?]\S+)$",
    re.IGNORECASE,
)

logger = structlog.get_logger(__name__)


class SanitizationConfig(BaseModel):
    """Configuration for input sanitization."""

    max_title_length: int = 1000
    max_content_length: int = 100000
    min_title_length: int = 1
    min_content_length: int = 1
    allow_html: bool = False
    strip_urls: bool = False
    max_url_length: int = 2048


class SanitizationError(Exception):
    """Custom exception for sanitization errors."""

    pass


class ArticleInput(BaseModel):
    """Pydantic model for article input validation."""

    title: str = Field(..., min_length=1, max_length=1000)
    content: str = Field(..., min_length=1, max_length=100000)
    url: str | None = Field(default=None, max_length=2048)

    @field_validator("title", mode="before")
    def validate_title(cls, v):
        """Validate title content."""
        if not v or len(v.strip()) == 0:
            raise ValueError("Title too short")
        if len(v) > 1000:
            raise ValueError("Title too long")
        # Check for control characters
        if any(ord(c) < 32 and c not in "\t\n\r" for c in v):
            raise ValueError("Contains control characters")
        return v

    @field_validator("content", mode="before")
    def validate_content(cls, v):
        """Validate content."""
        if not v or len(v.strip()) == 0:
            raise ValueError("Content too short")
        if len(v) > 100000:
            raise ValueError("Content too long")
        # Check for control characters
        if any(ord(c) < 32 and c not in "\t\n\r" for c in v):
            raise ValueError("Contains control characters")
        return v

    @field_validator("url", mode="before")
    def validate_url(cls, v):
        """Validate URL format."""
        if v is None:
            return v
        try:
            parsed = urlparse(v)
            if not parsed.scheme or not parsed.netloc:
                raise ValueError("Invalid URL format")
        except Exception as e:
            raise ValueError("Invalid URL format") from e
        return v

    @staticmethod
    def _is_valid_url(url: str) -> bool:
        """Check if URL is valid."""
        return bool(URL_PATTERN.fullmatch(url))


class SanitizedArticleInput(BaseModel):
    """Sanitized and validated article input."""

    title: str
    content: str
    url: str | None = None
    original_length: int
    sanitized_length: int
    normalized: bool = False


class SanitizationResult(BaseModel):
    """Result of input sanitization."""

    is_valid: bool
    sanitized_input: SanitizedArticleInput | None
    violations: list[str]
    warnings: list[str] = []


class InputSanitizer:
    """Main input sanitization class."""

    def __init__(self, config: SanitizationConfig | None = None):
        self.config = config or SanitizationConfig()

        self.allowed_tags: tuple[str, ...] = (
            (
                "a",
                "blockquote",
                "br",
                "code",
                "em",
                "h1",
                "h2",
                "h3",
                "h4",
                "h5",
                "h6",
                "i",
                "li",
                "ol",
                "p",
                "pre",
                "strong",
                "u",
                "ul",
            )
            if self.config.allow_html
            else ()
        )

        self.allowed_attributes: dict[str, tuple[str, ...]] = {"a": ("href", "title")} if self.config.allow_html else {}

        allowed_protocols: tuple[str, ...] = (
            ("http", "https", "mailto") if self.config.allow_html else ("http", "https")
        )

        # Pre-compute immutable sanitizer configuration so each call can reuse it with
        # ``nh3.clean`` without repeatedly constructing sanitizer objects.
        self._nh3_tags: set[str] = set(self.allowed_tags)
        self._nh3_attributes: dict[str, set[str]] = {tag: set(attrs) for tag, attrs in self.allowed_attributes.items()}
        self._nh3_url_schemes: set[str] = set(allowed_protocols)
        # We do not preserve the inner text of stripped scripting and embedding tags.
        self._nh3_clean_content_tags: set[str] = set()

    def sanitize(self, title: str, content: str, url: str | None = None) -> SanitizationResult:
        """
        Sanitize input text and return sanitization result.

        Args:
            title: Article title
            content: Article content
            url: Optional article URL

        Returns:
            SanitizationResult with validation status and sanitized input
        """
        violations = []
        warnings = []

        # Store original lengths
        original_title_length = len(title)
        original_content_length = len(content)
        original_total_length = original_title_length + original_content_length

        try:
            # Step 1: Basic validation using config limits instead of hardcoded Pydantic model
            try:
                # Manual validation using config
                if not title or len(title.strip()) == 0:
                    raise ValueError("Title too short")
                if len(title) < self.config.min_title_length:
                    raise ValueError("Title too short")
                if len(title) > self.config.max_title_length:
                    raise ValueError("Title too long")

                if not content or len(content.strip()) == 0:
                    raise ValueError("Content too short")
                if len(content) < self.config.min_content_length:
                    raise ValueError("Content too short")
                if len(content) > self.config.max_content_length:
                    raise ValueError("Content too long")

                # Check for control characters in title
                if any(ord(c) < 32 and c not in "\t\n\r" for c in title):
                    raise ValueError("Contains control characters")

                # Check for control characters in content
                if any(ord(c) < 32 and c not in "\t\n\r" for c in content):
                    raise ValueError("Contains control characters")

                # URL validation if provided
                if url and len(url) > 2048:
                    raise ValueError("URL too long")
                if url and not ArticleInput._is_valid_url(url):
                    raise ValueError("Invalid URL format")

            except ValueError as e:
                violations.append(str(e))
                return SanitizationResult(
                    is_valid=False,
                    sanitized_input=None,
                    violations=violations,
                    warnings=warnings,
                )

            # Step 2: Sanitize content
            sanitized_title = self._sanitize_text(title)
            sanitized_content = self._sanitize_text(content)

            # Step 3: Normalize Unicode
            sanitized_title = self._normalize_unicode(sanitized_title)
            sanitized_content = self._normalize_unicode(sanitized_content)

            # Step 4: Additional security checks
            security_violations = self._perform_security_checks(sanitized_title, sanitized_content)
            violations.extend(security_violations)

            # Step 5: Final validation
            if violations:
                return SanitizationResult(
                    is_valid=False,
                    sanitized_input=None,
                    violations=violations,
                    warnings=warnings,
                )

            # Create sanitized result
            sanitized_input = SanitizedArticleInput(
                title=sanitized_title,
                content=sanitized_content,
                url=url,
                original_length=original_total_length,
                sanitized_length=len(sanitized_title) + len(sanitized_content),
                normalized=True,
            )

            return SanitizationResult(
                is_valid=True,
                sanitized_input=sanitized_input,
                violations=[],
                warnings=warnings,
            )

        except Exception as e:
            logger.error("Sanitization failed", error=str(e))
            violations.append(f"Sanitization error: {str(e)}")
            return SanitizationResult(
                is_valid=False,
                sanitized_input=None,
                violations=violations,
                warnings=warnings,
            )

    def _sanitize_text(self, text: str) -> str:
        """Sanitize text content."""
        if not text:
            return ""

        # Drop high-risk embedded content before invoking ``nh3`` so payloads from
        # script-like tags never reach downstream consumers as plain text.
        text = DANGEROUS_ELEMENT_PATTERN.sub(" ", text)

        text = nh3.clean(
            text,
            tags=self._nh3_tags,
            attributes=self._nh3_attributes or None,
            url_schemes=self._nh3_url_schemes or None,
            clean_content_tags=self._nh3_clean_content_tags,
            strip_comments=True,
        )

        # Normalize excessive whitespace post-cleaning
        text = WHITESPACE_PATTERN.sub(" ", text).strip()

        # Remove control characters (except common whitespace)
        text = "".join(char for char in text if ord(char) >= 32 or char in "\t\n\r")

        return text

    def _normalize_unicode(self, text: str) -> str:
        """Normalize Unicode text."""
        # Use NFC normalization for consistent representation
        return unicodedata.normalize("NFC", text)

    def _perform_security_checks(self, title: str, content: str) -> list[str]:
        """Perform additional security checks."""
        violations = []

        # Check for suspicious patterns
        combined_text = f"{title} {content}"
        if self._contains_suspicious_patterns(combined_text):
            violations.append("Suspicious patterns detected")

        return violations

    def _contains_suspicious_patterns(self, text: str) -> bool:
        """Check for other suspicious patterns."""
        # Check for excessive repetition (potential DoS)
        if len(set(text.lower().split())) < len(text.split()) * 0.1:
            return True

        # Check for unusual character frequency
        if self._has_unusual_character_frequency(text):
            return True

        return False

    def _has_unusual_character_frequency(self, text: str) -> bool:
        """Check for unusual character frequency patterns."""
        if not text:
            return False

        # Check for excessive special characters
        special_chars = sum(1 for c in text if not c.isalnum() and not c.isspace())
        if special_chars > len(text) * 0.3:  # More than 30% special characters
            return True

        # Check for excessive uppercase
        uppercase_chars = sum(1 for c in text if c.isupper())
        if uppercase_chars > len(text) * 0.7:  # More than 70% uppercase
            return True

        return False
