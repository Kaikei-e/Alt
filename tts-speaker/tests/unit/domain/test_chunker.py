"""Unit tests for chunker."""

import re

from tts_speaker.domain.chunker import split_into_chunks


def test_empty_text() -> None:
    assert split_into_chunks("", max_chars=100) == []
    assert split_into_chunks("   \n\t  ", max_chars=100) == []


def test_short_sentences_packed_greedily() -> None:
    text = "吾輩は猫である。名前はまだ無い。どこで生れたかとんと見当がつかぬ。"
    # Each sentence is short (< 30 chars). With max_chars=50, consecutive sentences are packed.
    chunks = split_into_chunks(text, max_chars=50)
    assert len(chunks) >= 1
    for chunk in chunks:
        assert len(chunk) <= 50
        assert len(chunk) > 0


def test_sentence_split_punctuation() -> None:
    text = "こんにちは！元気ですか？はい、元気です。そうですか！"
    chunks = split_into_chunks(text, max_chars=20)
    for c in chunks:
        assert len(c) <= 20
        assert len(c) > 0


def test_join_ascii_alphanumeric_with_space() -> None:
    # When both neighbors are ASCII alphanumeric, join with single space
    text = "Part A!\nPart B!"
    # With max_chars=50, both should pack into one chunk
    chunks = split_into_chunks(text, max_chars=50)
    assert len(chunks) == 1
    # Check that punctuation is kept, newlines dropped
    assert "Part A!" in chunks[0]
    assert "Part B!" in chunks[0]


def test_long_sentence_split_by_comma() -> None:
    # A single long sentence without sentence-ending punctuation, but with commas
    clause1 = "これは非常に長い文章の前半部分であって、"
    clause2 = "読点によって適切に分割されるべき後半部分です。"
    long_sentence = clause1 + clause2
    chunks = split_into_chunks(long_sentence, max_chars=len(clause1) + 2)
    assert len(chunks) == 2
    assert chunks[0] == clause1
    assert chunks[1] == clause2


def test_hard_split_without_punctuation() -> None:
    # Text longer than max_chars with no punctuation
    text = "あ" * 150
    chunks = split_into_chunks(text, max_chars=50)
    assert len(chunks) == 3
    assert chunks[0] == "あ" * 50
    assert chunks[1] == "あ" * 50
    assert chunks[2] == "あ" * 50


def test_invariants_1000_char_no_punctuation() -> None:
    text = "あ" * 1000
    max_chars = 100
    chunks = split_into_chunks(text, max_chars=max_chars)
    assert len(chunks) == 10
    for chunk in chunks:
        assert len(chunk) == 100
    assert "".join(chunks) == text


def test_invariants_realistic_text() -> None:
    text = """
    人工知能の発展により、音声合成技術は飛躍的に進化しました。
    特に、高品質なニューラルネットワークモデルの登場によって、人間と聞き分けがつかないレベルの自然な音声が生成できるようになっています！
    しかし、長大な文章を一度に合成しようとすると、メモリ消費やレイテンシの増大といった課題が生じます。
    そのため、適切なチャンク分割が不可欠です。
    """
    max_chars = 60
    chunks = split_into_chunks(text, max_chars=max_chars)

    # Invariant 1: every chunk non-empty
    for c in chunks:
        assert len(c) > 0

    # Invariant 2: every chunk len <= max_chars
    for c in chunks:
        assert len(c) <= max_chars

    # Invariant 3 & 4: order preserved and content preserved without whitespace
    cleaned_original = re.sub(r"\s+", "", text)
    cleaned_chunks = re.sub(r"\s+", "", "".join(chunks))
    assert cleaned_chunks == cleaned_original
