"""Sentence embedding utilities."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable, Literal, Sequence

import numpy as np

from ..infra.cache import LRUCache


BackendLiteral = Literal["sentence-transformers", "onnx"]


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

    def _load_sentence_transformer(self):
        from sentence_transformers import SentenceTransformer  # lazy import

        return SentenceTransformer(self.config.model_id, device=self.config.device)

    def _ensure_model(self):
        if self._model is not None:
            return
        if self.config.backend == "sentence-transformers":
            self._model = self._load_sentence_transformer()
        else:
            # For now we re-use SentenceTransformer while keeping the backend flag so that
            # configuration remains forward compatible with a true ONNX implementation.
            self._model = self._load_sentence_transformer()

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        """Generate embeddings for sentences."""

        if not sentences:
            return np.empty((0, 0), dtype=np.float32)
        cached = self._fetch_cached(sentences)
        pending = [s for s in sentences if s not in cached]
        vectors = {}
        if pending:
            self._ensure_model()
            model = self._model
            assert model is not None
            embeddings = model.encode(  # type: ignore[attr-defined]
                pending,
                batch_size=self.config.batch_size,
                normalize_embeddings=True,
            )
            for sentence, vector in zip(pending, embeddings):
                vectors[sentence] = np.asarray(vector, dtype=np.float32)
                self._cache.set(sentence, vectors[sentence])
        return np.vstack([cached.get(s) or vectors.get(s) for s in sentences])

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
