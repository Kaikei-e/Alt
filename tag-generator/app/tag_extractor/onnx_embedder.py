from __future__ import annotations

from dataclasses import dataclass
from itertools import islice
from typing import Iterable, Iterator, Sequence

try:
    import numpy as np
except ImportError:  # pragma: no cover
    np = None  # type: ignore[assignment]

try:
    import onnxruntime as ort  # type: ignore
except ImportError:  # pragma: no cover
    ort = None  # type: ignore[assignment]

try:
    from transformers import AutoTokenizer
except ImportError:  # pragma: no cover
    AutoTokenizer = None  # type: ignore[assignment]


class OnnxRuntimeMissing(ImportError):
    """Raised when ONNX runtime dependencies are not available."""


@dataclass
class OnnxEmbeddingConfig:
    """Configuration for the ONNX embedding helper."""

    model_path: str
    tokenizer_name: str
    pooling: str = "cls"
    batch_size: int = 16
    max_length: int = 256


class OnnxEmbeddingModel:
    """Lightweight adapter that exposes `.encode()` like SentenceTransformer."""

    def __init__(self, config: OnnxEmbeddingConfig) -> None:
        if ort is None or AutoTokenizer is None or np is None:
            raise OnnxRuntimeMissing(
                "onnxruntime, transformers, and numpy are required for ONNX embedding support"
            )

        if config.pooling not in {"cls", "mean"}:
            raise ValueError("Pooling must be 'cls' or 'mean'")

        self._session = ort.InferenceSession(
            config.model_path, providers=["CPUExecutionProvider"]
        )
        self._tokenizer = AutoTokenizer.from_pretrained(config.tokenizer_name, use_fast=True)
        self._config = config

    def encode(
        self,
        texts: Sequence[str],
        batch_size: int | None = None,
        show_progress_bar: bool = False,  # Mirror SentenceTransformer signature
    ) -> np.ndarray:
        if batch_size is None:
            batch_size = self._config.batch_size

        embeddings: list[np.ndarray] = []
        for batch in self._batch(texts, batch_size):
            tokens = self._tokenizer(
                list(batch),
                padding=True,
                truncation=True,
                max_length=self._config.max_length,
                return_tensors="np",
            )

            ort_inputs = {k: v for k, v in tokens.items()}

            hidden_states = self._session.run(None, ort_inputs)[0]
            if self._config.pooling == "mean":
                emb = hidden_states.mean(axis=1)
            else:
                emb = hidden_states[:, 0, :]

            embeddings.append(emb)

        if not embeddings:
            return np.zeros((0, self._session.get_outputs()[0].shape[-1]), dtype=np.float32)

        return np.vstack(embeddings)

    @staticmethod
    def _batch(iterable: Iterable[str], size: int) -> Iterator[list[str]]:
        iterator = iter(iterable)
        while True:
            chunk = list(islice(iterator, size))
            if not chunk:
                break
            yield chunk
