"""Sentence embedding utilities."""

from __future__ import annotations

import hashlib
import math
from dataclasses import dataclass
from typing import Iterable, Literal, Sequence

import numpy as np

from ..infra.cache import LRUCache


BackendLiteral = Literal["sentence-transformers", "onnx", "hash"]


@dataclass(slots=True)
class EmbedderConfig:
    model_id: str
    distill_model_id: str
    backend: BackendLiteral
    device: str
    batch_size: int
    cache_size: int


class Embedder:
    """Embedding facade supporting multiple backends."""

    def __init__(self, config: EmbedderConfig) -> None:
        self.config = config
        self._model = None
        self._cache = LRUCache[str, np.ndarray](config.cache_size)
        self._hash_dimension = 256

    def _load_sentence_transformer(self):
        from sentence_transformers import SentenceTransformer  # lazy import

        return SentenceTransformer(self.config.model_id, device=self.config.device)

    def _ensure_model(self):
        if self._model is not None:
            return
        if self.config.backend == "sentence-transformers":
            self._model = self._load_sentence_transformer()
        elif self.config.backend == "onnx":
            # For now we re-use SentenceTransformer while keeping the backend flag so that
            # configuration remains forward compatible with a true ONNX implementation.
            self._model = self._load_sentence_transformer()
        else:
            # hash backend does not require lazy model initialization
            self._model = None

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        """Generate embeddings for sentences."""

        if not sentences:
            return np.empty((0, 0), dtype=np.float32)
        cached = self._fetch_cached(sentences)
        pending = [s for s in sentences if s not in cached]
        fresh: dict[str, np.ndarray] = {}
        if pending:
            self._ensure_model()
            if self.config.backend == "hash":
                for sentence in pending:
                    vector = self._hash_sentence(sentence)
                    fresh[sentence] = vector
                    self._cache.set(sentence, vector)
            else:
                model = self._model
                assert model is not None
                embeddings = model.encode(  # type: ignore[attr-defined]
                    pending,
                    batch_size=self.config.batch_size,
                    normalize_embeddings=True,
                )
                for sentence, vector in zip(pending, embeddings):
                    stored = np.asarray(vector, dtype=np.float32)
                    fresh[sentence] = stored
                    self._cache.set(sentence, stored)

        merged: dict[str, np.ndarray] = {}
        merged.update(cached)
        merged.update(fresh)

        ordered_vectors: list[np.ndarray] = []
        for sentence in sentences:
            vector = merged.get(sentence)
            if vector is not None:
                ordered_vectors.append(vector)
                continue

            if self.config.backend != "hash":
                raise KeyError(f"embedding missing for sentence: {sentence[:32]}")

            vector = self._hash_sentence(sentence)
            self._cache.set(sentence, vector)
            merged[sentence] = vector
            ordered_vectors.append(vector)

        if not ordered_vectors:
            return np.empty((0, 0), dtype=np.float32)
        return np.vstack(ordered_vectors)

    def _fetch_cached(self, sentences: Sequence[str]) -> dict[str, np.ndarray]:
        cached: dict[str, np.ndarray] = {}
        for sentence in sentences:
            vector = self._cache.get(sentence)
            if vector is not None:
                cached[sentence] = vector
        return cached

    def warmup(self, samples: Iterable[str]) -> int:
        """Prime the model by embedding sample sentences."""

        sample_list = [s for s in samples if s]
        if not sample_list:
            return 0
        self.encode(sample_list)
        return len(sample_list)

    def close(self) -> None:
        """Release resources (if any)."""

        self._model = None

    def _hash_sentence(self, sentence: str) -> np.ndarray:
        digest = hashlib.sha256(sentence.encode("utf-8")).digest()
        seed = int.from_bytes(digest[:8], "little")
        rng = np.random.default_rng(seed)
        vector = rng.normal(loc=0.0, scale=1.0, size=self._hash_dimension)
        norm = np.linalg.norm(vector)
        if not math.isfinite(norm) or norm == 0.0:
            return np.zeros(self._hash_dimension, dtype=np.float32)
        return (vector / norm).astype(np.float32)
