"""
Input sanitization module for tag extraction.
Provides Pydantic-based input validation and sanitization to prevent prompt injection attacks.
"""

import html
import re
import unicodedata
from html.parser import HTMLParser
from urllib.parse import urlparse

import bleach
import structlog
from pydantic import BaseModel, Field, field_validator

SCRIPT_LIKE_ELEMENTS = frozenset({"script", "style", "iframe", "object", "embed"})
WHITESPACE_PATTERN = re.compile(r"\s+")
DANGEROUS_ELEMENT_PATTERN = re.compile(
    r"<(script|style|iframe|object|embed)\b[^>]*>.*?</\1\s*>",
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

_ADVANCED_PROMPT_INJECTION_PATTERNS = tuple(
    re.compile(pattern, re.IGNORECASE)
    for pattern in (
        r"you\s+are\s+now\s+a\s+",
        r"pretend\s+to\s+be\s+",
        r"act\s+as\s+(?:if\s+you\s+were\s+)?a\s+",
        r"imagine\s+you\s+are\s+",
        r"ignore\s+(?:all\s+)?previous\s+instructions",
        r"disregard\s+(?:all\s+)?previous\s+",
        r"forget\s+(?:all\s+)?previous\s+",
        r"override\s+(?:all\s+)?previous\s+",
        r"system\s*:\s*",
        r"human\s*:\s*",
        r"assistant\s*:\s*",
        r"ai\s*:\s*",
        r"jailbreak",
        r"prompt\s+injection",
        r"escape\s+(?:the\s+)?system",
    )
)


class _DangerousElementStripper(HTMLParser):
    """HTML parser that removes dangerous elements and their contents."""

    def __init__(self, blocked_tags: frozenset[str]):
        super().__init__(convert_charrefs=False)
        self._blocked_tags = {tag.lower() for tag in blocked_tags}
        self._skip_stack: list[str] = []
        self._parts: list[str] = []
        self.dropped_tags: set[str] = set()
        self.had_unclosed_dangerous_tag = False

    def handle_starttag(self, tag: str, attrs):  # type: ignore[override]
        tag_lower = tag.lower()
        if tag_lower in self._blocked_tags:
            if not self._skip_stack:
                self._parts.append(" ")
            self._skip_stack.append(tag_lower)
            self.dropped_tags.add(tag_lower)
            return

        if self._skip_stack:
            return

        self._parts.append(self._serialize_tag(tag, attrs, self_closing=False))

    def handle_startendtag(self, tag: str, attrs):  # type: ignore[override]
        tag_lower = tag.lower()
        if tag_lower in self._blocked_tags:
            if not self._skip_stack:
                self._parts.append(" ")
            self.dropped_tags.add(tag_lower)
            return

        if self._skip_stack:
            return

        self._parts.append(self._serialize_tag(tag, attrs, self_closing=True))

    def handle_endtag(self, tag: str):  # type: ignore[override]
        tag_lower = tag.lower()
        if tag_lower in self._blocked_tags:
            if self._skip_stack:
                for index in range(len(self._skip_stack) - 1, -1, -1):
                    if self._skip_stack[index] == tag_lower:
                        del self._skip_stack[index:]
                        break
            return

        if self._skip_stack:
            return

        self._parts.append(f"</{tag}>")

    def handle_data(self, data: str):  # type: ignore[override]
        if not self._skip_stack:
            self._parts.append(data)

    def handle_entityref(self, name: str):  # type: ignore[override]
        if not self._skip_stack:
            self._parts.append(f"&{name};")

    def handle_charref(self, name: str):  # type: ignore[override]
        if not self._skip_stack:
            self._parts.append(f"&#{name};")

    def handle_comment(self, data: str):  # type: ignore[override]
        if not self._skip_stack:
            self._parts.append(f"<!--{data}-->")

    def handle_decl(self, decl: str):  # type: ignore[override]
        if not self._skip_stack:
            self._parts.append(f"<!{decl}>")

    def handle_pi(self, data: str):  # type: ignore[override]
        if not self._skip_stack:
            self._parts.append(f"<?{data}>")

    def close(self) -> None:  # type: ignore[override]
        super().close()
        if self._skip_stack:
            self.had_unclosed_dangerous_tag = True
            self._skip_stack.clear()

    def get_html(self) -> str:
        return "".join(self._parts)

    @staticmethod
    def _serialize_tag(tag: str, attrs, self_closing: bool) -> str:
        attr_fragments = []
        for name, value in attrs:
            if value is None:
                attr_fragments.append(name)
            else:
                attr_fragments.append(f'{name}="{html.escape(value, quote=True)}"')

        attr_segment = ""
        if attr_fragments:
            attr_segment = " " + " ".join(attr_fragments)

        if self_closing:
            return f"<{tag}{attr_segment} />"
        return f"<{tag}{attr_segment}>"


logger = structlog.get_logger(__name__)


class SanitizationConfig(BaseModel):
    """Configuration for input sanitization."""

    max_title_length: int = 1000
    max_content_length: int = 50000
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
    content: str = Field(..., min_length=1, max_length=50000)
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
        # Check for prompt injection patterns
        if cls._contains_prompt_injection(v):
            raise ValueError("Potential prompt injection detected")
        return v

    @field_validator("content", mode="before")
    def validate_content(cls, v):
        """Validate content."""
        if not v or len(v.strip()) == 0:
            raise ValueError("Content too short")
        if len(v) > 50000:
            raise ValueError("Content too long")
        # Check for control characters
        if any(ord(c) < 32 and c not in "\t\n\r" for c in v):
            raise ValueError("Contains control characters")
        # Check for prompt injection patterns
        if cls._contains_prompt_injection(v):
            raise ValueError("Potential prompt injection detected")
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
    def _contains_prompt_injection(text: str) -> bool:
        """Check if text contains prompt injection patterns."""
        # Convert to lowercase for case-insensitive matching
        text_lower = text.lower()

        # Common prompt injection patterns
        injection_patterns = [
            "ignore previous instructions",
            "system:",
            "human:",
            "assistant:",
            "act as if you were",
            "pretend to be",
            "you are now",
            "forget everything",
            "disregard",
            "override",
            "new instructions",
            "ignore all previous",
            "system prompt",
            "jailbreak",
            "prompt injection",
        ]

        return any(pattern in text_lower for pattern in injection_patterns)

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
        self._dangerous_element_tags = SCRIPT_LIKE_ELEMENTS

        # Configure bleach for HTML sanitization
        self.allowed_tags = (
            [
                "p",
                "br",
                "strong",
                "em",
                "b",
                "i",
                "u",
                "a",
                "ul",
                "ol",
                "li",
                "h1",
                "h2",
                "h3",
                "h4",
                "h5",
                "h6",
                "blockquote",
                "code",
                "pre",
            ]
            if self.config.allow_html
            else []
        )

        self.allowed_attributes = (
            {
                "a": ["href", "title"],
                "img": ["src", "alt", "title"],
            }
            if self.config.allow_html
            else {}
        )

        # Initialize a single Cleaner instance to avoid re-parsing rules per call
        # - Disallow "javascript:" and other non-http(s) protocols explicitly
        # - Strip comments to remove potential payloads hidden in comments
        allowed_protocols = ["http", "https", "mailto"] if self.config.allow_html else ["http", "https"]
        self._cleaner = bleach.Cleaner(
            tags=self.allowed_tags,
            attributes=self.allowed_attributes,
            strip=True,
            strip_comments=True,
            protocols=allowed_protocols,
        )

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

                # Check for prompt injection patterns
                if ArticleInput._contains_prompt_injection(title) or ArticleInput._contains_prompt_injection(content):
                    raise ValueError("Potential prompt injection detected")

                # URL validation if provided
                if url and len(url) > 2048:
                    raise ValueError("URL too long")
                if url and not ArticleInput._is_valid_url(url):
                    raise ValueError("Invalid URL format")

            except ValueError as e:
                violations.append(str(e))
                logger.warning("Input validation failed", error=str(e))
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
                logger.warning("Sanitization violations detected", violations=violations)
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
        # Remove high-risk elements entirely before running the standard cleaner.
        # Bleach strips the tag wrappers, but the embedded payload (e.g. the body of
        # a <script> tag) is kept as plain text.  Since our service forwards the
        # sanitized text to downstream models, we defensively drop the content of
        # scripting and embedded elements altogether.
        text = self._strip_dangerous_elements(text)

        # Clean HTML using Bleach's HTML5 parser and allowlist configuration
        # This avoids brittle regex-based HTML parsing and follows CodeQL guidance
        text = self._cleaner.clean(text)

        # Normalize excessive whitespace post-cleaning
        text = WHITESPACE_PATTERN.sub(" ", text).strip()

        # Remove control characters (except common whitespace)
        text = "".join(char for char in text if ord(char) >= 32 or char in "\t\n\r")

        return text

    def _strip_dangerous_elements(self, text: str) -> str:
        """Remove the contents of script-like elements entirely."""

        if not text:
            return text

        stripper = _DangerousElementStripper(self._dangerous_element_tags)

        try:
            stripper.feed(text)
            stripper.close()
        except Exception as exc:  # pragma: no cover - defensive safeguard
            logger.warning(
                "Failed to strip dangerous elements via HTML parser; falling back to regex removal",
                error=str(exc),
            )
            return WHITESPACE_PATTERN.sub(" ", DANGEROUS_ELEMENT_PATTERN.sub(" ", text))

        if stripper.dropped_tags:
            logger.debug(
                "Removed dangerous HTML elements before bleach cleaning",
                tags=sorted(stripper.dropped_tags),
            )

        if stripper.had_unclosed_dangerous_tag:
            logger.warning("Unterminated dangerous element detected; truncated trailing content")

        return stripper.get_html()

    def _normalize_unicode(self, text: str) -> str:
        """Normalize Unicode text."""
        # Use NFC normalization for consistent representation
        return unicodedata.normalize("NFC", text)

    def _perform_security_checks(self, title: str, content: str) -> list[str]:
        """Perform additional security checks."""
        violations = []

        # Check for remaining prompt injection patterns after sanitization
        combined_text = f"{title} {content}"
        if self._advanced_prompt_injection_check(combined_text):
            violations.append("Advanced prompt injection pattern detected")

        # Check for suspicious patterns
        if self._contains_suspicious_patterns(combined_text):
            violations.append("Suspicious patterns detected")

        return violations

    def _advanced_prompt_injection_check(self, text: str) -> bool:
        """Advanced prompt injection detection."""
        return any(pattern.search(text) for pattern in _ADVANCED_PROMPT_INJECTION_PATTERNS)

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
