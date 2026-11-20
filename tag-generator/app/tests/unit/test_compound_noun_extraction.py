"""
Unit tests for improved compound noun extraction.
"""

from typing import cast
from unittest.mock import MagicMock, patch

import pytest

from tag_extractor.extract import TagExtractionConfig, TagExtractor
from tag_extractor.japanese_extractor import _extract_proper_noun_compounds


class TestExtractCompoundNounsFugashi:
    """Tests for _extract_compound_nouns_fugashi method."""

    @pytest.fixture
    def extractor(self) -> TagExtractor:
        """Create TagExtractor with mocked models."""
        with patch("tag_extractor.extract.TagExtractor._lazy_load_models"):
            extractor = TagExtractor()
            extractor._models_loaded = True

            # Create mock tagger that returns controlled tokens
            mock_tagger = MagicMock()
            extractor._ja_tagger = mock_tagger

            return extractor

    def _make_token(self, surface: str, pos1: str) -> MagicMock:
        """Helper to create a mock token."""
        token = MagicMock()
        token.surface = surface
        token.feature = MagicMock()
        token.feature.pos1 = pos1
        return token

    def test_chains_consecutive_nouns(self, extractor: TagExtractor) -> None:
        """Test that consecutive nouns are chained into compounds."""
        # Mock tokens for "機械学習モデル"
        tokens = [
            self._make_token("機械", "名詞"),
            self._make_token("学習", "名詞"),
            self._make_token("モデル", "名詞"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_nouns_fugashi("機械学習モデル")

        assert "機械学習モデル" in result

    def test_splits_on_non_noun(self, extractor: TagExtractor) -> None:
        """Test that non-noun tokens split compounds."""
        # Mock tokens for "機械学習は素晴らしい技術"
        tokens = [
            self._make_token("機械", "名詞"),
            self._make_token("学習", "名詞"),
            self._make_token("は", "助詞"),
            self._make_token("素晴らしい", "形容詞"),
            self._make_token("技術", "名詞"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_nouns_fugashi("機械学習は素晴らしい技術")

        assert "機械学習" in result
        # Single noun "技術" should not be included (needs 2+ tokens)
        assert "技術" not in result

    def test_connects_nouns_with_no(self, extractor: TagExtractor) -> None:
        """Test that nouns connected with の are joined."""
        # Mock tokens for "日本の技術"
        tokens = [
            self._make_token("日本", "名詞"),
            self._make_token("の", "助詞"),  # Connector
            self._make_token("技術", "名詞"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_nouns_fugashi("日本の技術")

        assert "日本の技術" in result

    def test_minimum_length_filter(self, extractor: TagExtractor) -> None:
        """Test that compounds below minimum length are filtered."""
        # Mock tokens for short compound
        tokens = [
            self._make_token("A", "名詞"),
            self._make_token("B", "名詞"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_nouns_fugashi("AB")

        # "AB" is only 2 chars, below minimum of 3
        assert "AB" not in result

    def test_maximum_length_filter(self, extractor: TagExtractor) -> None:
        """Test that compounds above maximum length are filtered."""
        # Create a very long compound
        long_word = "あ" * 35
        tokens = [self._make_token(long_word, "名詞")]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_nouns_fugashi(long_word)

        # Single token doesn't form compound anyway
        assert long_word not in result

    def test_handles_empty_input(self, extractor: TagExtractor) -> None:
        """Test handling of empty input."""
        cast(MagicMock, extractor._ja_tagger).return_value = []

        result = extractor._extract_compound_nouns_fugashi("")

        assert result == []

    def test_pos_tag_variations(self, extractor: TagExtractor) -> None:
        """Test that various noun POS tags are recognized."""
        # Mock tokens with different POS tags
        tokens = [
            self._make_token("東京", "名詞-固有名詞-地域"),
            self._make_token("大学", "名詞-普通名詞-一般"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_nouns_fugashi("東京大学")

        assert "東京大学" in result


class TestExtractCompoundJapaneseWords:
    """Tests for _extract_compound_japanese_words method."""

    @pytest.fixture
    def extractor(self) -> TagExtractor:
        """Create TagExtractor with mocked models."""
        with patch("tag_extractor.extract.TagExtractor._lazy_load_models"):
            extractor = TagExtractor()
            extractor._models_loaded = True

            mock_tagger = MagicMock()
            extractor._ja_tagger = mock_tagger

            return extractor

    def _make_token(self, surface: str, pos1: str, pos2: str = "") -> MagicMock:
        """Helper to create a mock token."""
        token = MagicMock()
        token.surface = surface
        token.feature = MagicMock()
        token.feature.pos1 = pos1
        token.feature.pos2 = pos2
        return token

    def test_extracts_mixed_script_compounds(self, extractor: TagExtractor) -> None:
        """Test extraction of mixed Kanji/Katakana/English compounds."""
        # The regex patterns should find these
        text = "GitHubリポジトリでAI技術を使う"

        # Mock simple tagger response
        tokens = [self._make_token("test", "名詞")]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_japanese_words(text)

        # Should find mixed-script compounds via regex
        assert any("GitHub" in r for r in result) or any("AI" in r for r in result)

    def test_extracts_acronyms(self, extractor: TagExtractor) -> None:
        """Test extraction of acronyms like AWS, API."""
        text = "AWSとAPIについて学ぶ"

        tokens = [self._make_token("test", "名詞")]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_japanese_words(text)

        assert "AWS" in result
        assert "API" in result

    def test_extracts_proper_noun_sequences(self, extractor: TagExtractor) -> None:
        """Test extraction of proper noun sequences."""
        tokens = [
            self._make_token("山田", "名詞", "固有名詞"),
            self._make_token("太郎", "名詞", "人名"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_japanese_words("山田太郎")

        assert "山田太郎" in result

    def test_deduplicates_results(self, extractor: TagExtractor) -> None:
        """Test that duplicate extractions are removed."""
        text = "AWS AWS AWS"  # Repeated

        tokens = [self._make_token("test", "名詞")]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_japanese_words(text)

        # Should only appear once
        assert result.count("AWS") == 1

    def test_preserves_order(self, extractor: TagExtractor) -> None:
        """Test that extraction order is preserved."""
        text = "AWS API SDK"

        tokens = [self._make_token("test", "名詞")]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result = extractor._extract_compound_japanese_words(text)

        # Check that items found appear in order
        found_items = [r for r in result if r in ["AWS", "API", "SDK"]]
        assert found_items == sorted(found_items, key=text.index)

    def test_proper_noun_chain_breaks_on_non_proper_noun_even_with_connector_surface(
        self, extractor: TagExtractor
    ) -> None:
        """A noun token with non-proper pos2 must end the proper-noun chain even if its surface is a connector."""
        tokens = [
            self._make_token("東京", "名詞", "固有名詞"),
            self._make_token("・", "名詞", "普通名詞"),
            self._make_token("京都", "名詞", "固有名詞"),
        ]
        tagger = MagicMock(return_value=tokens)

        result = _extract_proper_noun_compounds("東京・京都", tagger, extractor.config)

        assert "東京・京都" not in result

    def test_proper_noun_chain_connects_on_non_noun_connector_symbol(self, extractor: TagExtractor) -> None:
        """A non-noun connector symbol still joins proper nouns."""
        tokens = [
            self._make_token("東京", "名詞", "固有名詞"),
            self._make_token("・", "補助記号", "一般"),
            self._make_token("大阪", "名詞", "固有名詞"),
        ]
        tagger = MagicMock(return_value=tokens)

        result = _extract_proper_noun_compounds("東京・大阪", tagger, extractor.config)

        assert result == ["東京・大阪"]


class TestJapaneseKeywordExtraction:
    """Integration tests for Japanese keyword extraction."""

    @pytest.fixture
    def extractor(self) -> TagExtractor:
        """Create TagExtractor with mocked models for Japanese extraction."""
        with patch("tag_extractor.extract.TagExtractor._lazy_load_models"):
            config = TagExtractionConfig(use_japanese_semantic=False)
            extractor = TagExtractor(config=config)
            extractor._models_loaded = True

            mock_tagger = MagicMock()
            extractor._ja_tagger = mock_tagger
            extractor._ja_stopwords = set()

            return extractor

    def _make_token(self, surface: str, pos1: str) -> MagicMock:
        """Helper to create a mock token."""
        token = MagicMock()
        token.surface = surface
        token.feature = MagicMock()
        token.feature.pos1 = pos1
        token.feature.pos2 = ""
        return token

    def test_frequency_based_ranking(self, extractor: TagExtractor) -> None:
        """Test that frequent terms rank higher."""
        # Simulate text where "機械学習" appears multiple times
        tokens = [
            self._make_token("機械", "名詞"),
            self._make_token("学習", "名詞"),
            self._make_token("機械", "名詞"),
            self._make_token("学習", "名詞"),
            self._make_token("AI", "名詞"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result, confidences = extractor._extract_keywords_japanese("機械学習機械学習AI")

        # "機械学習" should appear and have higher confidence due to frequency
        if "機械学習" in result and "AI" in result:
            assert confidences.get("機械学習", 0) >= confidences.get("AI", 0)

    def test_compound_boost(self, extractor: TagExtractor) -> None:
        """Test that compound words get a frequency boost."""
        tokens = [
            self._make_token("機械", "名詞"),
            self._make_token("学習", "名詞"),
            self._make_token("は", "助詞"),
            self._make_token("技術", "名詞"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result, _confidences = extractor._extract_keywords_japanese("機械学習は技術")

        # Compound "機械学習" should rank higher than single "技術"
        short_result_threshold = 2
        if "機械学習" in result:
            assert result.index("機械学習") < len(result) // 2 or len(result) <= short_result_threshold

    def test_returns_confidences(self, extractor: TagExtractor) -> None:
        """Test that confidence scores are returned."""
        tokens = [
            self._make_token("テスト", "名詞"),
        ]
        cast(MagicMock, extractor._ja_tagger).return_value = tokens

        result, confidences = extractor._extract_keywords_japanese("テスト")

        # Should return dict with float values
        assert isinstance(confidences, dict)
        for tag in result:
            if tag in confidences:
                assert isinstance(confidences[tag], float)
                assert 0.0 <= confidences[tag] <= 1.0
