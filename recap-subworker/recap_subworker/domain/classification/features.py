"""トークン列から特徴量を抽出する。"""

from collections import defaultdict
from typing import Dict, List, Optional

import numpy as np
import xxhash

EMBEDDING_DIM = 6
FALLBACK_BM25_K1 = 1.6
FALLBACK_BM25_B = 0.75
FALLBACK_AVG_DOC_LEN = 320.0

FALLBACK_VOCAB = [
    "人工知能",
    "自動運転",
    "資金調達",
    "投資",
    "決算",
    "政策",
    "政府",
    "遺伝子",
    "医療",
    "量子",
    "サッカー",
    "音楽",
    "confidential computing",
    "cybersecurity",
    "transformer",
    "diplomacy",
    "treaty",
    "economy",
    "business",
]

FALLBACK_IDF = [
    1.6,
    1.5,
    1.4,
    1.3,
    1.2,
    1.3,
    1.2,
    1.5,
    1.4,
    1.5,
    1.3,
    1.3,
    1.2,
    1.2,
    1.5,
    1.4,
    1.4,
    1.2,
    1.2,
]

# Embedding lookup table (Rust実装から移植)
EMBEDDING_LOOKUP: Dict[str, List[float]] = {
    "人工知能": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "自動運転": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "transformer": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "資金調達": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "投資": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "決算": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "economy": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "business": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "政策": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "政府": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "diplomacy": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "treaty": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "遺伝子": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "医療": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "量子": [0.4, 0.1, 0.0, 0.9, 0.0, 0.0],
    "サッカー": [0.0, 0.0, 0.0, 0.0, 1.0, 0.0],
    "音楽": [0.0, 0.0, 0.0, 0.0, 0.0, 1.0],
    "confidential computing": [0.8, 0.3, 0.0, 0.0, 0.0, 0.0],
    "cybersecurity": [0.8, 0.2, 0.0, 0.0, 0.0, 0.0],
}


class FeatureVector:
    """特徴ベクトル。"""

    def __init__(self, tfidf: List[float], bm25: List[float], embedding: List[float]):
        self.tfidf = tfidf
        self.bm25 = bm25
        self.embedding = embedding

    def max_bm25(self) -> Optional[float]:
        """BM25の最大値を取得。"""
        if not self.bm25:
            return None
        return max(self.bm25)


class EmbeddingStats:
    """Embedding統計情報（Z-score正規化用）。"""

    def __init__(self, mean: List[float], std: List[float]):
        self.mean = mean
        self.std = std

    @classmethod
    def empty(cls, dim: int) -> "EmbeddingStats":
        """空の統計情報を作成（正規化なし）。"""
        return cls(mean=[0.0] * dim, std=[1.0] * dim)

    @classmethod
    def from_embeddings(cls, embeddings: List[List[float]]) -> "EmbeddingStats":
        """Embeddingベクトルから統計を計算。"""
        if not embeddings:
            return cls.empty(EMBEDDING_DIM)

        dim = len(embeddings[0])
        n = len(embeddings)

        # 平均を計算
        mean = [0.0] * dim
        for emb in embeddings:
            for i, val in enumerate(emb):
                mean[i] += val
        mean = [m / n for m in mean]

        # 標準偏差を計算
        std = [0.0] * dim
        for emb in embeddings:
            for i, val in enumerate(emb):
                diff = val - mean[i]
                std[i] += diff * diff
        std = [max((s / n) ** 0.5, 1e-6) for s in std]

        return cls(mean=mean, std=std)

    def normalize(self, embedding: List[float]) -> None:
        """EmbeddingベクトルにZ-score正規化を適用（in-place）。"""
        for i, val in enumerate(embedding):
            if i < len(self.mean) and i < len(self.std):
                embedding[i] = (val - self.mean[i]) / self.std[i]


class FeatureExtractor:
    """特徴抽出器。"""

    def __init__(
        self,
        vocab: List[str],
        idf: List[float],
        bm25_k1: float = FALLBACK_BM25_K1,
        bm25_b: float = FALLBACK_BM25_B,
        average_doc_len: float = FALLBACK_AVG_DOC_LEN,
    ):
        self.vocab_index = {word: i for i, word in enumerate(vocab)}
        self.idf = idf
        self.bm25_k1 = bm25_k1
        self.bm25_b = bm25_b
        self.average_doc_len = average_doc_len
        self.embedding_index = EMBEDDING_LOOKUP
        self.embedding_stats = EmbeddingStats.empty(EMBEDDING_DIM)

    @classmethod
    def from_metadata(
        cls,
        vocab: List[str],
        idf: List[float],
        bm25_k1: float = FALLBACK_BM25_K1,
        bm25_b: float = FALLBACK_BM25_B,
        average_doc_len: float = FALLBACK_AVG_DOC_LEN,
    ) -> "FeatureExtractor":
        """メタデータからFeatureExtractorを作成。"""
        return cls(vocab, idf, bm25_k1, bm25_b, average_doc_len)

    def set_embedding_stats(self, stats: EmbeddingStats) -> None:
        """Embedding統計を設定。"""
        self.embedding_stats = stats

    def vocab_len(self) -> int:
        """語彙サイズを取得。"""
        return len(self.idf)

    def extract(self, tokens: List[str]) -> FeatureVector:
        """トークンから特徴ベクトルを抽出。"""
        vocab_len = len(self.idf)
        raw_counts = [0.0] * vocab_len
        total_hits = 0.0
        embedding = [0.0] * EMBEDDING_DIM
        embedding_hits = 0.0

        for token in tokens:
            lowered = token.lower()
            # TF-IDF/BM25用のカウント
            if lowered in self.vocab_index:
                index = self.vocab_index[lowered]
                raw_counts[index] += 1.0
                total_hits += 1.0

            # Embedding計算
            if lowered in self.embedding_index:
                vector = self.embedding_index[lowered]
                for i, value in enumerate(vector):
                    embedding[i] += value
                embedding_hits += 1.0
            else:
                # Fallback: xxhashを使用して決定論的なランダムベクトルを生成
                # Rust実装のxxh3_64と互換性を保つため、xxhash.xxh3_64_intdigestを使用
                hash_val = xxhash.xxh3_64_intdigest(lowered.encode("utf-8"))
                for i in range(EMBEDDING_DIM):
                    shift = i * 8
                    val = ((hash_val >> shift) & 0xFF) / 255.0
                    embedding[i] += val
                embedding_hits += 1.0

        # TF-IDFとBM25を計算
        tfidf = [0.0] * vocab_len
        bm25 = [0.0] * vocab_len

        doc_len = float(len(tokens))
        length_norm = (
            1.0 - self.bm25_b + self.bm25_b * (doc_len / self.average_doc_len)
            if doc_len > 0.0
            else 1.0
        )

        if total_hits > 0.0:
            for idx, raw in enumerate(raw_counts):
                if raw == 0.0:
                    continue
                tf = raw / total_hits
                tfidf[idx] = tf * self.idf[idx]

                numerator = raw * (self.bm25_k1 + 1.0)
                denominator = raw + self.bm25_k1 * length_norm
                bm25[idx] = self.idf[idx] * (numerator / denominator)

        # Embeddingを平均化
        if embedding_hits > 0.0:
            for i in range(EMBEDDING_DIM):
                embedding[i] /= embedding_hits

        # EmbeddingにZ-score正規化を適用
        self.embedding_stats.normalize(embedding)

        return FeatureVector(tfidf=tfidf, bm25=bm25, embedding=embedding)

