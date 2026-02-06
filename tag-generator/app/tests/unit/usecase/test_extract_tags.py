"""TDD RED phase: Tests for ExtractTagsUsecase."""

from dataclasses import dataclass, field
from unittest.mock import MagicMock

import pytest


@dataclass
class FakeOutcome:
    tags: list[str] = field(default_factory=lambda: ["ml", "ai"])
    confidence: float = 0.85
    tag_count: int = 2
    inference_ms: float = 42.0
    language: str = "en"
    model_name: str = "test-model"
    sanitized_length: int = 500
    tag_confidences: dict = field(default_factory=lambda: {"ml": 0.9, "ai": 0.8})
    embedding_backend: str = "test"
    embedding_metadata: dict = field(default_factory=dict)


class TestExtractTagsUsecase:
    def test_execute_returns_extraction_result(self):
        from tag_generator.domain.models import TagExtractionResult
        from tag_generator.usecase.extract_tags import ExtractTagsUsecase

        mock_extractor = MagicMock()
        mock_extractor.extract_tags_with_metrics.return_value = FakeOutcome()

        usecase = ExtractTagsUsecase(mock_extractor)
        result = usecase.execute("article-1", "Title", "Content text")

        assert isinstance(result, TagExtractionResult)
        assert result.article_id == "article-1"
        assert result.tag_names == ["ml", "ai"]
        assert result.overall_confidence == 0.85

    def test_execute_returns_empty_when_no_tags(self):
        from tag_generator.usecase.extract_tags import ExtractTagsUsecase

        mock_extractor = MagicMock()
        mock_extractor.extract_tags_with_metrics.return_value = FakeOutcome(
            tags=[], confidence=0.0, tag_count=0, tag_confidences={}
        )

        usecase = ExtractTagsUsecase(mock_extractor)
        result = usecase.execute("article-1", "Title", "Content")

        assert result.is_empty
        assert result.tag_names == []

    def test_execute_calls_extractor_with_title_and_content(self):
        from tag_generator.usecase.extract_tags import ExtractTagsUsecase

        mock_extractor = MagicMock()
        mock_extractor.extract_tags_with_metrics.return_value = FakeOutcome()

        usecase = ExtractTagsUsecase(mock_extractor)
        usecase.execute("article-1", "My Title", "My Content")

        mock_extractor.extract_tags_with_metrics.assert_called_once_with("My Title", "My Content")

    def test_execute_propagates_extraction_error(self):
        from tag_generator.domain.errors import TagExtractionError
        from tag_generator.usecase.extract_tags import ExtractTagsUsecase

        mock_extractor = MagicMock()
        mock_extractor.extract_tags_with_metrics.side_effect = TagExtractionError("model failed")

        usecase = ExtractTagsUsecase(mock_extractor)
        with pytest.raises(TagExtractionError):
            usecase.execute("article-1", "Title", "Content")
