"""
Unit tests for hybrid tag extractor.
"""

from unittest.mock import MagicMock, patch

import pytest

from tag_extractor.hybrid_extractor import (
    CandidateTag,
    HybridConfig,
    HybridExtractionResult,
    HybridExtractor,
    get_hybrid_extractor,
)


class TestHybridConfig:
    """Tests for HybridConfig."""

    def test_default_config(self):
        """Test default configuration values."""
        config = HybridConfig()
        assert config.top_k == 10
        assert config.min_score == 0.1
        assert config.use_ginza is True
        assert config.use_keybert_scoring is True
        assert config.ginza_weight == 1.2
        assert config.fugashi_weight == 1.0
        assert config.frequency_weight == 0.4
        assert config.position_weight == 0.2
        assert config.semantic_weight == 0.4

    def test_custom_config(self):
        """Test custom configuration."""
        config = HybridConfig(
            top_k=5,
            use_ginza=False,
            ginza_weight=1.5,
        )
        assert config.top_k == 5
        assert config.use_ginza is False
        assert config.ginza_weight == 1.5


class TestCandidateTag:
    """Tests for CandidateTag dataclass."""

    def test_default_values(self):
        """Test default candidate tag values."""
        tag = CandidateTag(text="機械学習", source="ginza")
        assert tag.text == "機械学習"
        assert tag.source == "ginza"
        assert tag.frequency == 1
        assert tag.position == 0
        assert tag.semantic_score == 0.0
        assert tag.combined_score == 0.0

    def test_with_values(self):
        """Test candidate tag with all values."""
        tag = CandidateTag(
            text="AI",
            source="fugashi",
            frequency=5,
            position=10,
            semantic_score=0.8,
            combined_score=0.75,
        )
        assert tag.frequency == 5
        assert tag.position == 10
        assert tag.semantic_score == 0.8
        assert tag.combined_score == 0.75


class TestHybridExtractionResult:
    """Tests for HybridExtractionResult dataclass."""

    def test_default_metadata(self):
        """Test default extraction result values."""
        result = HybridExtractionResult(
            tags=["機械学習"],
            tag_scores={"機械学習": 0.9},
            inference_ms=100.0,
        )
        assert result.metadata == {}

    def test_with_metadata(self):
        """Test extraction result with metadata."""
        result = HybridExtractionResult(
            tags=["AI", "ML"],
            tag_scores={"AI": 0.9, "ML": 0.8},
            inference_ms=50.0,
            metadata={"ginza_available": True},
        )
        assert result.metadata["ginza_available"] is True


