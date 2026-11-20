"""
Unit tests for hybrid tag extractor.
"""

import re
import sys
from collections.abc import Callable
from typing import cast
from unittest.mock import MagicMock, patch

import pytest

from tag_extractor.hybrid_extractor import (
    CandidateTag,
    HybridConfig,
    HybridExtractionResult,
    HybridExtractor,
    get_hybrid_extractor,
)


# Mock sklearn.feature_extraction.text.CountVectorizer for test environments
# that don't have sklearn installed (same as test_keybert_japanese_scoring.py)
class MockCountVectorizer:
    """Mock CountVectorizer that tracks initialization parameters."""

    def __init__(
        self,
        lowercase: bool = True,
        token_pattern: str | None = None,
        analyzer: Callable[[str], list[str]] | str | None = None,
        **kwargs: object,
    ) -> None:
        self.lowercase = lowercase
        self.token_pattern = token_pattern
        self.analyzer = analyzer
        self._kwargs = kwargs


# Install mock module before tests if sklearn is not available
if "sklearn" not in sys.modules:
    mock_sklearn = MagicMock()
    mock_sklearn.feature_extraction.text.CountVectorizer = MockCountVectorizer
    sys.modules["sklearn"] = mock_sklearn
    sys.modules["sklearn.feature_extraction"] = mock_sklearn.feature_extraction
    sys.modules["sklearn.feature_extraction.text"] = mock_sklearn.feature_extraction.text


class TestHybridConfig:
    """Tests for HybridConfig."""

    def test_default_config(self) -> None:
        """Test default configuration values."""
        default_top_k = 10
        default_min_score = 0.1
        default_ginza_weight = 1.2
        default_frequency_weight = 0.4
        default_position_weight = 0.2
        default_semantic_weight = 0.4
        config = HybridConfig()
        assert config.top_k == default_top_k
        assert config.min_score == default_min_score
        assert config.use_ginza is True
        assert config.use_keybert_scoring is True
        assert config.ginza_weight == default_ginza_weight
        assert config.fugashi_weight == 1.0
        assert config.frequency_weight == default_frequency_weight
        assert config.position_weight == default_position_weight
        assert config.semantic_weight == default_semantic_weight

    def test_custom_config(self) -> None:
        """Test custom configuration."""
        top_k = 5
        ginza_weight = 1.5
        config = HybridConfig(
            top_k=top_k,
            use_ginza=False,
            ginza_weight=ginza_weight,
        )
        assert config.top_k == top_k
        assert config.use_ginza is False
        assert config.ginza_weight == ginza_weight


class TestCandidateTag:
    """Tests for CandidateTag dataclass."""

    def test_default_values(self) -> None:
        """Test default candidate tag values."""
        tag = CandidateTag(text="機械学習", source="ginza")
        assert tag.text == "機械学習"
        assert tag.source == "ginza"
        assert tag.frequency == 1
        assert tag.position == 0
        assert tag.semantic_score == 0.0
        assert tag.combined_score == 0.0

    def test_with_values(self) -> None:
        """Test candidate tag with all values."""
        frequency = 5
        position = 10
        semantic_score = 0.8
        combined_score = 0.75
        tag = CandidateTag(
            text="AI",
            source="fugashi",
            frequency=frequency,
            position=position,
            semantic_score=semantic_score,
            combined_score=combined_score,
        )
        assert tag.frequency == frequency
        assert tag.position == position
        assert tag.semantic_score == semantic_score
        assert tag.combined_score == combined_score


