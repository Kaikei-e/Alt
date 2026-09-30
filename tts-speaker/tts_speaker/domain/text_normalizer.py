import re
import unicodedata

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


def normalize_for_tts(text: str) -> str:
    """Normalize text for TTS: NFKC, collapse spaces/tabs, keep newlines, expand acronyms and single letters."""
    if not text:
        return ""

    normalized = unicodedata.normalize("NFKC", text)
    collapsed = re.sub(r"[ \t]+", " ", normalized)

    def _replace_alpha(match: re.Match[str]) -> str:
        word = match.group(0)
        if len(word) == 1:
            return LETTER_MAP.get(word.upper(), word)
        if 2 <= len(word) <= 6 and word.isupper():
            return "".join(LETTER_MAP.get(c, c) for c in word)
        return word

    return re.sub(r"[A-Za-z]+", _replace_alpha, collapsed)
