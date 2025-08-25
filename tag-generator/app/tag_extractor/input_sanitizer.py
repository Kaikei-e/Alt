"""
Input sanitization module for tag extraction.
Provides Pydantic-based input validation and sanitization to prevent prompt injection attacks.
"""

import re
import unicodedata
from typing import List, Optional, Union
from urllib.parse import urlparse

import bleach
import structlog
from pydantic import BaseModel, Field, validator, root_validator, field_validator

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
    url: Optional[str] = Field(None, max_length=2048)

    @field_validator('title', mode="before")
    def validate_title(cls, v):
        """Validate title content."""
        if not v or len(v.strip()) == 0:
            raise ValueError("Title too short")
        if len(v) > 1000:
            raise ValueError("Title too long")
        # Check for control characters
        if any(ord(c) < 32 and c not in '\t\n\r' for c in v):
            raise ValueError("Contains control characters")
        # Check for prompt injection patterns
        if cls._contains_prompt_injection(v):
            raise ValueError("Potential prompt injection detected")
        return v

    @field_validator('content', mode="before")
    def validate_content(cls, v):
        """Validate content."""
        if not v or len(v.strip()) == 0:
            raise ValueError("Content too short")
        if len(v) > 50000:
            raise ValueError("Content too long")
        # Check for control characters
        if any(ord(c) < 32 and c not in '\t\n\r' for c in v):
            raise ValueError("Contains control characters")
        # Check for prompt injection patterns
        if cls._contains_prompt_injection(v):
            raise ValueError("Potential prompt injection detected")
        return v

    @field_validator('url', mode="before")
    def validate_url(cls, v):
        """Validate URL format."""
        if v is None:
            return v
        try:
            parsed = urlparse(v)
            if not parsed.scheme or not parsed.netloc:
                raise ValueError("Invalid URL format")
        except Exception:
            raise ValueError("Invalid URL format")
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
            "prompt injection"
        ]

        return any(pattern in text_lower for pattern in injection_patterns)


class SanitizedArticleInput(BaseModel):
    """Sanitized and validated article input."""

    title: str
    content: str
    url: Optional[str] = None
    original_length: int
    sanitized_length: int
    normalized: bool = False


class SanitizationResult(BaseModel):
    """Result of input sanitization."""

    is_valid: bool
    sanitized_input: Optional[SanitizedArticleInput]
    violations: List[str]
    warnings: List[str] = []


class InputSanitizer:
    """Main input sanitization class."""

    def __init__(self, config: Optional[SanitizationConfig] = None):
        self.config = config or SanitizationConfig()

        # Configure bleach for HTML sanitization
        self.allowed_tags = [
            'p', 'br', 'strong', 'em', 'b', 'i', 'u', 'a', 'ul', 'ol', 'li',
            'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'blockquote', 'code', 'pre'
        ] if self.config.allow_html else []

        self.allowed_attributes = {
            'a': ['href', 'title'],
            'img': ['src', 'alt', 'title'],
        } if self.config.allow_html else {}

    def sanitize(self, title: str, content: str, url: Optional[str] = None) -> SanitizationResult:
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
            # Step 1: Basic validation using Pydantic model
            try:
                validated_input = ArticleInput(title=title, content=content, url=url)
            except ValueError as e:
                violations.append(str(e))
                logger.warning("Input validation failed", error=str(e))
                return SanitizationResult(
                    is_valid=False,
                    sanitized_input=None,
                    violations=violations,
                    warnings=warnings
                )

            # Step 2: Sanitize content
            sanitized_title = self._sanitize_text(validated_input.title)
            sanitized_content = self._sanitize_text(validated_input.content)

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
                    warnings=warnings
                )

            # Create sanitized result
            sanitized_input = SanitizedArticleInput(
                title=sanitized_title,
                content=sanitized_content,
                url=url,
                original_length=original_total_length,
                sanitized_length=len(sanitized_title) + len(sanitized_content),
                normalized=True
            )

            return SanitizationResult(
                is_valid=True,
                sanitized_input=sanitized_input,
                violations=[],
                warnings=warnings
            )

        except Exception as e:
            logger.error("Sanitization failed", error=str(e))
            violations.append(f"Sanitization error: {str(e)}")
            return SanitizationResult(
                is_valid=False,
                sanitized_input=None,
                violations=violations,
                warnings=warnings
            )

    def _sanitize_text(self, text: str) -> str:
        """Sanitize text content."""
        # First, completely remove dangerous script/style content (not just tags)
        # This removes both tags and their content
        text = re.sub(r'<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>', '', text, flags=re.IGNORECASE | re.DOTALL)
        text = re.sub(r'<style\b[^<]*(?:(?!<\/style>)<[^<]*)*<\/style>', '', text, flags=re.IGNORECASE | re.DOTALL)
        
        # Remove excessive whitespace
        text = re.sub(r'\s+', ' ', text).strip()

        # Remove or clean remaining HTML based on config
        if self.config.allow_html:
            text = bleach.clean(
                text,
                tags=self.allowed_tags,
                attributes=self.allowed_attributes,
                strip=True
            )
        else:
            text = bleach.clean(text, tags=[], attributes={}, strip=True)

        # Remove control characters (except common whitespace)
        text = ''.join(char for char in text if ord(char) >= 32 or char in '\t\n\r')

        return text

    def _normalize_unicode(self, text: str) -> str:
        """Normalize Unicode text."""
        # Use NFC normalization for consistent representation
        return unicodedata.normalize('NFC', text)

    def _perform_security_checks(self, title: str, content: str) -> List[str]:
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
        text_lower = text.lower()

        # More sophisticated patterns
        advanced_patterns = [
            # Role-playing attempts
            r'you\s+are\s+now\s+a\s+',
            r'pretend\s+to\s+be\s+',
            r'act\s+as\s+(?:if\s+you\s+were\s+)?a\s+',
            r'imagine\s+you\s+are\s+',

            # Instruction override attempts
            r'ignore\s+(?:all\s+)?previous\s+instructions',
            r'disregard\s+(?:all\s+)?previous\s+',
            r'forget\s+(?:all\s+)?previous\s+',
            r'override\s+(?:all\s+)?previous\s+',

            # System prompt manipulation
            r'system\s*:\s*',
            r'human\s*:\s*',
            r'assistant\s*:\s*',
            r'ai\s*:\s*',

            # Jailbreak attempts
            r'jailbreak',
            r'prompt\s+injection',
            r'escape\s+(?:the\s+)?system',
        ]

        return any(re.search(pattern, text_lower) for pattern in advanced_patterns)

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