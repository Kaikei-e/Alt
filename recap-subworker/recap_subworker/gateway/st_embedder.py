"""SentenceTransformer embedder gateway implementing EmbedderPort.

This is a thin delegation layer: the actual model loading and inference
logic remains in services/embedder.py (Embedder class). This gateway
adapts it to satisfy the EmbedderPort protocol.
"""

from __future__ import annotations

from typing import Sequence

import numpy as np

from ..services.embedder import Embedder, EmbedderConfig


class StEmbedderGateway:
    """Gateway wrapping the existing Embedder for EmbedderPort compliance."""

    def __init__(self, config: EmbedderConfig) -> None:
        self._embedder = Embedder(config)

    @property
    def config(self) -> EmbedderConfig:
        return self._embedder.config

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        return self._embedder.encode(sentences)

    def warmup(self, samples: Sequence[str]) -> int:
        return self._embedder.warmup(samples)

    def close(self) -> None:
        self._embedder.close()
