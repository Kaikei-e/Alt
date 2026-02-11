"""TTS text preprocessing: English/acronym -> Katakana conversion.

Kokoro-82M's Japanese G2P silently skips English words not in unidic.
This module converts English words and acronyms to Katakana before TTS synthesis.
"""

from __future__ import annotations

import re

import alkana

LETTER_MAP: dict[str, str] = {
    "A": "エー",
    "B": "ビー",
    "C": "シー",
    "D": "ディー",
    "E": "イー",
    "F": "エフ",
    "G": "ジー",
    "H": "エイチ",
    "I": "アイ",
    "J": "ジェー",
    "K": "ケー",
    "L": "エル",
    "M": "エム",
    "N": "エヌ",
    "O": "オー",
    "P": "ピー",
    "Q": "キュー",
    "R": "アール",
    "S": "エス",
    "T": "ティー",
    "U": "ユー",
    "V": "ブイ",
    "W": "ダブリュー",
    "X": "エックス",
    "Y": "ワイ",
    "Z": "ゼット",
}

_ACRONYM_RE = re.compile(r"(?<![A-Za-z])[A-Z]{2,6}(?![A-Za-z])")
_ENGLISH_WORD_RE = re.compile(r"[A-Za-z]{2,}")
_ISOLATED_LETTER_RE = re.compile(r"(?<![A-Za-z])[A-Za-z](?![A-Za-z])")


def expand_acronyms(text: str) -> str:
    """Expand uppercase acronyms (2-6 chars) to Katakana letter names.

    Example: "RSS" -> "アールエスエス", "API" -> "エーピーアイ"
    """

    def _replace(m: re.Match) -> str:
        return "".join(LETTER_MAP[ch] for ch in m.group())

    return _ACRONYM_RE.sub(_replace, text)


def english_to_katakana(text: str) -> str:
    """Convert English words (2+ chars) to Katakana using alkana dictionary.

    Words not found in the dictionary are left unchanged.
    """

    def _replace(m: re.Match) -> str:
        word = m.group()
        kana = alkana.get_kana(word.lower())
        return kana if kana is not None else word

    return _ENGLISH_WORD_RE.sub(_replace, text)


def preprocess_for_tts(text: str) -> str:
    """Full preprocessing pipeline for TTS input text.

    Processing order:
    1. Expand acronyms (e.g. API -> エーピーアイ)
    2. Convert remaining English words to Katakana via alkana
    3. Expand isolated single letters (e.g. "A" -> "エー")
    """
    text = expand_acronyms(text)
    text = english_to_katakana(text)

    def _replace_letter(m: re.Match) -> str:
        return LETTER_MAP.get(m.group().upper(), m.group())

    text = _ISOLATED_LETTER_RE.sub(_replace_letter, text)
    return text