class TestHybridExtractionResult:
    """Tests for HybridExtractionResult dataclass."""

    def test_default_metadata(self) -> None:
        """Test default extraction result values."""
        result = HybridExtractionResult(
            tags=["機械学習"],
            tag_scores={"機械学習": 0.9},
            inference_ms=100.0,
        )
        assert result.metadata == {}

    def test_with_metadata(self) -> None:
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
    def extractor(self) -> HybridExtractor:
        """Create a HybridExtractor for testing."""
        config = HybridConfig(use_ginza=False, use_keybert_scoring=False)
        return HybridExtractor(config)

    def test_initialization(self) -> None:
        """Test extractor initialization."""
        top_k = 5
        config = HybridConfig(top_k=top_k)
        extractor = HybridExtractor(config)
        assert extractor.config.top_k == top_k
        assert extractor._ginza is None
        assert extractor._fugashi_tagger is None
        assert extractor._keybert is None
        assert extractor._models_loaded is False

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_calls_lazy_load(self, mock_load: MagicMock, extractor: HybridExtractor) -> None:
        """Test that extract_tags triggers lazy loading."""
        extractor.extract_tags("タイトル", "コンテンツ")
        mock_load.assert_called_once()

    def test_extract_candidates_fugashi_without_tagger(self, extractor: HybridExtractor) -> None:
        """Test Fugashi extraction returns empty when tagger unavailable."""
        extractor._fugashi_tagger = None
        result = extractor._extract_candidates_fugashi("テスト文章")
        assert result == []

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._extract_candidates_fugashi")
    @patch("tag_extractor.hybrid_extractor.HybridExtractor._extract_candidates_ginza")
    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_combines_sources(
        self, mock_load: MagicMock, mock_ginza: MagicMock, mock_fugashi: MagicMock, extractor: HybridExtractor
    ) -> None:
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

    def test_compute_combined_scores_empty(self, extractor: HybridExtractor) -> None:
        """Empty candidates must short-circuit before the max()-over-empty-sequence
        call that would otherwise raise ValueError, and return None (in-place mutation
        contract, nothing to mutate)."""
        candidates: list[CandidateTag] = []
        result = extractor._compute_combined_scores(candidates)
        assert result is None
        assert candidates == []

    def test_compute_combined_scores(self, extractor: HybridExtractor) -> None:
        """Test combined score computation."""
        candidates = [
            CandidateTag(text="機械学習", source="ginza", frequency=3, position=0),
            CandidateTag(text="AI", source="fugashi", frequency=1, position=5),
        ]
        extractor._compute_combined_scores(candidates)

        # GiNZA source should have higher base weight
        assert candidates[0].combined_score > 0
        assert candidates[1].combined_score > 0

    def test_deduplicate_candidates(self, extractor: HybridExtractor) -> None:
        """Test candidate deduplication."""
        higher_score = 0.8
        candidates = [
            CandidateTag(text="機械学習", source="ginza", combined_score=higher_score),
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
        assert ml_candidate.combined_score == higher_score

    def test_score_with_keybert_without_model(self, extractor: HybridExtractor) -> None:
        """Test KeyBERT scoring with no model does nothing."""
        candidates = [CandidateTag(text="test", source="fugashi")]
        extractor._keybert = None
        extractor._score_with_keybert("test text", candidates)
        # Should not raise, semantic_score should remain 0
        assert candidates[0].semantic_score == 0.0

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_returns_list(self, mock_load: MagicMock, extractor: HybridExtractor) -> None:
        """Test that extract_tags returns a list of strings."""
        result = extractor.extract_tags("タイトル", "コンテンツ")
        assert isinstance(result, list)

    @patch("tag_extractor.hybrid_extractor.HybridExtractor._lazy_load_models")
    def test_extract_tags_with_result_returns_result(self, mock_load: MagicMock, extractor: HybridExtractor) -> None:
        """Test that extract_tags_with_result returns HybridExtractionResult."""
        result = extractor.extract_tags_with_result("タイトル", "コンテンツ")
        assert isinstance(result, HybridExtractionResult)
        assert isinstance(result.tags, list)
        assert isinstance(result.tag_scores, dict)
        assert isinstance(result.inference_ms, float)
        assert "ginza_available" in result.metadata

    def test_extract_candidates_ginza_without_ginza(self, extractor: HybridExtractor) -> None:
        """Test GiNZA extraction returns empty when not available."""
        extractor._ginza = None
        result = extractor._extract_candidates_ginza("テスト")
        assert result == []


class TestHybridExtractorWithFugashi:
    """Tests for HybridExtractor with mocked Fugashi."""

    @pytest.fixture
    def extractor_with_fugashi(self) -> HybridExtractor:
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

    def test_extract_candidates_fugashi_chains_nouns(self, extractor_with_fugashi: HybridExtractor) -> None:
        """Test that Fugashi extraction chains consecutive nouns."""
        candidates = extractor_with_fugashi._extract_candidates_fugashi("機械学習は")

        # Should extract "機械学習" as compound
        compound_texts = [c.text for c in candidates]
        assert "機械学習" in compound_texts

    def test_extract_candidates_fugashi_skips_stopwords(self, extractor_with_fugashi: HybridExtractor) -> None:
        """Test that stopwords are skipped."""
        extractor_with_fugashi._stopwords = {"機械"}

        candidates = extractor_with_fugashi._extract_candidates_fugashi("機械学習は")

        # "機械" should be skipped
        texts = [c.text for c in candidates]
        assert "機械" not in texts


class TestGetHybridExtractor:
    """Tests for get_hybrid_extractor function."""

    def test_returns_extractor(self) -> None:
        """Test that function returns HybridExtractor."""
        extractor = get_hybrid_extractor()
        assert isinstance(extractor, HybridExtractor)

    def test_accepts_config(self) -> None:
        """Test that function accepts configuration."""
        top_k = 3
        config = HybridConfig(top_k=top_k)
        extractor = get_hybrid_extractor(config)
        assert extractor.config.top_k == top_k


class TestHybridExtractorKeyBERTScoring:
    """Tests for HybridExtractor KeyBERT scoring with CountVectorizer fix."""

    @pytest.fixture
    def extractor_with_keybert(self) -> HybridExtractor:
        """Create extractor with mocked KeyBERT."""
        config = HybridConfig(use_ginza=False, use_keybert_scoring=True)
        extractor = HybridExtractor(config)

        # Mock KeyBERT
        mock_keybert = MagicMock()
        extractor._keybert = mock_keybert
        extractor._models_loaded = True

        return extractor

    def test_keybert_scoring_uses_vectorizer(self, extractor_with_keybert: HybridExtractor) -> None:
        """Test that KeyBERT scoring passes a custom vectorizer."""
        candidates = [
            CandidateTag(text="GitHub", source="fugashi"),
            CandidateTag(text="AWS", source="fugashi"),
        ]

        # Capture the vectorizer passed to KeyBERT
        captured_args = {}

        def mock_extract(*args: object, **kwargs: object) -> list[tuple[str, float]]:
            captured_args.update(kwargs)
            return [("GitHub", 0.8), ("AWS", 0.7)]

        extractor_with_keybert._keybert.extract_keywords = mock_extract

        extractor_with_keybert._score_with_keybert("GitHubとAWS", candidates)

        # Verify vectorizer was passed
        assert "vectorizer" in captured_args, "vectorizer parameter should be passed to KeyBERT"
        vectorizer = captured_args["vectorizer"]
        assert vectorizer is not None, "vectorizer should not be None"

    def test_keybert_vectorizer_has_lowercase_false(self, extractor_with_keybert: HybridExtractor) -> None:
        """Test that the vectorizer has lowercase=False for uppercase matching."""
        candidates = [CandidateTag(text="API", source="fugashi")]

        captured_vectorizer = {}

        def mock_extract(*args: object, **kwargs: object) -> list[tuple[str, float]]:
            captured_vectorizer["instance"] = kwargs.get("vectorizer")
            return [("API", 0.9)]

        extractor_with_keybert._keybert.extract_keywords = mock_extract

        extractor_with_keybert._score_with_keybert("API連携", candidates)

        vectorizer = captured_vectorizer.get("instance")
        assert vectorizer is not None, "Vectorizer should be provided"
        assert hasattr(vectorizer, "lowercase"), "Vectorizer should have lowercase attribute"
        assert vectorizer.lowercase is False, (
            "Vectorizer must have lowercase=False to match uppercase candidates (ADR-176 fix)"
        )

    def test_uppercase_candidates_get_scored(self, extractor_with_keybert: HybridExtractor) -> None:
        """Test that uppercase candidates are scored correctly after the fix."""
        candidates = [
            CandidateTag(text="GitHub", source="fugashi"),
            CandidateTag(text="TensorFlow", source="fugashi"),
            CandidateTag(text="コード", source="fugashi"),
        ]

        def mock_extract(*args: object, **kwargs: object) -> list[tuple[str, float]]:
            vectorizer = kwargs.get("vectorizer")
            # Simulate proper behavior when vectorizer has lowercase=False
            if vectorizer and hasattr(vectorizer, "lowercase") and not vectorizer.lowercase:
                return [("GitHub", 0.85), ("TensorFlow", 0.75), ("コード", 0.65)]
            # Simulate the bug: no results when vectorizer not properly configured
            return []

        extractor_with_keybert._keybert.extract_keywords = mock_extract

        extractor_with_keybert._score_with_keybert("GitHubでコードを管理", candidates)

        # After fix, uppercase candidates should have non-zero semantic scores
        github_candidate = next(c for c in candidates if c.text == "GitHub")
        assert github_candidate.semantic_score > 0, "GitHub should have a semantic score after fix"


class TestHybridExtractorTagValidation:
    """Tests for _is_valid_tag method in HybridExtractor."""

    @pytest.fixture
    def extractor(self) -> HybridExtractor:
        """Create HybridExtractor for testing."""
        config = HybridConfig(use_ginza=False, use_keybert_scoring=False)
        return HybridExtractor(config)

    # Verb ending tests
    def test_rejects_desu_ending(self, extractor: HybridExtractor) -> None:
        """Tags ending with です should be rejected."""
        assert extractor._is_valid_tag("便利です") is False

    def test_rejects_masu_ending(self, extractor: HybridExtractor) -> None:
        """Tags ending with ます should be rejected."""
        assert extractor._is_valid_tag("使います") is False

    def test_rejects_mashita_ending(self, extractor: HybridExtractor) -> None:
        """Tags ending with ました should be rejected."""
        assert extractor._is_valid_tag("完了しました") is False

    def test_rejects_teiru_ending(self, extractor: HybridExtractor) -> None:
        """Tags ending with ている should be rejected."""
        assert extractor._is_valid_tag("動いている") is False

    def test_rejects_shita_ending(self, extractor: HybridExtractor) -> None:
        """Tags ending with した should be rejected."""
        assert extractor._is_valid_tag("実装した") is False

    def test_rejects_suru_ending(self, extractor: HybridExtractor) -> None:
        """Tags ending with する should be rejected."""
        assert extractor._is_valid_tag("実行する") is False

    # Number-only tests
    def test_rejects_number_only(self, extractor: HybridExtractor) -> None:
        """Tags that are numbers only should be rejected."""
        assert extractor._is_valid_tag("2025") is False
        assert extractor._is_valid_tag("12") is False
        assert extractor._is_valid_tag("100") is False

    def test_accepts_number_with_text(self, extractor: HybridExtractor) -> None:
        """Tags with numbers and text should be accepted."""
        assert extractor._is_valid_tag("Web3") is True
        assert extractor._is_valid_tag("5G通信") is True
        assert extractor._is_valid_tag("iOS17") is True

    # Valid tag tests
    def test_accepts_valid_tags(self, extractor: HybridExtractor) -> None:
        """Valid tags should be accepted."""
        valid_tags = [
            "機械学習",
            "TensorFlow",
            "GitHub",
            "AWS",
            "API",
            "Python",
        ]
        for tag in valid_tags:
            assert extractor._is_valid_tag(tag) is True, f"Tag '{tag}' should be valid"


class TestHybridExtractorFugashiFiltering:
    """Tests for Fugashi candidate filtering with _is_valid_tag."""

    @pytest.fixture
    def extractor_with_fugashi(self) -> HybridExtractor:
        """Create extractor with mocked Fugashi."""
        config = HybridConfig(use_ginza=False, use_keybert_scoring=False)
        extractor = HybridExtractor(config)

        # Mock Fugashi tagger
        mock_tagger = MagicMock()
        extractor._fugashi_tagger = mock_tagger
        extractor._stopwords = set()
        extractor._models_loaded = True

        return extractor

    def _make_token(self, surface: str, pos1: str) -> MagicMock:
        """Helper to create a mock token."""
        token = MagicMock()
        token.surface = surface
        token.feature = MagicMock()
        token.feature.pos1 = pos1
        return token

    def test_filters_verb_ending_compounds(self, extractor_with_fugashi: HybridExtractor) -> None:
        """Compounds ending with verbs should be filtered."""
        # Mock tokens that would produce "実装した"
        tokens = [
            self._make_token("実装", "名詞"),
            self._make_token("した", "動詞"),
        ]
        extractor_with_fugashi._fugashi_tagger.return_value = tokens

        candidates = extractor_with_fugashi._extract_candidates_fugashi("実装した")

        # Verb-ending compounds should not appear
        texts = [c.text for c in candidates]
        assert not any("した" in t for t in texts)

    def test_filters_number_only_candidates(self, extractor_with_fugashi: HybridExtractor) -> None:
        """Number-only candidates should be filtered."""
        tokens = [
            self._make_token("2025", "名詞-数詞"),
        ]
        extractor_with_fugashi._fugashi_tagger.return_value = tokens

        candidates = extractor_with_fugashi._extract_candidates_fugashi("2025")

        texts = [c.text for c in candidates]
        assert "2025" not in texts


class TestHybridExtractorGinzaPostProcessing:
    """Tests for GiNZA noun_chunks post-processing.

    GiNZA's noun_chunks can extract phrases with trailing particles or verbs.
    These should be cleaned and validated before being used as tags.
    """

    @pytest.fixture
    def extractor_with_mock_ginza(self) -> HybridExtractor:
        """Create extractor with mocked GiNZA."""
        config = HybridConfig(use_ginza=True, use_keybert_scoring=False)
        extractor = HybridExtractor(config)

        # Mock GiNZA extractor
        mock_ginza = MagicMock()
        extractor._ginza = mock_ginza
        extractor._stopwords = set()
        extractor._models_loaded = True

        return extractor

    def test_cleans_particle_endings_from_ginza(self, extractor_with_mock_ginza: HybridExtractor) -> None:
        """Noun phrases with particle endings should be cleaned."""
        # Mock GiNZA returning phrases with particles
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_noun_phrases.return_value = [
            "Databricksのセキュリティは",
            "Unity Catalog",
            "データガバナンス",
        ]
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_named_entities.return_value = []

        candidates = extractor_with_mock_ginza._extract_candidates_ginza("test")
        texts = [c.text for c in candidates]

        # Particle endings should not appear in final candidates
        assert not any(t.endswith("は") for t in texts)

    def test_cleans_verb_endings_from_ginza(self, extractor_with_mock_ginza: HybridExtractor) -> None:
        """Noun phrases with verb endings should be cleaned."""
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_noun_phrases.return_value = [
            "TablesはDatabricksが管理する",
            "セキュリティ設計",
        ]
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_named_entities.return_value = []

        candidates = extractor_with_mock_ginza._extract_candidates_ginza("test")
        texts = [c.text for c in candidates]

        # Verb endings should not appear in final candidates
        assert not any(t.endswith("する") for t in texts)

    def test_preserves_valid_noun_phrases(self, extractor_with_mock_ginza: HybridExtractor) -> None:
        """Valid noun phrases should be preserved unchanged."""
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_noun_phrases.return_value = [
            "Databricks",
            "セキュリティ",
            "Unity Catalog",
            "データガバナンス",
        ]
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_named_entities.return_value = []

        candidates = extractor_with_mock_ginza._extract_candidates_ginza("test")
        texts = [c.text for c in candidates]

        # Valid terms should be preserved
        assert "Databricks" in texts
        assert "セキュリティ" in texts
        assert "Unity Catalog" in texts

    def test_filters_sentence_fragments(self, extractor_with_mock_ginza: HybridExtractor) -> None:
        """Long sentence fragments should be filtered out."""
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_noun_phrases.return_value = [
            "Databricks運用の重要ポイントを網羅する内容になっています",
            "がセキュリティ設計の鉄則です",
            "セキュリティ",
        ]
        cast(MagicMock, extractor_with_mock_ginza._ginza).extract_named_entities.return_value = []

        candidates = extractor_with_mock_ginza._extract_candidates_ginza("test")
        texts = [c.text for c in candidates]

        # Long sentence fragments should be filtered
        max_tag_length = 15
        assert not any(len(t) > max_tag_length for t in texts)
        # But valid short terms should remain
        assert "セキュリティ" in texts


class TestHybridEmbedderRevision:
    def test_keybert_embedder_loaded_with_full_commit_sha(self) -> None:
        fake_st = MagicMock()
        extractor = HybridExtractor(HybridConfig(use_ginza=False, use_keybert_scoring=True))
        with patch.dict(
            sys.modules,
            {
                "sentence_transformers": MagicMock(SentenceTransformer=fake_st),
                "keybert": MagicMock(),
            },
        ):
            extractor._lazy_load_models()
        assert re.fullmatch(r"[0-9a-f]{40}", fake_st.call_args.kwargs["revision"])

    def test_score_with_keybert_catches_unexpected_exception(self) -> None:
        """Unexpected KeyBERT/sklearn failures fall back cleanly instead of failing extraction."""

        class _UnexpectedLibraryError(Exception):
            pass

        extractor = HybridExtractor(HybridConfig(use_ginza=False, use_keybert_scoring=True))
        mock_kb = MagicMock()
        mock_kb.extract_keywords.side_effect = _UnexpectedLibraryError("unexpected")
        extractor._keybert = mock_kb
        candidates = [CandidateTag(text="AI", source="fugashi")]

        extractor._score_with_keybert("AI text", candidates)

        assert candidates[0].semantic_score == 0.0
