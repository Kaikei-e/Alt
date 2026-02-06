"""Embedder port: Protocol for sentence embedding generation."""

from __future__ import annotations

from typing import Protocol, Sequence, runtime_checkable

import numpy as np


@runtime_checkable
class EmbedderPort(Protocol):
    """Port for generating sentence embeddings.

    Implementations: SentenceTransformer, ONNX, Ollama Remote, Hash (test).
    """

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        """Encode sentences into normalized embedding vectors.

        Args:
            sentences: List of text strings to embed.

        Returns:
            (N, D) float32 array of L2-normalized embeddings.
        """
        ...

    def warmup(self, samples: Sequence[str]) -> int:
        """Prime the model by embedding sample sentences.

        Args:
            samples: Sample sentences for warm-up.

        Returns:
            Number of sentences processed.
        """
        ...

    def close(self) -> None:
        """Release model resources."""
        ...
