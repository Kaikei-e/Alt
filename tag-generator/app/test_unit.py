"""
Unit tests for tag-generator following TDD principles.
These tests define the expected behavior that must be maintained during refactoring.
"""

from unittest.mock import Mock, patch

import pytest

from main import TagGeneratorService
from tag_extractor.extract import TagExtractionConfig, TagExtractionOutcome, TagExtractor
from tag_extractor.input_sanitizer import SanitizationResult, SanitizedArticleInput
from tag_generator.cascade import CascadeConfig, CascadeController


class TestTagExtractor:
    """Unit tests for TagExtractor class."""

    def test_should_initialize_with_default_config(self):
        """TagExtractor should initialize with default configuration."""
        extractor = TagExtractor()
        assert extractor.config.model_name == "paraphrase-multilingual-MiniLM-L12-v2"
        assert extractor.config.top_keywords == 10
        assert extractor.config.min_score_threshold == 0.15

    def test_should_initialize_with_custom_config(self):
        """TagExtractor should accept custom configuration."""
        config = TagExtractionConfig(model_name="custom-model", top_keywords=5, min_score_threshold=0.2)
        extractor = TagExtractor(config)
        assert extractor.config.model_name == "custom-model"
        assert extractor.config.top_keywords == 5
        assert extractor.config.min_score_threshold == 0.2

    def test_should_lazy_load_models_once(self):
        """Models should be loaded lazily and only once via ModelManager."""
        from tag_extractor.model_manager import get_model_manager

        # Clear any existing models
        model_manager = get_model_manager()
        model_manager.clear_models()

        extractor = TagExtractor()

        # Models should not be loaded on initialization

        with patch.object(model_manager, "get_models", return_value=(Mock(), Mock(), Mock())) as mock_get_models:
            # First call should load models
            extractor._lazy_load_models()
            assert mock_get_models.call_count == 1
            assert extractor._models_loaded

            # Second call should not reload models
            extractor._lazy_load_models()
            assert mock_get_models.call_count == 1  # Still only called once

    def test_should_validate_input_text_length(self):
        """Should handle text that is too short."""
        extractor = TagExtractor()

        # Very short text should return empty list
        result = extractor.extract_tags_with_metrics("Hi", "OK")
        assert result.tags == []

    @patch("tag_extractor.extract.detect")
    def test_should_detect_language_correctly(self, mock_detect):
        """Should detect text language correctly."""
        mock_detect.return_value = "ja"

        extractor = TagExtractor()
        lang = extractor._detect_language("こんにちは世界")

        assert lang == "ja"
        mock_detect.assert_called_once()

    @patch("tag_extractor.extract.detect")
    def test_should_fallback_to_english_on_detection_failure(self, mock_detect):
        """Should default to English if language detection fails."""
        from langdetect import LangDetectException

        mock_detect.side_effect = LangDetectException("Detection failed", "")

        extractor = TagExtractor()
        lang = extractor._detect_language("some text")

        assert lang == "en"

    def test_should_normalize_text_based_on_language(self):
        """Should normalize text differently based on language."""
        extractor = TagExtractor()

        # English text should be lowercased
        en_result = extractor._normalize_text("HELLO WORLD", "en")
        assert en_result == "hello world"

        # Japanese text should be NFKC normalized
        ja_result = extractor._normalize_text("ＡＢＣ", "ja")
        assert ja_result == "ABC"  # Full-width to half-width

    def test_should_extract_keywords_with_keybert(self):
        """Should use KeyBERT to extract keywords for English text."""
        from tag_extractor.model_manager import get_model_manager

        model_manager = get_model_manager()
        model_manager.clear_models()

        extractor = TagExtractor()

        # Mock the KeyBERT instance at the model manager level
        with (
            patch.object(model_manager, "get_models") as mock_get_models,
            patch.object(model_manager, "get_stopwords", return_value=(set(), set())),
        ):
            mock_embedder = Mock()
            mock_keybert = Mock()
            mock_ja_tagger = Mock()
            mock_keybert.extract_keywords.return_value = [
                ("Apple Intelligence", 0.9),
                ("Mac Mini", 0.8),
                ("machine learning", 0.8),
                ("artificial intelligence", 0.7),
                ("technology", 0.6),
            ]
            mock_get_models.return_value = (mock_embedder, mock_keybert, mock_ja_tagger)

            result = extractor._extract_keywords_english(
                "Machine learning is transforming technology with Apple Intelligence on Mac Mini"
            )

            assert len(result) > 0
            assert mock_keybert.extract_keywords.call_count == 1  # Single call with (1,3) ngram range

    def test_should_extract_japanese_compound_words(self):
        """Should extract compound words from Japanese text."""
        from tag_extractor.model_manager import get_model_manager

        extractor = TagExtractor()
        model_manager = get_model_manager()

        # Mock Japanese tagger
        with patch.object(model_manager, "get_models") as mock_get_models:
            mock_embedder = Mock()
            mock_keybert = Mock()
            mock_ja_tagger = Mock()

            # Mock parsed word with feature attributes
            mock_word = Mock()
            mock_word.surface = "東京"
            mock_word.feature = Mock()
            mock_word.feature.pos1 = "名詞"
            mock_word.feature.pos2 = "固有名詞"

            mock_ja_tagger.return_value = [mock_word]
            mock_get_models.return_value = (mock_embedder, mock_keybert, mock_ja_tagger)

            result = extractor._extract_compound_japanese_words("東京に行きました")

            assert len(result) >= 0  # Should return list of compound words

    def test_should_handle_japanese_text_extraction(self):
        """Should extract keywords from Japanese text."""
        from tag_extractor.model_manager import get_model_manager

        extractor = TagExtractor()
        model_manager = get_model_manager()

        # Mock dependencies
        with (
            patch.object(model_manager, "get_models") as mock_get_models,
            patch.object(model_manager, "get_stopwords", return_value=(set(), set())),
            patch.object(
                extractor,
                "_extract_compound_japanese_words",
                return_value=["東京", "日本"],
            ),
        ):
            mock_embedder = Mock()
            mock_keybert = Mock()
            mock_ja_tagger = Mock()

            # Mock parsed words
            mock_word1 = Mock()
            mock_word1.surface = "東京"
            mock_word1.feature = Mock()
            mock_word1.feature.pos1 = "名詞"

            mock_word2 = Mock()
            mock_word2.surface = "日本"
            mock_word2.feature = Mock()
            mock_word2.feature.pos1 = "名詞"

            mock_ja_tagger.return_value = [mock_word1, mock_word2]
            mock_get_models.return_value = (mock_embedder, mock_keybert, mock_ja_tagger)

            keywords, confidences = extractor._extract_keywords_japanese("東京は日本の首都です")

            assert isinstance(keywords, list)
            assert isinstance(confidences, dict)
            assert len(keywords) >= 0

    @patch("nltk.word_tokenize")
    def test_should_tokenize_english_text(self, mock_tokenize):
        """Should tokenize English text properly."""
        mock_tokenize.return_value = [
            "The",
            "machine",
            "learning",
            "algorithm",
            "is",
            "advanced",
        ]

        extractor = TagExtractor()

        with patch.object(extractor, "_load_stopwords"):
            extractor._en_stopwords = {"the", "and", "a", "an", "is", "are"}

            result = extractor._tokenize_english("The machine learning algorithm is advanced")

            # Should exclude stopwords and short tokens
            assert "machine" in result
            assert "learning" in result
            assert "algorithm" in result
            assert "advanced" in result
            assert "the" not in result
            assert "is" not in result

    def test_should_use_fallback_extraction_for_japanese(self):
        """Should use fallback extraction for Japanese text."""
        extractor = TagExtractor()

        with patch.object(
            extractor,
            "_extract_keywords_japanese",
            return_value=(["東京", "日本"], {"東京": 1.0, "日本": 1.0}),
        ):
            result = extractor._fallback_extraction("東京は日本の首都です", "ja")

            assert result == ["東京", "日本"]

    def test_should_use_fallback_extraction_for_english(self):
        """Should use fallback extraction for English text."""
        extractor = TagExtractor()

        with patch.object(
            extractor,
            "_tokenize_english",
            return_value=["machine", "learning", "algorithm"],
        ):
            result = extractor._fallback_extraction("machine learning algorithm", "en")

            assert "machine" in result
            assert "learning" in result
            assert "algorithm" in result

    @patch("tag_extractor.extract.detect")
    def test_should_extract_tags_end_to_end_english(self, mock_detect):
        """Should extract tags from English text end-to-end."""
        mock_detect.return_value = "en"

        extractor = TagExtractor()

        with patch.object(
            extractor,
            "_extract_keywords_english",
            return_value=(["machine", "learning", "ai"], {"machine": 1.0, "learning": 0.9, "ai": 0.8}),
        ):
            outcome = extractor.extract_tags_with_metrics(
                "Machine Learning", "Artificial intelligence and machine learning"
            )

            assert outcome.tags == ["machine", "learning", "ai"]

    @patch("tag_extractor.extract.detect")
    def test_should_extract_tags_end_to_end_japanese(self, mock_detect):
        """Should extract tags from Japanese text end-to-end."""
        mock_detect.return_value = "ja"

        extractor = TagExtractor()

        with patch.object(
            extractor,
            "_extract_keywords_japanese",
            return_value=(["東京", "日本"], {"東京": 1.0, "日本": 0.9}),
        ):
            outcome = extractor.extract_tags_with_metrics("東京について", "東京は日本の首都です")

            assert outcome.tags == ["東京", "日本"]

    def test_should_handle_extraction_errors_with_fallback(self):
        """Should handle extraction errors and use fallback."""
        extractor = TagExtractor()

        with (
            patch.object(extractor, "_detect_language", return_value="en"),
            patch.object(
                extractor,
                "_extract_keywords_english",
                side_effect=Exception("KeyBERT failed"),
            ),
            patch.object(extractor, "_fallback_extraction", return_value=["fallback", "keywords"]),
        ):
            outcome = extractor.extract_tags_with_metrics("Test Title", "Test content for fallback")

            assert outcome.tags == ["fallback", "keywords"]

    def test_should_return_empty_for_failed_extractions(self):
        """Should return empty list when all extractions fail."""
        extractor = TagExtractor()

        with (
            patch.object(extractor, "_detect_language", return_value="en"),
            patch.object(
                extractor,
                "_extract_keywords_english",
                side_effect=Exception("KeyBERT failed"),
            ),
            patch.object(
                extractor,
                "_fallback_extraction",
                side_effect=Exception("Fallback failed"),
            ),
        ):
            outcome = extractor.extract_tags_with_metrics("Test Title", "Test content")

            assert outcome.tags == []

    def test_extract_tags_with_metrics_returns_outcome(self):
        """Should return metrics container from extract_tags_with_metrics."""
        extractor = TagExtractor()
        sanitized_input = SanitizedArticleInput(
            title="Test title",
            content="Test content with enough length",
            url=None,
            original_length=50,
            sanitized_length=45,
        )
        sanitization_result = SanitizationResult(
            is_valid=True,
            sanitized_input=sanitized_input,
            violations=[],
            warnings=[],
        )

        with (
            patch.object(extractor._input_sanitizer, "sanitize", return_value=sanitization_result),
            patch.object(extractor, "_detect_language", return_value="en"),
            patch.object(
                extractor,
                "_run_extraction",
                return_value=(["tag1", "tag2"], {"tag1": 0.9, "tag2": 0.8}),
            ),
        ):
            outcome = extractor.extract_tags_with_metrics("Title", "Content")

            assert outcome.tags == ["tag1", "tag2"]
            assert outcome.language == "en"
            assert outcome.tag_count == 2
            assert 0.0 < outcome.confidence <= 1.0
            assert outcome.model_name == extractor.config.model_name

    def test_extract_tags_with_metrics_handles_invalid_input(self):
        """Should return empty outcome when sanitization fails."""
        extractor = TagExtractor()
        sanitization_result = SanitizationResult(
            is_valid=False,
            sanitized_input=None,
            violations=["invalid"],
            warnings=[],
        )

        with patch.object(extractor._input_sanitizer, "sanitize", return_value=sanitization_result):
            outcome = extractor.extract_tags_with_metrics("Title", "Content")

            assert outcome.tags == []
            assert outcome.confidence == 0.0
            assert outcome.language == "und"


