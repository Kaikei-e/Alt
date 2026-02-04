"""
Unit tests for GiNZA extractor.
"""

from unittest.mock import MagicMock, patch

import pytest

from tag_extractor.ginza_extractor import (
    ExtractionResult,
    GinzaConfig,
    GinzaExtractor,
    get_ginza_extractor,
)


class TestGinzaConfig:
    """Tests for GinzaConfig."""

    def test_default_config(self):
        """Test default configuration values."""
        config = GinzaConfig()
        assert config.model_name == "ja_ginza"
        assert config.max_text_length == 50000
        assert config.min_phrase_length == 2
        assert config.max_phrase_length == 30
        assert config.entity_types is None
        assert config.enable_cache is True
        assert config.max_cache_size == 100

    def test_custom_config(self):
        """Test custom configuration."""
        config = GinzaConfig(
            model_name="ja_ginza_electra",
            min_phrase_length=3,
            max_phrase_length=20,
            entity_types=["PERSON", "ORG"],
        )
        assert config.model_name == "ja_ginza_electra"
        assert config.min_phrase_length == 3
        assert config.max_phrase_length == 20
        assert config.entity_types == ["PERSON", "ORG"]


class TestExtractionResult:
    """Tests for ExtractionResult dataclass."""

    def test_default_values(self):
        """Test default extraction result values."""
        result = ExtractionResult()
        assert result.items == []
        assert result.scores == {}
        assert result.metadata == {}

    def test_with_values(self):
        """Test extraction result with values."""
        result = ExtractionResult(
            items=["機械学習", "AI"],
            scores={"機械学習": 0.9, "AI": 0.8},
            metadata={"total_tokens": 100},
        )
        assert len(result.items) == 2
        assert result.scores["機械学習"] == 0.9
        assert result.metadata["total_tokens"] == 100


