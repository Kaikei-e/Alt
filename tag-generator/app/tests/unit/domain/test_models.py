"""TDD RED phase: Tests for domain models.

These tests define the expected behavior of domain models before implementation.
"""

from datetime import UTC, datetime

import pytest

from tag_extractor.extract import TagExtractionOutcome
from tag_generator.domain.errors import (
    BatchProcessingError,
    CursorError,
    DatabaseConnectionError,
    ModelLoadError,
    TagExtractionError,
    TagGeneratorError,
)
from tag_generator.domain.models import Article, BatchResult, CursorPosition, Tag, TagExtractionResult


class TestArticle:
    """Tests for the Article domain model."""

    def test_create_article_with_required_fields(self) -> None:
        article = Article(
            id="550e8400-e29b-41d4-a716-446655440000",
            title="Machine Learning Tutorial",
            content="This is a comprehensive tutorial about ML.",
            created_at="2025-01-01T00:00:00+00:00",
        )
        assert article.id == "550e8400-e29b-41d4-a716-446655440000"
        assert article.title == "Machine Learning Tutorial"
        assert article.content == "This is a comprehensive tutorial about ML."
        assert article.created_at == "2025-01-01T00:00:00+00:00"
        assert article.feed_id is None
        assert article.url is None

    def test_create_article_with_all_fields(self) -> None:
        article = Article(
            id="550e8400-e29b-41d4-a716-446655440000",
            title="Test",
            content="Content",
            created_at="2025-01-01T00:00:00+00:00",
            feed_id="feed-uuid-123",
            url="https://example.com/article",
        )
        assert article.feed_id == "feed-uuid-123"
        assert article.url == "https://example.com/article"

    def test_article_is_frozen(self) -> None:
        article = Article(
            id="test-id",
            title="Test",
            content="Content",
            created_at="2025-01-01T00:00:00+00:00",
        )
        with pytest.raises(AttributeError, match="cannot assign to field 'title'"):
            article.title = "Modified"  # type: ignore[misc]

    def test_article_from_dict(self) -> None:
        raw = {
            "id": "test-id",
            "title": "Title",
            "content": "Content",
            "created_at": "2025-01-01T00:00:00+00:00",
            "feed_id": "feed-1",
            "url": "https://example.com",
        }
        article = Article.from_dict(raw)
        assert article.id == "test-id"
        assert article.feed_id == "feed-1"

    def test_article_from_dict_with_datetime_created_at(self) -> None:
        raw = {
            "id": "test-id",
            "title": "Title",
            "content": "Content",
            "created_at": datetime(2025, 1, 1, tzinfo=UTC),
        }
        article = Article.from_dict(raw)
        assert isinstance(article.created_at, str)

    def test_article_from_dict_missing_optional_fields(self) -> None:
        raw = {
            "id": "test-id",
            "title": "Title",
            "content": "Content",
            "created_at": "2025-01-01T00:00:00+00:00",
        }
        article = Article.from_dict(raw)
        assert article.feed_id is None
        assert article.url is None

    def test_article_to_dict(self) -> None:
        article = Article(
            id="test-id",
            title="Title",
            content="Content",
            created_at="2025-01-01T00:00:00+00:00",
            feed_id="feed-1",
            url="https://example.com",
        )
        d = article.to_dict()
        assert d["id"] == "test-id"
        assert d["title"] == "Title"
        assert d["feed_id"] == "feed-1"


class TestTag:
    """Tests for the Tag domain model."""

    def test_create_tag(self) -> None:
        expected_confidence = 0.85
        tag = Tag(name="machine-learning", confidence=expected_confidence)
        assert tag.name == "machine-learning"
        assert tag.confidence == expected_confidence

    def test_tag_is_frozen(self) -> None:
        tag = Tag(name="test", confidence=0.5)
        with pytest.raises(AttributeError, match="cannot assign to field 'name'"):
            tag.name = "modified"  # type: ignore[misc]

    def test_tag_confidence_bounds(self) -> None:
        tag_low = Tag(name="low", confidence=0.0)
        tag_high = Tag(name="high", confidence=1.0)
        assert tag_low.confidence == 0.0
        assert tag_high.confidence == 1.0


