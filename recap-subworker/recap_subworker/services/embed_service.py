"""Embedding application service.

Exposes text embedding generation conforming to the /v1/embed endpoint contract,
reusing the existing EmbedderPort / gateway infrastructure and canonical
model identity resolution (ADR-000872 / ADR-000899).
"""

from __future__ import annotations

from collections.abc import Sequence

import numpy as np
from pydantic import BaseModel

from ..port.embedder import EmbedderPort


class EmbedResponse(BaseModel):
    """Response payload for text embedding."""

    model: str
    dim: int
    embeddings: list[list[float]]


class EmbedderError(Exception):
    """Base exception for embedder backend errors."""


class UnresolvableModelIdentityError(EmbedderError):
    """Raised when the underlying embedder backend does not report a canonical model identity."""


class UnresolvableDimensionError(EmbedderError):
    """Raised when the underlying embedder backend does not report or produce valid vector dimensions."""


class EmbedService:
    """Service generating sentence embeddings for text collections."""

    def __init__(self, embedder: EmbedderPort) -> None:
        self.embedder = embedder

    def resolve_model_identity(self) -> str:
        """Resolve canonical runtime embedder identity from underlying embedder.

        Follows ADR-000872 canonical naming and existing backend configurations.
        Fails closed: raises UnresolvableModelIdentityError if the backend
        does not report a canonical identity. Never substitutes a synthetic default.
        """
        # 1. Explicit getter method if provided
        if hasattr(self.embedder, "get_model_identity") and callable(
            self.embedder.get_model_identity
        ):
            ident = self.embedder.get_model_identity()
            if ident and str(ident).strip():
                return str(ident).strip()

        # 2. Inspect embedder.config (Embedder / StEmbedderGateway / HashEmbedder)
        config = getattr(self.embedder, "config", None)
        if config is not None:
            backend = getattr(config, "backend", None)
            if backend == "ollama-remote":
                model = getattr(config, "ollama_embed_model", None)
                if model and str(model).strip():
                    return str(model).strip()
            elif backend == "sentence-transformers":
                model = getattr(config, "model_id", None)
                if model and str(model).strip():
                    return str(model).strip()
            elif backend == "onnx":
                model = getattr(config, "onnx_tokenizer_name", None) or getattr(
                    config, "model_id", None
                )
                if model and str(model).strip():
                    return str(model).strip()
            elif backend == "hash":
                model = getattr(config, "model_id", None)
                if model and str(model).strip():
                    return str(model).strip()
            elif getattr(config, "model_id", None):
                model = str(config.model_id).strip()
                if model:
                    return model

        # 3. Attributes directly on embedder
        if hasattr(self.embedder, "model_id"):
            model = getattr(self.embedder, "model_id", None)
            if model and str(model).strip():
                return str(model).strip()
        if hasattr(self.embedder, "model_name"):
            model = getattr(self.embedder, "model_name", None)
            if model and str(model).strip():
                return str(model).strip()

        raise UnresolvableModelIdentityError(
            "Embedder backend did not report a canonical model identity; failing closed."
        )

    def embed(
        self,
        texts: Sequence[str],
        normalize: bool = True,
    ) -> EmbedResponse:
        """Generate embedding vectors for the given texts in input order.

        Args:
            texts: List of 1..256 text strings.
            normalize: When True, vectors are L2-normalized.

        Returns:
            EmbedResponse with model identity, dimension, and embeddings in input order.

        Raises:
            UnresolvableModelIdentityError: If embedder model identity cannot be resolved.
            UnresolvableDimensionError: If embedding vector dimension cannot be determined.
            EmbedderError: If embedder backend fails during encoding.
        """
        model_id = self.resolve_model_identity()

        if not texts:
            dim = getattr(self.embedder, "dim", None) or getattr(self.embedder, "dimension", None)
            if dim is None or int(dim) <= 0:
                raise UnresolvableDimensionError(
                    "Embedder backend did not produce vectors or report embedding dimension."
                )
            return EmbedResponse(
                model=model_id,
                dim=int(dim),
                embeddings=[],
            )

        # Delegate to underlying embedder port
        try:
            vectors = self.embedder.encode(texts)
        except Exception as exc:
            raise EmbedderError(f"Embedder backend encode failed: {exc}") from exc

        if not isinstance(vectors, np.ndarray):
            vectors = np.array(vectors, dtype=np.float32)

        if vectors.ndim != 2 or vectors.shape[1] <= 0:
            raise UnresolvableDimensionError(
                f"Embedder backend returned invalid vector shape {vectors.shape}; expected 2D with dim > 0."
            )

        dim = int(vectors.shape[1])

        # Ensure L2 normalization when requested
        if normalize and len(vectors) > 0:
            norms = np.linalg.norm(vectors, axis=1, keepdims=True)
            norms = np.where(norms == 0.0, 1.0, norms)
            vectors = vectors / norms

        return EmbedResponse(
            model=model_id,
            dim=dim,
            embeddings=vectors.tolist(),
        )
