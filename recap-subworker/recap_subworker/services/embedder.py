"""Sentence embedding utilities."""

from __future__ import annotations

import hashlib
import math
import time
from dataclasses import dataclass
from typing import Iterable, Literal, Sequence

import numpy as np
import structlog

from ..infra.cache import LRUCache

from threading import Lock

logger = structlog.get_logger(__name__)


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
        self._lock = Lock()

    def _load_sentence_transformer(self):
        from sentence_transformers import SentenceTransformer  # lazy import

        logger.info(
            "Loading SentenceTransformer",
            model_id=self.config.model_id,
            backend=self.config.backend
        )

        if self.config.model_id != "intfloat/multilingual-e5-large":
            logger.warn(
                "Model ID mismatch recommendation",
                current=self.config.model_id,
                recommended="intfloat/multilingual-e5-large"
            )

        model_kwargs = {
            "low_cpu_mem_usage": False,
            "trust_remote_code": False,
        }

        if self.config.device.startswith("cuda"):
            import torch
            logger.info("Enabling FP16 for CUDA device")
            model_kwargs["torch_dtype"] = torch.float16

        logger.info("Initializing SentenceTransformer model (this may take time for large models)...")
        model = SentenceTransformer(
            self.config.model_id,
            device=self.config.device,
            model_kwargs=model_kwargs,
        )
        logger.info("SentenceTransformer model initialized", device=self.config.device)
        return model

    def _ensure_model(self):
        if self._model is not None:
            return

        with self._lock:
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

        total_sentences = len(sentences)
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

                # Manual batching with progress logging for large batches
                if len(pending) > self.config.batch_size * 2:
                    logger.info(
                        "Starting embedding generation with progress tracking",
                        total_sentences=total_sentences,
                        cached_count=len(cached),
                        pending_count=len(pending),
                        batch_size=self.config.batch_size,
                        estimated_batches=math.ceil(len(pending) / self.config.batch_size),
                    )

                    all_embeddings = []
                    start_time = time.time()

                    for batch_idx in range(0, len(pending), self.config.batch_size):
                        batch = pending[batch_idx:batch_idx + self.config.batch_size]
                        batch_start = time.time()

                        batch_embeddings = model.encode(  # type: ignore[attr-defined]
                            batch,
                            batch_size=len(batch),
                            normalize_embeddings=True,
                            show_progress_bar=False,  # Disable tqdm progress bar
                        )

                        batch_elapsed = time.time() - batch_start
                        batch_num = (batch_idx // self.config.batch_size) + 1
                        total_batches = math.ceil(len(pending) / self.config.batch_size)
                        progress_pct = (batch_num / total_batches) * 100
                        elapsed_total = time.time() - start_time
                        avg_time_per_batch = elapsed_total / batch_num
                        remaining_batches = total_batches - batch_num
                        eta_seconds = avg_time_per_batch * remaining_batches

                        logger.info(
                            "Embedding batch progress",
                            batch_num=batch_num,
                            total_batches=total_batches,
                            progress_percent=round(progress_pct, 1),
                            batch_size=len(batch),
                            batch_seconds=round(batch_elapsed, 2),
                            elapsed_seconds=round(elapsed_total, 2),
                            avg_seconds_per_batch=round(avg_time_per_batch, 2),
                            eta_seconds=round(eta_seconds, 2),
                            eta_minutes=round(eta_seconds / 60, 1),
                        )

                        all_embeddings.extend(batch_embeddings)

                    embeddings = np.array(all_embeddings)
                    total_elapsed = time.time() - start_time
                    logger.info(
                        "Embedding generation completed",
                        total_sentences=len(pending),
                        total_seconds=round(total_elapsed, 2),
                        throughput_per_sec=round(len(pending) / total_elapsed, 2) if total_elapsed > 0 else 0,
                    )
                else:
                    # Small batches: use direct encoding without progress logging
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
