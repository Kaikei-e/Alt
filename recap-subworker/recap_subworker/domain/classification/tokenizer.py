"""言語別のトークナイズと正規化処理。"""

import re
import unicodedata
from abc import ABC, abstractmethod
from enum import Enum
from typing import List

try:
    from janome.tokenizer import Tokenizer as JanomeTokenizer
except ImportError:
    JanomeTokenizer = None


class ClassificationLanguage(Enum):
    """分類対象テキストの言語。"""

    JAPANESE = "japanese"
    ENGLISH = "english"
    UNKNOWN = "unknown"

    @classmethod
    def from_code(cls, code: str) -> "ClassificationLanguage":
        """言語コードからClassificationLanguageを取得。"""
        code_lower = code.lower()
        if code_lower in ("ja", "jp"):
            return cls.JAPANESE
        if code_lower in ("en", "us", "uk"):
            return cls.ENGLISH
        return cls.UNKNOWN


class NormalizedDocument:
    """正規化された文書。"""

    def __init__(self, tokens: List[str], normalized: str):
        self.tokens = tokens
        self.normalized = normalized


def normalize_text(input_text: str) -> str:
    """テキストをNFC正規化。"""
    return unicodedata.normalize("NFC", input_text)


class Tokenizer(ABC):
    """トークナイザーの基底クラス。"""

    @abstractmethod
    def tokenize(self, text: str) -> List[str]:
        """テキストをトークン化。"""
        pass


class JapaneseTokenizer(Tokenizer):
    """日本語トークナイザー（Janome使用）。"""

    def __init__(self):
        self.tokenizer = JanomeTokenizer() if JanomeTokenizer else None
        # Pythonのreモジュールでは\p{L}などが使えないため、代替パターンを使用
        self.fallback_word_re = re.compile(r"[^\w\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]+")

    def tokenize(self, text: str) -> List[str]:
        """日本語テキストをトークン化。"""
        if self.tokenizer:
            try:
                tokens = self.tokenizer.tokenize(text)
                results = [token.surface for token in tokens if token.surface.strip()]
                if results:
                    return results
            except Exception:
                pass
        return self._fallback_tokenize(text)

    def _fallback_tokenize(self, text: str) -> List[str]:
        """フォールバックトークン化。"""
        normalized = normalize_text(text)
        tokens = []
        for piece in normalized.split():
            for token in self.fallback_word_re.split(piece):
                if token:
                    tokens.append(token)
        return tokens


class EnglishTokenizer(Tokenizer):
    """英語トークナイザー。"""

    @staticmethod
    def tokenize(text: str) -> List[str]:
        """英語テキストをトークン化。"""
        normalized = normalize_text(text)
        # 単語境界で分割
        tokens = []
        current_word = []
        for char in normalized:
            if char.isalnum():
                current_word.append(char)
            else:
                if current_word:
                    tokens.append("".join(current_word))
                    current_word = []
        if current_word:
            tokens.append("".join(current_word))

        # 正規化
        return [EnglishTokenizer._normalize_token(token) for token in tokens if token]

    @staticmethod
    def _normalize_token(token: str) -> str:
        """英語トークンを正規化。"""
        lower = token.lower()
        if lower.endswith("ies") and len(lower) > 3:
            return lower[:-3] + "y"
        if lower.endswith("ing") and len(lower) > 4:
            return lower[:-3]
        if lower.endswith("s") and len(lower) > 3:
            return lower[:-1]
        return lower


class FallbackTokenizer(Tokenizer):
    """フォールバックトークナイザー。"""

    def __init__(self):
        # Pythonのreモジュールでは\p{L}などが使えないため、代替パターンを使用
        self.split_re = re.compile(r"[^\w\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]+")

    def tokenize(self, text: str) -> List[str]:
        """フォールバックトークン化。"""
        normalized = normalize_text(text)
        tokens = []
        for piece in normalized.split():
            for token in self.split_re.split(piece):
                if token:
                    tokens.append(token.lower())
        return tokens


def apply_augmented_tokens(tokens: List[str], mapping: List[tuple[str, List[str]]]) -> None:
    """同義語拡張を適用。"""
    extras = []
    for needle, synonyms in mapping:
        if any(token == needle or needle in token for token in tokens):
            extras.extend(syn.lower() for syn in synonyms)
    tokens.extend(extras)


class TokenPipeline:
    """トークンパイプライン。"""

    def __init__(self):
        self.japanese = JapaneseTokenizer()
        self.english = EnglishTokenizer()
        self.fallback = FallbackTokenizer()

    def resolve_language(
        self, provided: ClassificationLanguage, text: str
    ) -> ClassificationLanguage:
        """言語を解決。"""
        if provided != ClassificationLanguage.UNKNOWN:
            return provided
        # 簡易的な言語検出（日本語文字が含まれていれば日本語）
        if any("\u3040" <= char <= "\u309F" or "\u30A0" <= char <= "\u30FF" for char in text):
            return ClassificationLanguage.JAPANESE
        # 英語の簡易検出
        if any(char.isascii() and char.isalpha() for char in text):
            return ClassificationLanguage.ENGLISH
        return ClassificationLanguage.UNKNOWN

    def tokenize(self, text: str, lang: ClassificationLanguage) -> List[str]:
        """テキストをトークン化。"""
        if lang == ClassificationLanguage.JAPANESE:
            return self.japanese.tokenize(text)
        if lang == ClassificationLanguage.ENGLISH:
            return EnglishTokenizer.tokenize(text)
        return self.fallback.tokenize(text)

    def preprocess(
        self, title: str, body: str, lang: ClassificationLanguage
    ) -> NormalizedDocument:
        """前処理（トークン化 + 同義語拡張）。"""
        combined = f"{title} {body}"
        resolved = self.resolve_language(lang, combined)
        tokens = self.tokenize(combined, resolved)
        self._augment_tokens(tokens, resolved)
        normalized = " ".join(tokens)
        return NormalizedDocument(tokens=tokens, normalized=normalized)

    def _augment_tokens(self, tokens: List[str], lang: ClassificationLanguage) -> None:
        """同義語拡張を適用。"""
        if lang == ClassificationLanguage.JAPANESE:
            apply_augmented_tokens(
                tokens,
                [
                    ("資本提携", ["資金調達", "投資"]),
                    ("政調会長", ["政策", "政府"]),
                    ("ゲノム", ["遺伝子", "医療"]),
                    ("干渉計", ["量子", "研究"]),
                    ("劇伴", ["音楽", "エンタメ"]),
                    ("自律走行", ["自動運転", "人工知能"]),
                ],
            )
        elif lang == ClassificationLanguage.ENGLISH:
            apply_augmented_tokens(
                tokens,
                [
                    ("confidential", ["confidential computing", "cloud", "cybersecurity"]),
                    ("attestation", ["cybersecurity", "cloud"]),
                    ("ceasefire", ["diplomacy", "treaty"]),
                    ("reconstruction", ["economy", "business"]),
                    ("multimodal", ["transformer", "machine learning"]),
                ],
            )