class TestTagExtractionResult:
    """Tests for the TagExtractionResult domain model."""

    def test_create_extraction_result(self) -> None:
        expected_tags = [Tag(name="ml", confidence=0.9), Tag(name="ai", confidence=0.8)]
        expected_inference_ms = 45.2
        expected_confidence = 0.85
        result = TagExtractionResult(
            article_id="article-1",
            tags=expected_tags,
            language="en",
            inference_ms=expected_inference_ms,
            overall_confidence=expected_confidence,
        )
        assert result.article_id == "article-1"
        assert len(result.tags) == len(expected_tags)
        assert result.language == "en"
        assert result.inference_ms == expected_inference_ms
        assert result.overall_confidence == expected_confidence

    def test_extraction_result_tag_names(self) -> None:
        result = TagExtractionResult(
            article_id="article-1",
            tags=[Tag(name="ml", confidence=0.9), Tag(name="ai", confidence=0.8)],
            language="en",
            inference_ms=10.0,
            overall_confidence=0.85,
        )
        assert result.tag_names == ["ml", "ai"]

    def test_extraction_result_tag_confidences_dict(self) -> None:
        result = TagExtractionResult(
            article_id="article-1",
            tags=[Tag(name="ml", confidence=0.9), Tag(name="ai", confidence=0.8)],
            language="en",
            inference_ms=10.0,
            overall_confidence=0.85,
        )
        assert result.tag_confidences == {"ml": 0.9, "ai": 0.8}

    def test_extraction_result_is_empty(self) -> None:
        empty = TagExtractionResult(
            article_id="article-1",
            tags=[],
            language="en",
            inference_ms=5.0,
            overall_confidence=0.0,
        )
        assert empty.is_empty

    def test_extraction_result_not_empty(self) -> None:
        result = TagExtractionResult(
            article_id="article-1",
            tags=[Tag(name="ml", confidence=0.9)],
            language="en",
            inference_ms=10.0,
            overall_confidence=0.7,
        )
        assert not result.is_empty

    def test_extraction_result_from_outcome(self) -> None:
        """Test converting from TagExtractionOutcome to domain model."""
        outcome = TagExtractionOutcome(
            tags=["ml", "ai"],
            confidence=0.85,
            tag_count=2,
            inference_ms=42.0,
            language="en",
            model_name="test-model",
            sanitized_length=500,
            tag_confidences={"ml": 0.9, "ai": 0.8},
        )
        result = TagExtractionResult.from_outcome("article-1", outcome)
        assert result.article_id == "article-1"
        assert len(result.tags) == len(outcome.tags)
        assert result.tags[0].name == "ml"
        assert outcome.tag_confidences is not None
        assert result.tags[0].confidence == outcome.tag_confidences["ml"]
        assert result.language == "en"
        assert result.inference_ms == outcome.inference_ms
        assert result.overall_confidence == outcome.confidence


class TestBatchResult:
    """Tests for the BatchResult domain model."""

    def test_default_batch_result(self) -> None:
        result = BatchResult()
        assert result.total_processed == 0
        assert result.successful == 0
        assert result.failed == 0
        assert result.has_more_pending is False

    def test_batch_result_with_values(self) -> None:
        expected_total = 100
        expected_successful = 95
        expected_failed = 5
        result = BatchResult(
            total_processed=expected_total,
            successful=expected_successful,
            failed=expected_failed,
            has_more_pending=True,
        )
        assert result.total_processed == expected_total
        assert result.successful == expected_successful
        assert result.failed == expected_failed
        assert result.has_more_pending is True

    def test_batch_result_is_mutable(self) -> None:
        expected_total = 10
        result = BatchResult()
        result.total_processed = expected_total
        result.successful = 8
        result.failed = 2
        assert result.total_processed == expected_total

    def test_batch_result_to_dict(self) -> None:
        result = BatchResult(total_processed=10, successful=8, failed=2, has_more_pending=True)
        d = result.to_dict()
        assert d == {
            "total_processed": 10,
            "successful": 8,
            "failed": 2,
            "has_more_pending": True,
        }

    def test_batch_result_success_property(self) -> None:
        # Success: no failures
        assert BatchResult(total_processed=10, successful=10, failed=0).is_success
        # Success: nothing processed
        assert BatchResult(total_processed=0, successful=0, failed=0).is_success
        # Failure: some failures
        assert not BatchResult(total_processed=10, successful=5, failed=5).is_success


class TestCursorPosition:
    """Tests for the CursorPosition domain model."""

    def test_create_cursor_position(self) -> None:
        cursor = CursorPosition(
            created_at="2025-01-01T00:00:00+00:00",
            article_id="550e8400-e29b-41d4-a716-446655440000",
        )
        assert cursor.created_at == "2025-01-01T00:00:00+00:00"
        assert cursor.article_id == "550e8400-e29b-41d4-a716-446655440000"

    def test_cursor_position_is_frozen(self) -> None:
        cursor = CursorPosition(created_at="2025-01-01T00:00:00+00:00", article_id="test-id")
        with pytest.raises(AttributeError, match="cannot assign to field 'created_at'"):
            cursor.created_at = "modified"  # type: ignore[misc]


class TestDomainErrors:
    """Tests for domain error hierarchy."""

    def test_error_hierarchy(self) -> None:
        assert issubclass(TagExtractionError, TagGeneratorError)
        assert issubclass(ModelLoadError, TagGeneratorError)
        assert issubclass(BatchProcessingError, TagGeneratorError)
        assert issubclass(DatabaseConnectionError, TagGeneratorError)
        assert issubclass(CursorError, TagGeneratorError)

    def test_errors_are_exceptions(self) -> None:
        assert issubclass(TagGeneratorError, Exception)