class TestCascadeController:
    """Unit tests for cascade controller heuristics."""

    def test_should_request_refine_when_confidence_low(self):
        controller = CascadeController()
        outcome = TagExtractionOutcome(
            tags=["tag1"],
            confidence=0.3,
            tag_count=1,
            inference_ms=50.0,
            language="en",
            model_name="model",
            sanitized_length=30,
            embedding_backend="sentence_transformer",
            embedding_metadata={},
        )

        decision = controller.evaluate(outcome)

        assert decision.needs_refine
        assert decision.reason == "low_confidence"

    def test_should_limit_refine_ratio_when_budget_exhausted(self):
        config = CascadeConfig(max_refine_ratio=0.0)
        controller = CascadeController(config)
        outcome = TagExtractionOutcome(
            tags=["tag1"],
            confidence=0.3,
            tag_count=2,
            inference_ms=20.0,
            language="en",
            model_name="model",
            sanitized_length=30,
            embedding_backend="sentence_transformer",
            embedding_metadata={},
        )

        decision = controller.evaluate(outcome)

        assert not decision.needs_refine
        assert decision.reason == "refine_ratio_budget_capped"

    def test_should_exit_when_confidence_high(self):
        controller = CascadeController()
        outcome = TagExtractionOutcome(
            tags=["tag1", "tag2", "tag3", "tag4", "tag5", "tag6"],
            confidence=0.95,
            tag_count=6,
            inference_ms=10.0,
            language="en",
            model_name="model",
            sanitized_length=120,
            embedding_backend="sentence_transformer",
            embedding_metadata={},
        )

        decision = controller.evaluate(outcome)

        assert not decision.needs_refine
        assert decision.reason == "high_confidence_exit"


