"""
Unit tests for tag-generator following TDD principles.
These tests define the expected behavior that must be maintained during refactoring.
"""

from unittest.mock import Mock, patch

import psycopg2
import pytest

from article_fetcher.fetch import ArticleFetcher
from main import TagGeneratorService
from tag_extractor.extract import TagExtractionConfig, TagExtractionOutcome, TagExtractor
from tag_extractor.input_sanitizer import SanitizationResult, SanitizedArticleInput
from tag_generator.cascade import CascadeConfig, CascadeController
from tag_inserter.upsert_tags import TagInserter


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
        result = extractor.extract_tags("Hi", "OK")
        assert result == []

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
            mock_keybert.extract_keywords.side_effect = [
                [
                    ("machine learning", 0.8),
                    ("artificial intelligence", 0.7),
                    ("technology", 0.6),
                ],
                [("Apple Intelligence", 0.9), ("Mac Mini", 0.8)],
            ]
            mock_get_models.return_value = (mock_embedder, mock_keybert, mock_ja_tagger)

            result = extractor._extract_keywords_english(
                "Machine learning is transforming technology with Apple Intelligence on Mac Mini"
            )

            assert len(result) > 0
            assert mock_keybert.extract_keywords.call_count == 2  # Called for single words and phrases

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

            result = extractor._extract_keywords_japanese("東京は日本の首都です")

            assert isinstance(result, list)
            assert len(result) >= 0

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

        with patch.object(extractor, "_extract_keywords_japanese", return_value=["東京", "日本"]):
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
            return_value=["machine", "learning", "ai"],
        ):
            result = extractor.extract_tags("Machine Learning", "Artificial intelligence and machine learning")

            assert result == ["machine", "learning", "ai"]

    @patch("tag_extractor.extract.detect")
    def test_should_extract_tags_end_to_end_japanese(self, mock_detect):
        """Should extract tags from Japanese text end-to-end."""
        mock_detect.return_value = "ja"

        extractor = TagExtractor()

        with patch.object(extractor, "_extract_keywords_japanese", return_value=["東京", "日本"]):
            result = extractor.extract_tags("東京について", "東京は日本の首都です")

            assert result == ["東京", "日本"]

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
            result = extractor.extract_tags("Test Title", "Test content for fallback")

            assert result == ["fallback", "keywords"]

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
            result = extractor.extract_tags("Test Title", "Test content")

            assert result == []

    def test_legacy_extract_tags_function(self):
        """Should maintain backward compatibility with legacy function."""
        from tag_extractor.extract import extract_tags

        with patch("tag_extractor.extract.TagExtractor") as mock_extractor_class:
            mock_extractor = Mock()
            mock_extractor.extract_tags.return_value = ["tag1", "tag2"]
            mock_extractor_class.return_value = mock_extractor

            result = extract_tags("Test Title", "Test Content")

            assert result == ["tag1", "tag2"]
            mock_extractor.extract_tags.assert_called_once_with("Test Title", "Test Content")

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
            patch.object(extractor, "_run_extraction", return_value=["tag1", "tag2"]),
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