class TestGinzaExtractor:
    """Tests for GinzaExtractor."""

    @pytest.fixture(autouse=True)
    def reset_singleton(self):
        """Reset singleton before each test."""
        GinzaExtractor._instance = None
        yield
        GinzaExtractor._instance = None

    def test_singleton_pattern(self):
        """Test that GinzaExtractor follows singleton pattern."""
        extractor1 = GinzaExtractor()
        extractor2 = GinzaExtractor()
        assert extractor1 is extractor2

    def test_initialization(self):
        """Test extractor initialization."""
        config = GinzaConfig(min_phrase_length=3)
        extractor = GinzaExtractor(config)
        assert extractor.config.min_phrase_length == 3
        assert extractor._nlp is None
        assert extractor._available is None

    @patch("tag_extractor.ginza_extractor.GinzaExtractor._lazy_load_model")
    def test_is_available_calls_lazy_load(self, mock_load):
        """Test that is_available calls lazy load."""
        mock_load.return_value = True
        extractor = GinzaExtractor()
        result = extractor.is_available()
        mock_load.assert_called_once()
        assert result is True

    def test_extract_noun_phrases_without_model(self):
        """Test noun phrase extraction returns empty when model unavailable."""
        extractor = GinzaExtractor()
        extractor._available = False

        result = extractor.extract_noun_phrases("テスト文章")
        assert result == []

    def test_extract_named_entities_without_model(self):
        """Test NER returns empty when model unavailable."""
        extractor = GinzaExtractor()
        extractor._available = False

        result = extractor.extract_named_entities("テスト文章")
        assert result == []

    @patch("tag_extractor.ginza_extractor.GinzaExtractor._get_doc")
    def test_extract_noun_phrases_with_mock_doc(self, mock_get_doc):
        """Test noun phrase extraction with mocked spaCy doc."""
        # Create mock noun chunks
        mock_chunk1 = MagicMock()
        mock_chunk1.text = "機械学習"
        mock_chunk2 = MagicMock()
        mock_chunk2.text = "ニューラルネットワーク"

        mock_doc = MagicMock()
        mock_doc.noun_chunks = [mock_chunk1, mock_chunk2]
        mock_get_doc.return_value = mock_doc

        extractor = GinzaExtractor()
        result = extractor.extract_noun_phrases("テスト")

        assert "機械学習" in result
        assert "ニューラルネットワーク" in result

    @patch("tag_extractor.ginza_extractor.GinzaExtractor._get_doc")
    def test_extract_named_entities_with_mock_doc(self, mock_get_doc):
        """Test NER with mocked spaCy doc."""
        # Create mock entities
        mock_ent1 = MagicMock()
        mock_ent1.text = "Google"
        mock_ent1.label_ = "ORG"
        mock_ent2 = MagicMock()
        mock_ent2.text = "東京"
        mock_ent2.label_ = "GPE"

        mock_doc = MagicMock()
        mock_doc.ents = [mock_ent1, mock_ent2]
        mock_get_doc.return_value = mock_doc

        extractor = GinzaExtractor()
        result = extractor.extract_named_entities("テスト")

        assert "Google" in result
        assert "東京" in result

    @patch("tag_extractor.ginza_extractor.GinzaExtractor._get_doc")
    def test_extract_named_entities_with_type_filter(self, mock_get_doc):
        """Test NER with entity type filtering."""
        mock_ent1 = MagicMock()
        mock_ent1.text = "Google"
        mock_ent1.label_ = "ORG"
        mock_ent2 = MagicMock()
        mock_ent2.text = "東京"
        mock_ent2.label_ = "GPE"

        mock_doc = MagicMock()
        mock_doc.ents = [mock_ent1, mock_ent2]
        mock_get_doc.return_value = mock_doc

        # Reset singleton and create with filter
        GinzaExtractor._instance = None
        config = GinzaConfig(entity_types=["ORG"])
        extractor = GinzaExtractor(config)

        result = extractor.extract_named_entities("テスト")

        assert "Google" in result
        assert "東京" not in result

    @patch("tag_extractor.ginza_extractor.GinzaExtractor._get_doc")
    def test_deduplication(self, mock_get_doc):
        """Test that duplicate phrases are removed."""
        mock_chunk1 = MagicMock()
        mock_chunk1.text = "機械学習"
        mock_chunk2 = MagicMock()
        mock_chunk2.text = "機械学習"  # Duplicate
        mock_chunk3 = MagicMock()
        mock_chunk3.text = "AI"

        mock_doc = MagicMock()
        mock_doc.noun_chunks = [mock_chunk1, mock_chunk2, mock_chunk3]
        mock_get_doc.return_value = mock_doc

        extractor = GinzaExtractor()
        result = extractor.extract_noun_phrases("テスト")

        assert result.count("機械学習") == 1
        assert "AI" in result

    @patch("tag_extractor.ginza_extractor.GinzaExtractor._get_doc")
    def test_length_filter(self, mock_get_doc):
        """Test that phrases outside length bounds are filtered."""
        mock_chunk1 = MagicMock()
        mock_chunk1.text = "A"  # Too short
        mock_chunk2 = MagicMock()
        mock_chunk2.text = "機械学習"
        mock_chunk3 = MagicMock()
        mock_chunk3.text = "A" * 50  # Too long

        mock_doc = MagicMock()
        mock_doc.noun_chunks = [mock_chunk1, mock_chunk2, mock_chunk3]
        mock_get_doc.return_value = mock_doc

        extractor = GinzaExtractor()
        result = extractor.extract_noun_phrases("テスト")

        assert "A" not in result
        assert "機械学習" in result
        assert len([r for r in result if len(r) > 30]) == 0

    @patch("tag_extractor.ginza_extractor.GinzaExtractor._get_doc")
    def test_extract_with_scores(self, mock_get_doc):
        """Test extraction with scores."""
        mock_chunk = MagicMock()
        mock_chunk.text = "機械学習"

        mock_ent = MagicMock()
        mock_ent.text = "Google"
        mock_ent.label_ = "ORG"

        mock_doc = MagicMock()
        mock_doc.noun_chunks = [mock_chunk]
        mock_doc.ents = [mock_ent]
        mock_doc.__len__ = MagicMock(return_value=10)
        mock_get_doc.return_value = mock_doc

        extractor = GinzaExtractor()
        result = extractor.extract_with_scores("テスト")

        assert isinstance(result, ExtractionResult)
        assert len(result.items) > 0
        assert len(result.scores) > 0

    def test_clear_cache(self):
        """Test cache clearing."""
        extractor = GinzaExtractor()
        extractor._cache = {"key": "value"}
        extractor.clear_cache()
        assert extractor._cache == {}


class TestGetGinzaExtractor:
    """Tests for get_ginza_extractor function."""

    @pytest.fixture(autouse=True)
    def reset_singleton(self):
        """Reset singleton before each test."""
        GinzaExtractor._instance = None
        yield
        GinzaExtractor._instance = None

    def test_returns_singleton(self):
        """Test that function returns singleton instance."""
        extractor1 = get_ginza_extractor()
        extractor2 = get_ginza_extractor()
        assert extractor1 is extractor2

    def test_accepts_config(self):
        """Test that function accepts configuration."""
        config = GinzaConfig(min_phrase_length=5)
        extractor = get_ginza_extractor(config)
        assert extractor.config.min_phrase_length == 5
