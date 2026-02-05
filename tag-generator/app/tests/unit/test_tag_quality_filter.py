"""
Unit tests for tag quality filtering.

Tests the _is_valid_japanese_tag method that filters out:
- Tags that are too long (>20 chars) or too short (<2 chars)
- Tags ending with verbs/auxiliary verbs (sentence fragments)
- Tags ending with particles (incomplete phrases)
- Tags that are numbers only
- URL/HTML fragments
"""

from unittest.mock import patch

import pytest


class TestIsValidJapaneseTag:
    """Tests for _is_valid_japanese_tag method in TagExtractor."""

    @pytest.fixture
    def extractor(self):
        """Create TagExtractor for testing."""
        with patch("tag_extractor.extract.TagExtractor._lazy_load_models"):
            from tag_extractor.extract import TagExtractor

            extractor = TagExtractor()
            extractor._models_loaded = True
            return extractor

    # Length validation tests
    def test_rejects_empty_tag(self, extractor):
        """Empty tags should be rejected."""
        assert extractor._is_valid_japanese_tag("") is False

    def test_rejects_single_char_tag(self, extractor):
        """Single character tags should be rejected."""
        assert extractor._is_valid_japanese_tag("A") is False
        assert extractor._is_valid_japanese_tag("あ") is False

    def test_accepts_two_char_tag(self, extractor):
        """Two character tags should be accepted."""
        assert extractor._is_valid_japanese_tag("AI") is True
        assert extractor._is_valid_japanese_tag("機械") is True

    def test_accepts_normal_length_tag(self, extractor):
        """Normal length tags (2-20 chars) should be accepted."""
        assert extractor._is_valid_japanese_tag("機械学習") is True
        assert extractor._is_valid_japanese_tag("TensorFlow") is True
        assert extractor._is_valid_japanese_tag("ディープラーニング") is True

    def test_accepts_exactly_15_char_tag(self, extractor):
        """Tags exactly 15 characters should be accepted (updated from 20)."""
        tag_15_chars = "あ" * 15
        assert extractor._is_valid_japanese_tag(tag_15_chars) is True

    def test_rejects_tag_over_15_chars(self, extractor):
        """Tags over 15 characters should be rejected (updated from 20)."""
        tag_16_chars = "あ" * 16
        assert extractor._is_valid_japanese_tag(tag_16_chars) is False

    def test_rejects_long_sentence_fragment(self, extractor):
        """Long sentence-like fragments should be rejected."""
        long_fragment = "vscodeで使用できるtailwindの公式intellisenseや"
        assert extractor._is_valid_japanese_tag(long_fragment) is False

    # Verb ending tests
    def test_rejects_desu_ending(self, extractor):
        """Tags ending with です should be rejected."""
        assert extractor._is_valid_japanese_tag("便利です") is False
        assert extractor._is_valid_japanese_tag("機械学習です") is False

    def test_rejects_masu_ending(self, extractor):
        """Tags ending with ます should be rejected."""
        assert extractor._is_valid_japanese_tag("使います") is False
        assert extractor._is_valid_japanese_tag("動作します") is False

    def test_rejects_mashita_ending(self, extractor):
        """Tags ending with ました should be rejected."""
        assert extractor._is_valid_japanese_tag("完了しました") is False
        assert extractor._is_valid_japanese_tag("実装しました") is False

    def test_rejects_teiru_ending(self, extractor):
        """Tags ending with ている should be rejected."""
        assert extractor._is_valid_japanese_tag("動いている") is False
        assert extractor._is_valid_japanese_tag("使用している") is False

    def test_rejects_shita_ending(self, extractor):
        """Tags ending with した should be rejected."""
        assert extractor._is_valid_japanese_tag("実装した") is False
        assert extractor._is_valid_japanese_tag("開発した") is False

    def test_rejects_suru_ending(self, extractor):
        """Tags ending with する should be rejected."""
        assert extractor._is_valid_japanese_tag("実行する") is False
        assert extractor._is_valid_japanese_tag("処理する") is False

    def test_rejects_nai_ending(self, extractor):
        """Tags ending with ない should be rejected."""
        assert extractor._is_valid_japanese_tag("できない") is False
        assert extractor._is_valid_japanese_tag("動作しない") is False

    def test_rejects_aru_ending(self, extractor):
        """Tags ending with ある should be rejected."""
        assert extractor._is_valid_japanese_tag("必要がある") is False

    def test_rejects_iru_ending(self, extractor):
        """Tags ending with いる should be rejected."""
        assert extractor._is_valid_japanese_tag("使っている") is False

    def test_rejects_reru_ending(self, extractor):
        """Tags ending with れる should be rejected."""
        assert extractor._is_valid_japanese_tag("呼ばれる") is False
        assert extractor._is_valid_japanese_tag("使用される") is False

    def test_rejects_rareru_ending(self, extractor):
        """Tags ending with られる should be rejected."""
        assert extractor._is_valid_japanese_tag("考えられる") is False

    # Particle ending tests (short tags only)
    def test_rejects_short_tag_with_particle_ha(self, extractor):
        """Short tags ending with は should be rejected."""
        assert extractor._is_valid_japanese_tag("これは") is False

    def test_rejects_short_tag_with_particle_ga(self, extractor):
        """Short tags ending with が should be rejected."""
        assert extractor._is_valid_japanese_tag("これが") is False

    def test_rejects_short_tag_with_particle_wo(self, extractor):
        """Short tags ending with を should be rejected."""
        assert extractor._is_valid_japanese_tag("これを") is False

    def test_rejects_short_tag_with_particle_ni(self, extractor):
        """Short tags ending with に should be rejected."""
        assert extractor._is_valid_japanese_tag("ここに") is False

    def test_rejects_short_tag_with_particle_de(self, extractor):
        """Short tags ending with で should be rejected."""
        assert extractor._is_valid_japanese_tag("ここで") is False

    def test_rejects_tag_with_particle_ending(self, extractor):
        """Tags ending with particles should now be rejected regardless of length.

        This is a behavior change from the original - particle endings are now
        filtered for ALL lengths to prevent sentence fragments like
        "Databricksのセキュリティは" from becoming tags.
        """
        # Particle endings are rejected for all lengths
        assert extractor._is_valid_japanese_tag("ディープラーニングは") is False
        assert extractor._is_valid_japanese_tag("機械学習手法は") is False
        # But valid nouns without particles are accepted
        assert extractor._is_valid_japanese_tag("ディープラーニング") is True
        assert extractor._is_valid_japanese_tag("機械学習手法") is True

    # Number-only tests
    def test_rejects_number_only(self, extractor):
        """Tags that are numbers only should be rejected."""
        assert extractor._is_valid_japanese_tag("2025") is False
        assert extractor._is_valid_japanese_tag("12") is False
        assert extractor._is_valid_japanese_tag("100") is False
        assert extractor._is_valid_japanese_tag("0") is False

    def test_accepts_number_with_text(self, extractor):
        """Tags with numbers and text should be accepted."""
        assert extractor._is_valid_japanese_tag("Web3") is True
        assert extractor._is_valid_japanese_tag("5G通信") is True
        assert extractor._is_valid_japanese_tag("3Dプリンター") is True
        assert extractor._is_valid_japanese_tag("iOS17") is True

    # URL/HTML fragment tests
    def test_rejects_https_fragment(self, extractor):
        """Tags that are 'https' should be rejected."""
        assert extractor._is_valid_japanese_tag("https") is False
        assert extractor._is_valid_japanese_tag("HTTPS") is False
        assert extractor._is_valid_japanese_tag("http") is False

    def test_rejects_www_fragment(self, extractor):
        """Tags that are 'www' should be rejected."""
        assert extractor._is_valid_japanese_tag("www") is False
        assert extractor._is_valid_japanese_tag("WWW") is False

    def test_rejects_domain_fragment(self, extractor):
        """Tags that are domain TLDs should be rejected."""
        assert extractor._is_valid_japanese_tag("com") is False
        assert extractor._is_valid_japanese_tag("org") is False
        assert extractor._is_valid_japanese_tag("net") is False
        assert extractor._is_valid_japanese_tag("html") is False

    def test_rejects_html_entity_fragments(self, extractor):
        """Tags that are HTML entity fragments should be rejected."""
        assert extractor._is_valid_japanese_tag("gt") is False
        assert extractor._is_valid_japanese_tag("lt") is False
        assert extractor._is_valid_japanese_tag("amp") is False
        assert extractor._is_valid_japanese_tag("nbsp") is False

    # Valid tag tests (positive cases)
    def test_accepts_valid_tech_terms(self, extractor):
        """Valid tech terms should be accepted."""
        valid_tags = [
            "機械学習",
            "ディープラーニング",
            "TensorFlow",
            "GitHub",
            "AWS",
            "API",
            "Python",
            "JavaScript",
            "クラウド",
            "セキュリティ",
        ]
        for tag in valid_tags:
            assert extractor._is_valid_japanese_tag(tag) is True, f"Tag '{tag}' should be valid"

    def test_accepts_valid_japanese_nouns(self, extractor):
        """Valid Japanese nouns should be accepted."""
        valid_tags = [
            "技術",
            "開発",
            "プログラミング",
            "データベース",
            "アルゴリズム",
            "ネットワーク",
            "サーバー",
            "アプリケーション",
        ]
        for tag in valid_tags:
            assert extractor._is_valid_japanese_tag(tag) is True, f"Tag '{tag}' should be valid"


