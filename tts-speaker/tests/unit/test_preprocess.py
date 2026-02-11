"""Tests for TTS text preprocessing (English -> Katakana conversion)."""

from __future__ import annotations

from unittest.mock import patch

import pytest

from tts_speaker.core.preprocess import (
    LETTER_MAP,
    expand_acronyms,
    english_to_katakana,
    preprocess_for_tts,
)


class TestExpandAcronyms:
    """Tests for expand_acronyms()."""

    def test_single_acronym(self):
        assert expand_acronyms("RSS") == "アールエスエス"

    def test_api_acronym(self):
        assert expand_acronyms("API") == "エーピーアイ"

    def test_multiple_acronyms(self):
        result = expand_acronyms("RSSフィードのAPI")
        assert result == "アールエスエスフィードのエーピーアイ"

    def test_japanese_only_unchanged(self):
        text = "日本語のテキストです"
        assert expand_acronyms(text) == text

    def test_mixed_case_excluded(self):
        """Mixed case words like 'Machine' should not be treated as acronyms."""
        assert expand_acronyms("Machine") == "Machine"

    def test_single_letter_excluded(self):
        """Single letters are not acronyms."""
        assert expand_acronyms("I am A") == "I am A"

    def test_two_letter_acronym(self):
        assert expand_acronyms("AI") == "エーアイ"

    def test_six_letter_acronym(self):
        assert expand_acronyms("ABCDEF") == "エービーシーディーイーエフ"

    def test_seven_letter_excluded(self):
        """Words longer than 6 uppercase letters are not treated as acronyms."""
        assert expand_acronyms("ABCDEFG") == "ABCDEFG"

    def test_empty_string(self):
        assert expand_acronyms("") == ""

    def test_acronym_with_surrounding_japanese(self):
        result = expand_acronyms("最新のAI技術")
        assert result == "最新のエーアイ技術"


class TestEnglishToKatakana:
    """Tests for english_to_katakana(). Uses mock for alkana."""

    @patch("tts_speaker.core.preprocess.alkana")
    def test_known_word(self, mock_alkana):
        mock_alkana.get_kana.return_value = "マシン"
        assert english_to_katakana("Machine") == "マシン"
        mock_alkana.get_kana.assert_called_once_with("machine")

    @patch("tts_speaker.core.preprocess.alkana")
    def test_unknown_word_kept(self, mock_alkana):
        mock_alkana.get_kana.return_value = None
        assert english_to_katakana("Xylophone") == "Xylophone"

    @patch("tts_speaker.core.preprocess.alkana")
    def test_multiple_words(self, mock_alkana):
        def fake_kana(word):
            return {"machine": "マシン", "learning": "ラーニング"}.get(word)

        mock_alkana.get_kana.side_effect = fake_kana
        result = english_to_katakana("Machine Learningの最新トレンド")
        assert result == "マシン ラーニングの最新トレンド"

    @patch("tts_speaker.core.preprocess.alkana")
    def test_single_letter_excluded(self, mock_alkana):
        """Single character words should not be passed to alkana."""
        mock_alkana.get_kana.return_value = "テスト"
        result = english_to_katakana("I am test")
        # "I" is single char, should not be converted
        # "am" and "test" should be processed
        assert "I" in result
        # "am" was passed to alkana
        mock_alkana.get_kana.assert_any_call("am")
        mock_alkana.get_kana.assert_any_call("test")

    @patch("tts_speaker.core.preprocess.alkana")
    def test_japanese_only_unchanged(self, mock_alkana):
        text = "日本語のみのテキスト"
        assert english_to_katakana(text) == text
        mock_alkana.get_kana.assert_not_called()

    @patch("tts_speaker.core.preprocess.alkana")
    def test_empty_string(self, mock_alkana):
        assert english_to_katakana("") == ""
        mock_alkana.get_kana.assert_not_called()


class TestPreprocessForTts:
    """Tests for preprocess_for_tts() integration pipeline."""

    @patch("tts_speaker.core.preprocess.alkana")
    def test_acronym_and_english_mixed(self, mock_alkana):
        """Acronyms are expanded first, then remaining English words are converted."""

        def fake_kana(word):
            return {"trending": "トレンディング"}.get(word)

        mock_alkana.get_kana.side_effect = fake_kana
        result = preprocess_for_tts("APIがtrending")
        assert result == "エーピーアイがトレンディング"

    @patch("tts_speaker.core.preprocess.alkana")
    def test_isolated_single_letter_expanded(self, mock_alkana):
        """Isolated single letters are expanded via LETTER_MAP."""
        mock_alkana.get_kana.return_value = None
        result = preprocess_for_tts("カテゴリ A のニュース")
        assert result == "カテゴリ エー のニュース"

    @patch("tts_speaker.core.preprocess.alkana")
    def test_pure_japanese_unchanged(self, mock_alkana):
        text = "今日のニュースをお伝えします。"
        assert preprocess_for_tts(text) == text
        mock_alkana.get_kana.assert_not_called()

    @patch("tts_speaker.core.preprocess.alkana")
    def test_realistic_news_sentence(self, mock_alkana):
        """Realistic mixed-language news text."""

        def fake_kana(word):
            return {
                "machine": "マシン",
                "learning": "ラーニング",
            }.get(word)

        mock_alkana.get_kana.side_effect = fake_kana
        result = preprocess_for_tts("Machine LearningのAPIを活用したRSSリーダー")
        assert result == "マシン ラーニングのエーピーアイを活用したアールエスエスリーダー"

    @patch("tts_speaker.core.preprocess.alkana")
    def test_empty_string(self, mock_alkana):
        assert preprocess_for_tts("") == ""

    @patch("tts_speaker.core.preprocess.alkana")
    def test_acronym_not_passed_to_alkana(self, mock_alkana):
        """Acronyms should be expanded before alkana is called, so alkana never sees them."""
        mock_alkana.get_kana.return_value = None
        preprocess_for_tts("API")
        # alkana should not be called because "API" was already expanded to katakana
        mock_alkana.get_kana.assert_not_called()

    @patch("tts_speaker.core.preprocess.alkana")
    def test_letter_map_completeness(self, mock_alkana):
        """All 26 letters should be in LETTER_MAP."""
        import string

        for letter in string.ascii_uppercase:
            assert letter in LETTER_MAP, f"Missing letter: {letter}"

    @patch("tts_speaker.core.preprocess.alkana")
    def test_single_letter_in_word_not_expanded(self, mock_alkana):
        """Letters within words should NOT be expanded."""
        mock_alkana.get_kana.return_value = None
        result = preprocess_for_tts("Apple")
        # "Apple" should stay as-is (alkana returned None), not have individual letters expanded
        assert result == "Apple"
