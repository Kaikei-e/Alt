"""Candle を用いた軽量ハイブリッド分類モデル。"""

import json
from pathlib import Path
from typing import Dict, List, Optional, Tuple

import numpy as np

from .features import (
    EMBEDDING_DIM,
    FALLBACK_AVG_DOC_LEN,
    FALLBACK_BM25_B,
    FALLBACK_BM25_K1,
    FALLBACK_IDF,
    FALLBACK_VOCAB,
    FeatureExtractor,
    FeatureVector,
)

# Default weights (embedded in Rust code)
DEFAULT_WEIGHTS_JSON = """{
  "feature_dim": 19,
  "embedding_dim": 6,
  "feature_vocab": [],
  "feature_idf": [],
  "genres": ["ai", "tech", "business", "science", "entertainment", "sports", "politics", "health", "world", "security", "society_justice", "art_culture", "other"],
  "tfidf_weights": [],
  "embedding_weights": [],
  "bias": []
}"""



class GenreClassifier:
    """ジャンル分類器。"""

    def __init__(
        self,
        weights_path: Optional[str] = None,
        weights_data: Optional[Dict] = None,
    ):
        """初期化。

        Args:
            weights_path: 重みファイルのパス（オプション）
            weights_data: 重みデータの辞書（オプション、weights_pathより優先）
        """
        if weights_data is None:
            if weights_path is None:
                # 環境変数から取得を試みる
                import os

                weights_path = os.getenv("RECAP_GENRE_MODEL_WEIGHTS")
            if weights_path is None:
                # デフォルトの重みファイルパスを試す
                default_weights_path = Path("/app/resources/genre_classifier_weights.json")
                if default_weights_path.exists():
                    weights_path = str(default_weights_path)
            if weights_path:
                with open(weights_path, "r", encoding="utf-8") as f:
                    weights_data = json.load(f)
            else:
                # デフォルトの埋め込みJSONを使用（フォールバック）
                weights_data = json.loads(DEFAULT_WEIGHTS_JSON)

        self._validate_weights(weights_data)

        self.genres = weights_data["genres"]
        self.feature_dim = weights_data["feature_dim"]
        self.feature_vocab = (
            weights_data.get("feature_vocab") or FALLBACK_VOCAB.copy()
        )
        self.feature_idf = weights_data.get("feature_idf") or FALLBACK_IDF.copy()
        self.bm25_k1 = weights_data.get("bm25_k1") or FALLBACK_BM25_K1
        self.bm25_b = weights_data.get("bm25_b") or FALLBACK_BM25_B
        self.average_doc_len = (
            weights_data.get("average_doc_len") or FALLBACK_AVG_DOC_LEN
        )

        self.tfidf_weight = np.array(weights_data["tfidf_weights"])
        self.embedding_weight = np.array(weights_data["embedding_weights"])
        self.bias = np.array(weights_data["bias"])

        # FeatureExtractorを初期化
        self.feature_extractor = FeatureExtractor.from_metadata(
            self.feature_vocab,
            self.feature_idf,
            self.bm25_k1,
            self.bm25_b,
            self.average_doc_len,
        )

    def _validate_weights(self, weights: Dict) -> None:
        """重みデータを検証。"""
        if not weights.get("tfidf_weights") or len(weights["tfidf_weights"]) == 0:
            raise ValueError(
                "tfidf_weights is empty. Please provide a valid weights file via RECAP_GENRE_MODEL_WEIGHTS environment variable or weights_path parameter."
            )
        if len(weights["tfidf_weights"]) != len(weights["genres"]):
            raise ValueError(
                f"tfidf weight matrix row count mismatch: expected {len(weights['genres'])} rows (one per genre), got {len(weights['tfidf_weights'])} rows"
            )
        if not weights.get("embedding_weights") or len(weights["embedding_weights"]) == 0:
            raise ValueError(
                "embedding_weights is empty. Please provide a valid weights file via RECAP_GENRE_MODEL_WEIGHTS environment variable or weights_path parameter."
            )
        if len(weights["embedding_weights"]) != len(weights["genres"]):
            raise ValueError(
                f"embedding weight matrix row count mismatch: expected {len(weights['genres'])} rows (one per genre), got {len(weights['embedding_weights'])} rows"
            )
        for row in weights["tfidf_weights"]:
            if len(row) != weights["feature_dim"]:
                raise ValueError("tfidf weight row length mismatch")
        for row in weights["embedding_weights"]:
            if len(row) != weights["embedding_dim"]:
                raise ValueError("embedding weight row length mismatch")
        if not weights.get("bias") or len(weights["bias"]) == 0:
            raise ValueError(
                "bias is empty. Please provide a valid weights file via RECAP_GENRE_MODEL_WEIGHTS environment variable or weights_path parameter."
            )
        if len(weights["bias"]) != len(weights["genres"]):
            raise ValueError(
                f"bias length mismatch: expected {len(weights['genres'])} values (one per genre), got {len(weights['bias'])} values"
            )
        if weights.get("feature_vocab") and len(weights["feature_vocab"]) != weights[
            "feature_dim"
        ]:
            raise ValueError("feature vocab length mismatch")
        if weights.get("feature_idf") and len(weights["feature_idf"]) != weights[
            "feature_dim"
        ]:
            raise ValueError("feature idf length mismatch")

    def predict(self, features: FeatureVector) -> List[Tuple[str, float]]:
        """特徴ベクトルからジャンルを予測。

        Args:
            features: 特徴ベクトル

        Returns:
            ジャンルとスコアのペアのリスト（スコア降順）
        """
        if len(features.tfidf) != self.feature_dim:
            raise ValueError(
                f"feature dimension mismatch: expected {self.feature_dim}, got {len(features.tfidf)}"
            )
        if len(features.embedding) != EMBEDDING_DIM:
            raise ValueError("embedding dimension mismatch")

        # NumPy配列に変換
        tfidf_vec = np.array(features.tfidf)
        embedding_vec = np.array(features.embedding)

        # 各ジャンルのスコアを計算
        scores = []
        for idx, genre in enumerate(self.genres):
            score = float(self.bias[idx])
            # TF-IDF重みとの内積
            score += float(np.dot(tfidf_vec, self.tfidf_weight[idx]))
            # Embedding重みとの内積
            score += float(np.dot(embedding_vec, self.embedding_weight[idx]))
            scores.append((genre, score))

        # スコア降順でソート
        scores.sort(key=lambda x: x[1], reverse=True)
        return scores