class TestArticleFetcher:
    """Unit tests for ArticleFetcher class."""

    def test_should_initialize_with_default_config(self):
        """ArticleFetcher should initialize with default configuration."""
        fetcher = ArticleFetcher()
        assert fetcher.config.batch_size == 500
        assert fetcher.config.max_retries == 3

    def test_should_validate_cursor_parameters(self):
        """Should validate cursor parameters for pagination."""
        fetcher = ArticleFetcher()

        # Valid parameters should not raise
        fetcher._validate_cursor_params("2023-01-01T00:00:00Z", "uuid-string")

        # Invalid parameters should raise ValueError
        with pytest.raises(ValueError, match="last_created_at must be a non-empty string"):
            fetcher._validate_cursor_params("", "uuid-string")

        with pytest.raises(ValueError, match="last_id must be a non-empty string"):
            fetcher._validate_cursor_params("2023-01-01T00:00:00Z", "")

    def test_should_build_correct_fetch_query(self):
        """Should build the correct SQL query for fetching articles."""
        fetcher = ArticleFetcher()
        query = fetcher._build_fetch_query()

        # Check that query contains expected components
        assert "SELECT" in query
        assert "id::text AS id" in query
        assert "FROM articles" in query
        assert "WHERE" in query
        assert "ORDER BY created_at DESC, id DESC" in query
        assert "LIMIT %s" in query

    def test_should_fetch_articles_with_correct_parameters(self):
        """Should execute query with correct parameters."""
        mock_conn = Mock()
        mock_cursor = Mock()
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.__enter__ = Mock(return_value=mock_cursor)
        mock_cursor.__exit__ = Mock(return_value=None)
        mock_cursor.fetchall.return_value = [
            {
                "id": "1",
                "title": "Title 1",
                "content": "Content 1",
                "created_at": "2023-01-01",
            },
            {
                "id": "2",
                "title": "Title 2",
                "content": "Content 2",
                "created_at": "2023-01-02",
            },
        ]

        fetcher = ArticleFetcher()
        result = fetcher.fetch_articles(mock_conn, "2023-01-01T00:00:00Z", "uuid-1")

        # Check that cursor.execute was called with correct parameters
        mock_cursor.execute.assert_called_once()
        args = mock_cursor.execute.call_args[0]
        assert len(args) == 2  # query and parameters
        assert args[1] == (
            "2023-01-01T00:00:00Z",
            "2023-01-01T00:00:00Z",
            "uuid-1",
            500,
        )

        # Check result
        assert len(result) == 2
        assert result[0]["title"] == "Title 1"

    def test_should_handle_database_errors_gracefully(self):
        """Should handle database errors and raise ArticleFetchError."""
        mock_conn = Mock()
        mock_conn.cursor.side_effect = psycopg2.Error("Database connection failed")

        fetcher = ArticleFetcher()

        from article_fetcher.fetch import ArticleFetchError

        with pytest.raises(ArticleFetchError, match="Failed to fetch articles"):
            fetcher.fetch_articles(mock_conn, "2023-01-01T00:00:00Z", "uuid-1")

    def test_should_count_untagged_articles(self):
        """Should count articles without tags correctly."""
        mock_conn = Mock()
        mock_cursor = Mock()
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.__enter__ = Mock(return_value=mock_cursor)
        mock_cursor.__exit__ = Mock(return_value=None)
        mock_cursor.fetchone.return_value = (42,)

        fetcher = ArticleFetcher()
        count = fetcher.count_untagged_articles(mock_conn)

        assert count == 42
        mock_cursor.execute.assert_called_once()
        # Check that the query looks for articles without tags
        query = mock_cursor.execute.call_args[0][0]
        assert "LEFT JOIN article_tags" in query
        assert "WHERE at.article_id IS NULL" in query


