"""Unit tests for text normalizer."""

import string

from tts_speaker.domain.text_normalizer import LETTER_MAP, normalize_for_tts


def test_letter_map_completeness() -> None:
    for letter in string.ascii_uppercase:
        assert letter in LETTER_MAP
        assert len(LETTER_MAP[letter]) > 0


def test_empty_string() -> None:
    assert normalize_for_tts("") == ""


def test_japanese_only_unchanged() -> None:
    text = "日本語のテキストです。句読点もあります。"
    assert normalize_for_tts(text) == text


def test_nfkc_normalization() -> None:
    text = "ＡＰＩ　１２３"
    result = normalize_for_tts(text)
    assert "エーピーアイ" in result
    assert "123" in result


def test_collapse_spaces_and_tabs_but_keep_newlines() -> None:
    text = "Python   \t  feed\n\nNext\t  Line"
    result = normalize_for_tts(text)
    assert "   " not in result
    assert "\t" not in result
    assert "Python feed" in result
    assert "\n" in result


def test_expand_acronyms() -> None:
    assert normalize_for_tts("RSS") == "アールエスエス"
    assert normalize_for_tts("API") == "エーピーアイ"
    assert normalize_for_tts("AI") == "エーアイ"
    assert normalize_for_tts("ABCDEF") == "エービーシーディーイーエフ"
    assert normalize_for_tts("最新のAI技術") == "最新のエーアイ技術"
    assert normalize_for_tts("RSSフィードのAPI") == "アールエスエスフィードのエーピーアイ"


def test_acronym_longer_than_six_letters_unchanged() -> None:
    assert normalize_for_tts("ABCDEFG") == "ABCDEFG"


def test_isolated_single_letter_expanded() -> None:
    assert normalize_for_tts("カテゴリ A のニュース") == "カテゴリ エー のニュース"
    assert normalize_for_tts("カテゴリ a のニュース") == "カテゴリ エー のニュース"


def test_ordinary_english_words_stay_unchanged() -> None:
    # Lowercase and mixed-case English words stay unchanged
    assert normalize_for_tts("Python") == "Python"
    assert normalize_for_tts("feed") == "feed"
    assert normalize_for_tts("Machine") == "Machine"
    assert normalize_for_tts("Learning") == "Learning"
    assert normalize_for_tts("Apple") == "Apple"
    assert normalize_for_tts("trending") == "trending"
    assert normalize_for_tts("Machine Learningの最新トレンド") == "Machine Learningの最新トレンド"


def test_acronym_and_ordinary_english_mixed() -> None:
    # Acronyms expand to katakana, but regular English words stay unchanged
    assert normalize_for_tts("APIがtrending") == "エーピーアイがtrending"
    assert (
        normalize_for_tts("Machine LearningのAPIを活用したRSSリーダー")
        == "Machine Learningのエーピーアイを活用したアールエスエスリーダー"
    )


def test_single_letter_in_word_not_expanded() -> None:
    # Letters inside words are not expanded as isolated letters
    assert normalize_for_tts("Apple") == "Apple"
    assert normalize_for_tts("banana") == "banana"