class TestHybridExtractor:
    """Tests for HybridExtractor."""

    @pytest.fixture
    def extractor(self):
        """Create a HybridExtractor for testing."""
        config = HybridConfig(use_ginza=False, use_keybert_scoring=False)
        return HybridExtractor(config)

    def test_initialization(self):
        """Test extractor initialization."""
        config = HybridConfig(top_k=5)
        extractor = HybridExtractor(config)
        assert extractor.config.top_k == 5
        assert extractor._ginza is None
        assert extractor._fugashi_tagger is None
        assert extractor._keybert is None
        assert extractor._models_loaded is False

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_calls_lazy_load(self, mock_load, extractor):
        """Test that extract_tags triggers lazy loading."""
        extractor.extract_tags("タイトル", "コンテンツ")
        mock_load.assert_called_once()

    def test_extract_candidates_fugashi_without_tagger(self, extractor):
        """Test Fugashi extraction returns empty when tagger unavailable."""
        extractor._fugashi_tagger = None
        result = extractor._extract_candidates_fugashi("テスト文章")
        assert result == []

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._extract_candidates_fugashi")
    @patch("tag_extractor.hybrid_extractor.HybridExtractor._extract_candidates_ginza")
    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_combines_sources(self, mock_load, mock_ginza, mock_fugashi, extractor):
        """Test that extraction combines candidates from multiple sources."""
        mock_ginza.return_value = [
            CandidateTag(text="機械学習", source="ginza", frequency=1),
        ]
        mock_fugashi.return_value = [
            CandidateTag(text="AI", source="fugashi", frequency=2),
        ]

        extractor.extract_tags("タイトル", "AI機械学習")

        # Both sources should be called
        mock_ginza.assert_called_once()
        mock_fugashi.assert_called_once()

    def test_compute_combined_scores_empty(self, extractor):
        """Test combined score computation with empty candidates."""
        extractor._compute_combined_scores([])
        # Should not raise

    def test_compute_combined_scores(self, extractor):
        """Test combined score computation."""
        candidates = [
            CandidateTag(text="機械学習", source="ginza", frequency=3, position=0),
            CandidateTag(text="AI", source="fugashi", frequency=1, position=5),
        ]
        extractor._compute_combined_scores(candidates)

        # GiNZA source should have higher base weight
        assert candidates[0].combined_score > 0
        assert candidates[1].combined_score > 0

    def test_deduplicate_candidates(self, extractor):
        """Test candidate deduplication."""
        candidates = [
            CandidateTag(text="機械学習", source="ginza", combined_score=0.8),
            CandidateTag(text="機械学習", source="fugashi", combined_score=0.5),
            CandidateTag(text="AI", source="ginza", combined_score=0.7),
        ]
        result = extractor._deduplicate_candidates(candidates)

        # Should keep only one "機械学習" with higher score
        texts = [c.text for c in result]
        assert texts.count("機械学習") == 1
        assert "AI" in texts

        # Should keep the higher-scoring version
        ml_candidate = next(c for c in result if c.text == "機械学習")
        assert ml_candidate.combined_score == 0.8

    def test_score_with_keybert_without_model(self, extractor):
        """Test KeyBERT scoring with no model does nothing."""
        candidates = [CandidateTag(text="test", source="fugashi")]
        extractor._keybert = None
        extractor._score_with_keybert("test text", candidates)
        # Should not raise, semantic_score should remain 0
        assert candidates[0].semantic_score == 0.0

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_returns_list(self, mock_load, extractor):
        """Test that extract_tags returns a list of strings."""
        result = extractor.extract_tags("タイトル", "コンテンツ")
        assert isinstance(result, list)

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_with_result_returns_result(self, mock_load, extractor):
        """Test that extract_tags_with_result returns HybridExtractionResult."""
        result = extractor.extract_tags_with_result("タイトル", "コンテンツ")
        assert isinstance(result, HybridExtractionResult)
        assert isinstance(result.tags, list)
        assert isinstance(result.tag_scores, dict)
        assert isinstance(result.inference_ms, float)
        assert "ginza_available" in result.metadata

    def test_extract_candidates_ginza_without_ginza(self, extractor):
        """Test GiNZA extraction returns empty when not available."""
        extractor._ginza = None
        result = extractor._extract_candidates_ginza("テスト")
        assert result == []


class TestHybridExtractorWithFugashi:
    """Tests for HybridExtractor with mocked Fugashi."""

    @pytest.fixture
    def extractor_with_fugashi(self):
        """Create extractor with mocked Fugashi."""
        config = HybridConfig(use_ginza=False, use_keybert_scoring=False)
        extractor = HybridExtractor(config)

        # Mock Fugashi tagger
        mock_tagger = MagicMock()

        # Create mock tokens
        token1 = MagicMock()
        token1.surface = "機械"
        token1.feature = MagicMock()
        token1.feature.pos1 = "名詞"

        token2 = MagicMock()
        token2.surface = "学習"
        token2.feature = MagicMock()
        token2.feature.pos1 = "名詞"

        token3 = MagicMock()
        token3.surface = "は"
        token3.feature = MagicMock()
        token3.feature.pos1 = "助詞"

        mock_tagger.return_value = [token1, token2, token3]
        extractor._fugashi_tagger = mock_tagger
        extractor._stopwords = set()
        extractor._models_loaded = True

        return extractor

    def test_extract_candidates_fugashi_chains_nouns(self, extractor_with_fugashi):
        """Test that Fugashi extraction chains consecutive nouns."""
        candidates = extractor_with_fugashi._extract_candidates_fugashi("機械学習は")

        # Should extract "機械学習" as compound
        compound_texts = [c.text for c in candidates]
        assert "機械学習" in compound_texts

    def test_extract_candidates_fugashi_skips_stopwords(self, extractor_with_fugashi):
        """Test that stopwords are skipped."""
        extractor_with_fugashi._stopwords = {"機械"}

        candidates = extractor_with_fugashi._extract_candidates_fugashi("機械学習は")

        # "機械" should be skipped
        texts = [c.text for c in candidates]
        assert "機械" not in texts


class TestGetHybridExtractor:
    """Tests for get_hybrid_extractor function."""

    def test_returns_extractor(self):
        """Test that function returns HybridExtractor."""
        extractor = get_hybrid_extractor()
        assert isinstance(extractor, HybridExtractor)

    def test_accepts_config(self):
        """Test that function accepts configuration."""
        config = HybridConfig(top_k=3)
        extractor = get_hybrid_extractor(config)
        assert extractor.config.top_k == 3
