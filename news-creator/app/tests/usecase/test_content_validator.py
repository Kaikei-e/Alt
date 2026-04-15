"""Tests for ContentValidator (Phase 4 refactoring).

Following Python 3.14 best practices:
- Dataclass for validation results
- Protocol for structural typing
"""

from __future__ import annotations

import pytest


class TestContentValidator:
    """Tests for ContentValidator class."""

    def test_content_validator_cleans_html(self):
        """ContentValidator should clean HTML from full HTML documents."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(min_length=10, max_length=1000)

        # The html_cleaner only activates for full HTML documents
        # (starting with <!DOCTYPE or <html, or high tag ratio)
        html_content = """<!DOCTYPE html>
        <html><body>
        <p>This is <strong>bold</strong> text.</p>
        <p>More paragraph content here.</p>
        </body></html>"""
        result = validator.validate_and_clean(html_content, article_id="test-123")

        assert "bold" in result.cleaned_content
        assert "<strong>" not in result.cleaned_content
        assert "<p>" not in result.cleaned_content
        assert result.was_html is True

    def test_content_validator_preserves_plain_text(self):
        """ContentValidator should preserve plain text without modification."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(min_length=10, max_length=1000)

        plain_content = "This is plain text without any HTML."
        result = validator.validate_and_clean(plain_content, article_id="test-123")

        assert result.cleaned_content == plain_content
        assert result.was_html is False

    def test_content_validator_rejects_too_short_content(self):
        """ContentValidator should reject content shorter than min_length."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(min_length=100, max_length=1000)

        short_content = "Too short"
        with pytest.raises(ValueError, match="too short"):
            validator.validate_and_clean(short_content, article_id="test-123")

    def test_content_validator_truncates_long_content(self):
        """ContentValidator should truncate content exceeding max_length."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(min_length=10, max_length=100)

        long_content = "A" * 200
        result = validator.validate_and_clean(long_content, article_id="test-123")

        assert len(result.cleaned_content) == 100
        assert result.was_truncated is True
        assert result.original_length == 200

    def test_content_validator_reports_no_truncation(self):
        """ContentValidator should report was_truncated=False when content fits."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(min_length=10, max_length=1000)

        content = "This content fits within the limit."
        result = validator.validate_and_clean(content, article_id="test-123")

        assert result.was_truncated is False

    def test_content_validator_strips_whitespace(self):
        """ContentValidator should strip whitespace from content."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(min_length=10, max_length=1000)

        content = "  \n  Valid content with whitespace  \n  "
        result = validator.validate_and_clean(content, article_id="test-123")

        assert result.cleaned_content == "Valid content with whitespace"

    def test_content_validator_rejects_empty_after_cleaning(self):
        """ContentValidator should reject content that becomes empty after HTML cleaning."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(min_length=10, max_length=1000)

        # Full HTML document that becomes nearly empty after cleaning
        html_only = """<!DOCTYPE html>
        <html><head><title>Empty</title></head>
        <body><div><span></span></div></body></html>"""
        with pytest.raises(ValueError, match="too short|empty"):
            validator.validate_and_clean(html_only, article_id="test-123")

    def test_content_validator_provides_warnings_for_abnormal_size(self):
        """ContentValidator should add warning for abnormally large content."""
        from news_creator.usecase.content_validator import ContentValidator

        validator = ContentValidator(
            min_length=10,
            max_length=1000,
            abnormal_size_threshold=500,
        )

        large_content = "A" * 600
        result = validator.validate_and_clean(large_content, article_id="test-123")

        assert len(result.warnings) > 0
        assert any(
            "abnormal" in w.lower() or "large" in w.lower() for w in result.warnings
        )


class TestValidationResult:
    """Tests for ValidationResult dataclass."""

    def test_validation_result_has_required_fields(self):
        """ValidationResult should have all required fields."""
        from news_creator.usecase.content_validator import ValidationResult

        result = ValidationResult(
            cleaned_content="Test content",
            was_html=False,
            was_truncated=False,
            original_length=12,
            warnings=[],
        )

        assert result.cleaned_content == "Test content"
        assert result.was_html is False
        assert result.was_truncated is False
        assert result.original_length == 12
        assert result.warnings == []

    def test_validation_result_is_frozen(self):
        """ValidationResult should be immutable."""
        from news_creator.usecase.content_validator import ValidationResult

        result = ValidationResult(
            cleaned_content="Test",
            was_html=False,
            was_truncated=False,
            original_length=4,
            warnings=[],
        )

        with pytest.raises(AttributeError):
            result.cleaned_content = "Modified"


class TestContentValidatorDefaults:
    """Tests for ContentValidator default values."""

    def test_content_validator_uses_sensible_defaults(self):
        """ContentValidator should use sensible defaults for summarization."""
        from news_creator.usecase.content_validator import ContentValidator

        # Default should work for typical summarization use case
        validator = ContentValidator()

        # Should have reasonable defaults
        assert validator.min_length == 100  # CDC contract minimum
        assert validator.max_length == 60_000  # ~15K tokens for 16K context

    def test_content_validator_from_config(self, monkeypatch):
        """ContentValidator.from_config() should load from config."""
        from news_creator.usecase.content_validator import ContentValidator
        from news_creator.config.config import NewsCreatorConfig


        config = NewsCreatorConfig()
        validator = ContentValidator.from_config(config)

        assert validator is not None
        assert hasattr(validator, "validate_and_clean")