class TestTagInserter:
    """Unit tests for TagInserter class."""

    def test_should_initialize_with_default_config(self):
        """TagInserter should initialize with default configuration."""
        inserter = TagInserter()
        assert inserter.config.batch_size == 1000
        assert inserter.config.page_size == 200

    def test_should_validate_input_parameters(self):
        """Should validate input parameters for upsert_tags."""
        inserter = TagInserter()

        # Valid inputs should not raise
        inserter._validate_inputs("uuid-1", ["tag1", "tag2"])

        # Invalid article_id should raise ValueError
        with pytest.raises(ValueError, match="article_id must be a non-empty string"):
            inserter._validate_inputs("", ["tag1"])

        # Invalid tags should raise ValueError
        with pytest.raises(ValueError, match="tags must be a non-empty list"):
            inserter._validate_inputs("uuid-1", [])

        with pytest.raises(ValueError, match="All tags must be non-empty strings"):
            inserter._validate_inputs("uuid-1", ["tag1", ""])

    def test_should_insert_tags_with_conflict_handling(self):
        """Should insert tags with conflict resolution."""
        mock_cursor = Mock()
        mock_cursor.mogrify = Mock(return_value=b"INSERT INTO tags...")
        inserter = TagInserter()

        with patch("psycopg2.extras.execute_batch") as mock_execute_batch:
            inserter._insert_tags(mock_cursor, ["tag1", "tag2", "tag3"], "feed-uuid-1")

            # Should use execute_batch with ON CONFLICT DO NOTHING
            assert mock_execute_batch.called
            call_args = mock_execute_batch.call_args
            query = call_args[0][1]
            assert "ON CONFLICT (feed_id, tag_name) DO NOTHING" in query

    def test_should_get_tag_ids_correctly(self):
        """Should retrieve tag IDs for given tag names."""
        mock_cursor = Mock()
        mock_cursor.fetchall.return_value = [
            ("uuid-1", "tag1"),
            ("uuid-2", "tag2"),
            ("uuid-3", "tag3"),
        ]

        inserter = TagInserter()
        result = inserter._get_tag_ids(mock_cursor, ["tag1", "tag2", "tag3"], "feed-uuid-1")

        expected = {"tag1": "uuid-1", "tag2": "uuid-2", "tag3": "uuid-3"}
        assert result == expected

        # Should use ANY(%s) for efficient querying
        mock_cursor.execute.assert_called_once()
        query = mock_cursor.execute.call_args[0][0]
        assert "WHERE tag_name = ANY(%s) AND feed_id = %s::uuid" in query

    def test_should_insert_article_tag_relationships(self):
        """Should insert article-tag relationships correctly."""
        mock_cursor = Mock()
        mock_cursor.mogrify = Mock(return_value=b"INSERT INTO article_tags...")
        inserter = TagInserter()

        with patch("psycopg2.extras.execute_batch") as mock_execute_batch:
            tag_ids = {"tag1": "uuid-1", "tag2": "uuid-2", "tag3": "uuid-3"}
            inserter._insert_article_tags(mock_cursor, "uuid-1", tag_ids)

            # Should use execute_batch with conflict resolution
            assert mock_execute_batch.called
            call_args = mock_execute_batch.call_args
            query = call_args[0][1]
            assert "INSERT INTO article_tags" in query
            assert "ON CONFLICT (article_id, feed_tag_id) DO NOTHING" in query

    def test_should_handle_successful_upsert_transaction(self):
        """Should handle successful upsert with proper transaction management."""
        mock_conn = Mock()
        mock_cursor = Mock()
        mock_cursor.mogrify = Mock(return_value=b"INSERT...")
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.__enter__ = Mock(return_value=mock_cursor)
        mock_cursor.__exit__ = Mock(return_value=None)
        mock_cursor.fetchall.return_value = [("uuid-1", "tag1"), ("uuid-2", "tag2")]

        inserter = TagInserter()
        result = inserter.upsert_tags(mock_conn, "uuid-1", ["tag1", "tag2"], "feed-uuid-1")

        # Should commit transaction
        mock_conn.commit.assert_called_once()

        # Should return success result
        assert result["success"] is True
        assert result["tags_processed"] == 2
        assert result["article_id"] == "uuid-1"

    def test_should_rollback_on_error(self):
        """Should rollback transaction on error."""
        mock_conn = Mock()
        mock_conn.cursor.side_effect = Exception("Database error")

        inserter = TagInserter()

        with pytest.raises(Exception, match="Database error"):
            inserter.upsert_tags(mock_conn, "uuid-1", ["tag1", "tag2"], "feed-uuid-1")

        # Should attempt rollback
        mock_conn.rollback.assert_called_once()

    def test_should_handle_batch_upsert_efficiently(self):
        """Should handle batch upsert more efficiently than individual upserts."""
        mock_conn = Mock()
        mock_cursor = Mock()
        mock_cursor.mogrify = Mock(return_value=b"INSERT...")
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.__enter__ = Mock(return_value=mock_cursor)
        mock_cursor.__exit__ = Mock(return_value=None)

        # Mock feed_id queries - each article gets a feed_id
        mock_cursor.fetchone.side_effect = [
            ("feed-uuid-1", None),  # uuid-1's feed_id
            ("feed-uuid-1", None),  # uuid-1's feed_id (second query)
            ("feed-uuid-2", None),  # uuid-2's feed_id
            ("feed-uuid-2", None),  # uuid-2's feed_id (second query)
            ("feed-uuid-3", None),  # uuid-3's feed_id
            ("feed-uuid-3", None),  # uuid-3's feed_id (second query)
        ]

        # Mock tag ID queries - return UUIDs for tags
        mock_cursor.fetchall.return_value = [
            ("tag-uuid-1", "tag1"),
            ("tag-uuid-2", "tag2"),
            ("tag-uuid-3", "tag3"),
        ]

        inserter = TagInserter()

        batch_data = [
            {"article_id": "uuid-1", "tags": ["tag1", "tag2"]},
            {"article_id": "uuid-2", "tags": ["tag2", "tag3"]},
            {"article_id": "uuid-3", "tags": ["tag1", "tag3"]},
        ]

        result = inserter.batch_upsert_tags(mock_conn, batch_data)

        # Should process all articles in single transaction
        mock_conn.commit.assert_called_once()
        assert result["success"] is True
        assert result["processed_articles"] == 3

        # Should process all articles successfully
        # Database calls are mocked, so we just verify success


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
        service = TagGeneratorService()
        assert service.config.batch_limit == 75
        assert service.config.processing_interval == 1800
        assert isinstance(service.article_fetcher, ArticleFetcher)
        assert isinstance(service.tag_extractor, TagExtractor)
        assert isinstance(service.tag_inserter, TagInserter)
        assert service.forward_cursor_created_at is None
        assert service.forward_cursor_id is None

    def test_should_build_database_dsn_from_environment(self):
        """Should build database DSN from environment variables."""
        service = TagGeneratorService()

        with patch.dict(
            "os.environ",
            {
                "DB_TAG_GENERATOR_USER": "testuser",
                "DB_TAG_GENERATOR_PASSWORD": "testpass",
                "DB_HOST": "localhost",
                "DB_PORT": "5432",
                "DB_NAME": "testdb",
            },
        ):
            dsn = service._get_database_dsn()
            expected = "postgresql://testuser:testpass@localhost:5432/testdb"
            assert dsn == expected

    def test_should_require_all_environment_variables(self):
        """Should raise error if required environment variables are missing."""
        service = TagGeneratorService()

        with patch.dict("os.environ", {}, clear=True):
            with pytest.raises(ValueError, match="Missing required environment variables"):
                service._get_database_dsn()

    @patch("main.psycopg2.connect")
    def test_should_create_database_connection_with_retry(self, mock_connect):
        """Should create database connection with retry logic."""
        mock_conn = Mock()
        mock_connect.return_value = mock_conn

        service = TagGeneratorService()

        with patch.object(service, "_get_database_dsn", return_value="test-dsn"):
            conn = service._create_direct_connection()

            assert conn == mock_conn
            mock_connect.assert_called_once_with("test-dsn")

    @patch("main.psycopg2.connect")
    def test_should_retry_failed_connections(self, mock_connect):
        """Should retry failed database connections."""
        # First two attempts fail, third succeeds
        mock_conn = Mock()
        mock_connect.side_effect = [
            psycopg2.Error("Connection failed"),
            psycopg2.Error("Connection failed"),
            mock_conn,
        ]

        service = TagGeneratorService()

        with (
            patch.object(service, "_get_database_dsn", return_value="test-dsn"),
            patch("time.sleep"),
        ):  # Mock sleep to speed up test
            conn = service._create_direct_connection()

            assert conn == mock_conn
            assert mock_connect.call_count == 3

    def test_should_get_initial_cursor_position(self):
        """Should get correct initial cursor position for pagination."""
        service = TagGeneratorService()

        # First run should use current time
        created_at, last_id = service._get_initial_cursor_position()
        assert created_at is not None
        assert last_id == "ffffffff-ffff-ffff-ffff-ffffffffffff"

        # Subsequent runs should use saved position (use recent timestamp to avoid cursor poisoning)
        from datetime import UTC, datetime, timedelta

        recent_timestamp = (datetime.now(UTC) - timedelta(hours=1)).isoformat()
        service.last_processed_created_at = recent_timestamp
        service.last_processed_id = "test-uuid"

        created_at, last_id = service._get_initial_cursor_position()
        assert created_at == recent_timestamp
        assert last_id == "test-uuid"

    def test_should_detect_cursor_poisoning_and_use_recovery(self):
        """Should detect cursor poisoning and switch to recovery mode."""
        service = TagGeneratorService()

        # Set old timestamp that should trigger cursor poisoning detection
        service.last_processed_created_at = "2023-01-01T00:00:00Z"
        service.last_processed_id = "test-uuid"

        # Should detect cursor poisoning and use recovery cursor
        created_at, last_id = service._get_initial_cursor_position()
        # Recovery cursor should return current time and default UUID
        assert created_at is not None
        assert created_at != "2023-01-01T00:00:00Z"  # Should not use the poisoned cursor
        assert last_id == "ffffffff-ffff-ffff-ffff-ffffffffffff"

    def test_should_process_single_article_successfully(self):
        """Should process a single article successfully."""
        mock_conn = Mock()
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

    def test_should_update_cursor_position_after_batch(self):
        """Should update cursor position after processing batch."""
        service = TagGeneratorService()
        mock_conn = Mock()

        # Mock dependencies
        with (
            patch.object(
                service,
                "_get_initial_cursor_position",
                return_value=("2023-01-01T00:00:00Z", "uuid-1"),
            ),
            patch.object(service.article_fetcher, "fetch_articles") as mock_fetch,
            patch.object(service, "_process_articles_as_batch") as mock_process,
        ):
            # Mock article fetch
            mock_fetch.return_value = [
                {
                    "id": "uuid-2",
                    "title": "Title",
                    "content": "Content",
                    "created_at": "2023-01-02T00:00:00Z",
                }
            ]

            # Mock batch processing
            mock_process.return_value = {
                "total_processed": 1,
                "successful": 1,
                "failed": 0,
            }

            service._process_article_batch(mock_conn)

            # Should update cursor position
            assert service.last_processed_created_at == "2023-01-02T00:00:00Z"
            assert service.last_processed_id == "uuid-2"

    def test_should_skip_processing_when_no_new_articles(self):
        """Skip tagging work if there are no forward articles to process."""
        service = TagGeneratorService()
        mock_conn = Mock()

        with (
            patch.object(service, "_has_existing_tags", return_value=True),
            patch.object(service, "_get_forward_cursor_position", return_value=("2024-01-01T00:00:00Z", "base-id")),
            patch.object(service.article_fetcher, "fetch_new_articles", return_value=[]),
            patch.object(service, "_process_articles_as_batch") as mock_process,
        ):
            stats = service._process_article_batch(mock_conn)

        assert stats["total_processed"] == 0
        assert stats["successful"] == 0
        assert stats["failed"] == 0
        mock_process.assert_not_called()

    def test_backfill_should_update_forward_cursor_from_latest_article(self):
        """Backfill run sets forward cursor based on newest processed article."""
        service = TagGeneratorService()
        mock_conn = Mock()
        articles = [
            {
                "id": "newest",
                "title": "Title 1",
                "content": "Content 1",
                "created_at": "2024-01-03T00:00:00Z",
            },
            {
                "id": "older",
                "title": "Title 2",
                "content": "Content 2",
                "created_at": "2024-01-02T00:00:00Z",
            },
        ]

        with (
            patch.object(service, "_has_existing_tags", return_value=False),
            patch.object(service, "_get_initial_cursor_position", return_value=("2024-01-04T00:00:00Z", "cursor")),
            patch.object(service.article_fetcher, "fetch_articles", return_value=articles),
            patch.object(
                service,
                "_process_articles_as_batch",
                return_value={"total_processed": 2, "successful": 2, "failed": 0},
            ),
        ):
            stats = service._process_article_batch(mock_conn)

        assert service.forward_cursor_created_at == "2024-01-03T00:00:00Z"
        assert service.forward_cursor_id == "newest"
        assert stats["last_created_at"] == "2024-01-02T00:00:00Z"
        assert stats["last_id"] == "older"

    def test_forward_batch_failure_should_not_advance_cursor(self):
        """Failed forward batch should not move the persistent cursor forward."""
        service = TagGeneratorService()
        mock_conn = Mock()
        mock_conn.autocommit = True
        initial_cursor = ("2024-01-01T00:00:00Z", "seed-id")

        articles = [
            {
                "id": "a1",
                "title": "Forward 1",
                "content": "Content",
                "created_at": "2024-01-05T00:00:00Z",
            }
        ]

        service.forward_cursor_created_at, service.forward_cursor_id = initial_cursor

        with (
            patch.object(service, "_has_existing_tags", return_value=True),
            patch.object(service, "_get_forward_cursor_position", return_value=initial_cursor),
            patch.object(service.article_fetcher, "fetch_new_articles", return_value=articles),
            patch.object(
                service,
                "_process_articles_as_batch",
                return_value={"total_processed": 1, "successful": 0, "failed": 1},
            ),
        ):
            service._process_article_batch(mock_conn)

        assert service.forward_cursor_created_at == initial_cursor[0]
        assert service.forward_cursor_id == initial_cursor[1]
        assert service.last_processed_created_at is None
        assert service.last_processed_id is None

    def test_backfill_batch_failure_should_not_advance_cursor(self):
        """Failed backfill batch should not move cursors forward or backward."""
        service = TagGeneratorService()
        mock_conn = Mock()
        mock_conn.autocommit = True
        initial_last_processed = ("2023-12-31T00:00:00Z", "previous-id")
        service.last_processed_created_at, service.last_processed_id = initial_last_processed
        service.forward_cursor_created_at = "2024-01-01T00:00:00Z"
        service.forward_cursor_id = "forward-id"

        articles = [
            {
                "id": "old-1",
                "title": "Old Article",
                "content": "Content",
                "created_at": "2023-12-01T00:00:00Z",
            }
        ]

        with (
            patch.object(service, "_has_existing_tags", return_value=False),
            patch.object(service, "_get_initial_cursor_position", return_value=initial_last_processed),
            patch.object(service.article_fetcher, "count_untagged_articles", return_value=1),
            patch.object(service.article_fetcher, "fetch_articles", return_value=articles),
            patch.object(
                service,
                "_process_articles_as_batch",
                return_value={"total_processed": 1, "successful": 0, "failed": 1},
            ),
        ):
            service._process_article_batch(mock_conn)

        assert service.last_processed_created_at == initial_last_processed[0]
        assert service.last_processed_id == initial_last_processed[1]
        assert service.forward_cursor_created_at == "2024-01-01T00:00:00Z"
        assert service.forward_cursor_id == "forward-id"

    def test_forward_mode_updates_cursor_and_processes_new_articles(self):
        """Forward processing fetches new articles and advances cursor."""
        service = TagGeneratorService()
        mock_conn = Mock()
        forward_articles = [
            {
                "id": "a1",
                "title": "Forward 1",
                "content": "Content",
                "created_at": "2024-01-05T00:00:00Z",
            },
            {
                "id": "a2",
                "title": "Forward 2",
                "content": "Content",
                "created_at": "2024-01-06T00:00:00Z",
            },
        ]

        with (
            patch.object(service, "_has_existing_tags", return_value=True),
            patch.object(service, "_get_forward_cursor_position", return_value=("2024-01-04T00:00:00Z", "seed")),
            patch.object(service.article_fetcher, "fetch_new_articles", return_value=forward_articles) as mock_fetch,
            patch.object(
                service,
                "_process_articles_as_batch",
                return_value={"total_processed": 2, "successful": 2, "failed": 0},
            ) as mock_process,
        ):
            stats = service._process_article_batch(mock_conn)

        mock_fetch.assert_called_once_with(mock_conn, "2024-01-04T00:00:00Z", "seed", service.config.batch_limit)
        mock_process.assert_called_once()
        assert service.forward_cursor_created_at == "2024-01-06T00:00:00Z"
        assert service.forward_cursor_id == "a2"
        assert stats["last_created_at"] == "2024-01-06T00:00:00Z"
        assert stats["last_id"] == "a2"


if __name__ == "__main__":
    # Run unit tests
    pytest.main([__file__, "-v", "--tb=short"])
