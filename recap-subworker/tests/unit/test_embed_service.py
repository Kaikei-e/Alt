"""Unit tests for EmbedService with synthetic vectors."""

from __future__ import annotations

from collections.abc import Sequence

import numpy as np
import pytest

from recap_subworker.services.embed_service import (
    EmbedService,
    UnresolvableDimensionError,
    UnresolvableModelIdentityError,
)


class _SyntheticEmbedder:
    """Fake embedder returning pre-configured synthetic vectors."""

    def __init__(
        self,
        vectors: dict[str, list[float]],
        model_id: str = "test-embed-model",
    ) -> None:
        self.vectors = vectors
        self.model_id = model_id
        self.config = type(
            "Cfg",
            (),
            {"backend": "ollama-remote", "ollama_embed_model": model_id},
        )()

    def encode(
        self,
        sentences: Sequence[str],
    ) -> np.ndarray:
        dim = len(next(iter(self.vectors.values())))
        arr = np.zeros((len(sentences), dim), dtype=np.float32)
        for i, s in enumerate(sentences):
            if s in self.vectors:
                arr[i] = self.vectors[s]
            else:
                arr[i] = np.ones(dim, dtype=np.float32)
        return arr

    def warmup(self, samples: Sequence[str]) -> int:
        return len(samples)

    def close(self) -> None:
        pass


def test_embed_service_normalizes_embeddings():
    """Verify that EmbedService returns L2-normalized embeddings when normalize=True."""
    synthetic_vectors = {
        "Example headline 1": [3.0, 4.0, 0.0],  # norm = 5.0 -> normalized: [0.6, 0.8, 0.0]
        "Example headline 2": [0.0, 0.0, 2.0],  # norm = 2.0 -> normalized: [0.0, 0.0, 1.0]
    }
    embedder = _SyntheticEmbedder(synthetic_vectors)
    service = EmbedService(embedder=embedder)

    result = service.embed(["Example headline 1", "Example headline 2"], normalize=True)

    assert result.model == "test-embed-model"
    assert result.dim == 3
    assert len(result.embeddings) == 2

    # Check L2 norm is 1.0
    vec1 = np.array(result.embeddings[0])
    vec2 = np.array(result.embeddings[1])
    assert np.linalg.norm(vec1) == pytest.approx(1.0, rel=1e-5)
    assert np.linalg.norm(vec2) == pytest.approx(1.0, rel=1e-5)
    assert vec1[0] == pytest.approx(0.6, rel=1e-5)
    assert vec1[1] == pytest.approx(0.8, rel=1e-5)


def test_embed_service_unnormalized():
    """Verify that EmbedService preserves unnormalized embeddings when normalize=False."""
    synthetic_vectors = {
        "Example headline 1": [3.0, 4.0, 0.0],
    }
    embedder = _SyntheticEmbedder(synthetic_vectors)
    service = EmbedService(embedder=embedder)

    result = service.embed(["Example headline 1"], normalize=False)

    vec = np.array(result.embeddings[0])
    assert np.linalg.norm(vec) == pytest.approx(5.0, rel=1e-5)
    assert vec[0] == pytest.approx(3.0, rel=1e-5)
    assert vec[1] == pytest.approx(4.0, rel=1e-5)


def test_embed_service_preserves_input_order():
    """Verify that EmbedService maintains exact input order for returned embeddings."""
    synthetic_vectors = {
        "Example headline 1": [1.0, 0.0, 0.0],
        "Example headline 2": [0.0, 1.0, 0.0],
        "Example headline 3": [0.0, 0.0, 1.0],
    }
    embedder = _SyntheticEmbedder(synthetic_vectors)
    service = EmbedService(embedder=embedder)

    texts = ["Example headline 3", "Example headline 1", "Example headline 2"]
    result = service.embed(texts, normalize=True)

    assert len(result.embeddings) == 3
    assert result.embeddings[0] == [0.0, 0.0, 1.0]
    assert result.embeddings[1] == [1.0, 0.0, 0.0]
    assert result.embeddings[2] == [0.0, 1.0, 0.0]


def test_embed_service_resolves_model_identity():
    """Verify that EmbedService resolves model identity from embedder configuration."""
    synthetic_vectors = {"Example headline 1": [1.0, 2.0]}
    embedder = _SyntheticEmbedder(synthetic_vectors, model_id="bge-m3")
    service = EmbedService(embedder=embedder)

    result = service.embed(["Example headline 1"], normalize=True)
    assert result.model == "bge-m3"
    assert result.dim == 2


def test_embed_service_unresolvable_identity_fails_closed():
    """Verify that EmbedService raises UnresolvableModelIdentityError when identity cannot be resolved."""

    class _AnonymousEmbedder:
        def encode(self, sentences: Sequence[str]) -> np.ndarray:
            return np.ones((len(sentences), 4), dtype=np.float32)

        def warmup(self, samples: Sequence[str]) -> int:
            return len(samples)

        def close(self) -> None:
            pass

    service = EmbedService(embedder=_AnonymousEmbedder())
    with pytest.raises(UnresolvableModelIdentityError):
        service.resolve_model_identity()

    with pytest.raises(UnresolvableModelIdentityError):
        service.embed(["Example text"])


def test_embed_service_unresolvable_dimension_fails_closed():
    """Verify that EmbedService raises UnresolvableDimensionError when dimensions are invalid."""

    class _BadShapeEmbedder:
        model_id = "test-model"

        def encode(self, sentences: Sequence[str]) -> np.ndarray:
            return np.array([1.0, 2.0], dtype=np.float32)  # 1D array instead of 2D

        def warmup(self, samples: Sequence[str]) -> int:
            return len(samples)

        def close(self) -> None:
            pass

    service = EmbedService(embedder=_BadShapeEmbedder())
    with pytest.raises(UnresolvableDimensionError):
        service.embed(["Example text"])