class TestRegexPhaseMaxLengthFilter:
    """Tests for regex phase length filtering in _extract_compound_japanese_words."""

    @pytest.fixture
    def extractor(self):
        """Create TagExtractor for testing."""
        with patch("tag_extractor.extract.TagExtractor._lazy_load_models"):
            from tag_extractor.extract import TagExtractor

            extractor = TagExtractor()
            extractor._models_loaded = True
            # Mock tagger to return empty to isolate regex phase
            from unittest.mock import MagicMock

            extractor._ja_tagger = MagicMock(return_value=[])
            return extractor

    def test_regex_filters_long_matches(self, extractor):
        """Regex extraction should filter matches over MAX_TAG_LENGTH."""
        # Create text with a very long camelCase that would match regex
        long_term = "A" + "a" * 25  # 26 chars total
        text = f"この{long_term}は長すぎる"

        result = extractor._extract_compound_japanese_words(text)

        # Long matches should be filtered out
        assert long_term not in result


class TestProperNounMaxLengthFilter:
    """Tests for proper noun extraction max length filtering."""

    @pytest.fixture
    def extractor(self):
        """Create TagExtractor for testing."""
        with patch("tag_extractor.extract.TagExtractor._lazy_load_models"):
            from tag_extractor.extract import TagExtractor

            extractor = TagExtractor()
            extractor._models_loaded = True
            return extractor

    def _make_token(self, surface: str, pos1: str, pos2: str = ""):
        """Create a mock token."""
        from unittest.mock import MagicMock

        token = MagicMock()
        token.surface = surface
        token.feature = MagicMock()
        token.feature.pos1 = pos1
        token.feature.pos2 = pos2
        return token

    def test_proper_noun_filters_long_sequences(self, extractor):
        """Proper noun extraction should filter sequences over MAX_TAG_LENGTH."""
        from unittest.mock import MagicMock

        # Create tokens that would form a very long proper noun sequence
        long_surface = "あ" * 25
        tokens = [
            self._make_token(long_surface, "名詞", "固有名詞"),
        ]
        extractor._ja_tagger = MagicMock(return_value=tokens)

        result = extractor._extract_compound_japanese_words("test")

        # Long proper noun should be filtered
        assert long_surface not in result