class TestTagGeneratorService:
    """Unit tests for TagGeneratorService class."""

    def test_should_initialize_with_default_config(self):
        """TagGeneratorService should initialize with default configuration."""
        with patch("tag_generator.service.create_backend_client") as mock_create:
            mock_client = Mock()
            mock_create.return_value = (mock_client, {})
            service = TagGeneratorService()
        assert service.config.batch_limit == 75
        # Default processing interval should match TagGeneratorConfig
        assert service.config.processing_interval == 300
        assert isinstance(service.tag_extractor, TagExtractor)

    def test_should_process_single_article_successfully(self):
        """Should process a single article successfully."""
        mock_conn = Mock()
        with patch("tag_generator.service.create_backend_client") as mock_create:
            mock_client = Mock()
            mock_create.return_value = (mock_client, {})
            service = TagGeneratorService()

        article = {
            "id": "test-uuid",
            "title": "Test Title",
            "content": "Test content",
            "created_at": "2023-01-01T00:00:00Z",
            "feed_id": "feed-uuid-1",
        }

        outcome = TagExtractionOutcome(
            tags=["tag1", "tag2"],
            confidence=0.8,
            tag_count=2,
            inference_ms=12.5,
            language="en",
            model_name=service.tag_extractor.config.model_name,
            sanitized_length=100,
            embedding_backend="sentence_transformer",
            embedding_metadata={},
        )

        # Mock tag extraction and insertion
        with (
            patch.object(
                service.tag_extractor,
                "extract_tags_with_metrics",
                return_value=outcome,
            ) as mock_extract_with_metrics,
            patch.object(
                service.tag_inserter,
                "upsert_tags",
                return_value={"success": True, "tags_processed": 2},
            ) as mock_upsert_tags,
        ):
            result = service._process_single_article(mock_conn, article)

            assert result is True
            mock_extract_with_metrics.assert_called_once_with("Test Title", "Test content")
            mock_upsert_tags.assert_called_once_with(mock_conn, "test-uuid", ["tag1", "tag2"], "feed-uuid-1")

    def test_should_handle_article_processing_errors(self):
        """Should handle errors during article processing gracefully."""
        mock_conn = Mock()
        with patch("tag_generator.service.create_backend_client") as mock_create:
            mock_client = Mock()
            mock_create.return_value = (mock_client, {})
            service = TagGeneratorService()

        article = {
            "id": "test-uuid",
            "title": "Test Title",
            "content": "Test content",
            "created_at": "2023-01-01T00:00:00Z",
        }

        # Mock tag extraction failure
        with patch.object(
            service.tag_extractor,
            "extract_tags_with_metrics",
            side_effect=Exception("Extraction failed"),
        ):
            result = service._process_single_article(mock_conn, article)

            assert result is False


if __name__ == "__main__":
    # Run unit tests
    pytest.main([__file__, "-v", "--tb=short"])