class TestCandidateFilteringIntegration:
    """Tests for candidate filtering at line 597 in _extract_keywords_japanese."""

    @pytest.fixture
    def extractor(self):
        """Create TagExtractor for testing."""
        with patch("tag_extractor.extract.TagExtractor._lazy_load_models"):
            from tag_extractor.extract import TagExtractionConfig, TagExtractor

            config = TagExtractionConfig(use_japanese_semantic=False)
            extractor = TagExtractor(config=config)
            extractor._models_loaded = True
            extractor._ja_stopwords = set()
            return extractor

    def _make_token(self, surface: str, pos1: str, pos2: str = ""):
        """Create a mock token."""
        from unittest.mock import MagicMock

        token = MagicMock()
        token.surface = surface
        token.feature = MagicMock()
        token.feature.pos1 = pos1
        token.feature.pos2 = pos2
        return token

    def test_filters_verb_endings_from_candidates(self, extractor):
        """Verb-ending candidates should be filtered from final results."""
        from unittest.mock import MagicMock

        # Create tokens that produce a verb-ending compound
        tokens = [
            self._make_token("実装", "名詞"),
            self._make_token("しました", "助動詞"),
        ]
        extractor._ja_tagger = MagicMock(return_value=tokens)

        keywords, _ = extractor._extract_keywords_japanese("実装しました")

        # Verb endings should be filtered out
        assert not any("しました" in kw for kw in keywords)

    def test_filters_number_only_from_candidates(self, extractor):
        """Number-only candidates should be filtered from final results."""
        from unittest.mock import MagicMock

        tokens = [
            self._make_token("2025", "名詞-数詞"),
            self._make_token("年", "名詞"),
        ]
        extractor._ja_tagger = MagicMock(return_value=tokens)

        keywords, _ = extractor._extract_keywords_japanese("2025年")

        # Number-only should be filtered
        assert "2025" not in keywords
