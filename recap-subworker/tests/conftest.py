"""Shared test fixtures for recap-subworker tests."""

from __future__ import annotations

import hashlib
from typing import Sequence

import numpy as np
import pytest

from recap_subworker.domain.models import (
    ClusterDocument,
    EvidenceConstraints,
    EvidenceRequest,
)
from recap_subworker.infra.config import Settings
from recap_subworker.services.clusterer import ClusterResult, HDBSCANSettings


class HashEmbedder:
    """Deterministic fake embedder using xxhash-like hashing.

    Produces reproducible embeddings based on text content,
    satisfying EmbedderPort protocol.
    """

    def __init__(self, dim: int = 64) -> None:
        self.dim = dim
        self.config = type("Cfg", (), {"backend": "hash", "model_id": "hash-fake"})()

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        result = np.zeros((len(sentences), self.dim), dtype=np.float32)
        for i, text in enumerate(sentences):
            digest = hashlib.sha256(text.encode()).digest()
            for j in range(min(self.dim, 32)):
                result[i, j] = (digest[j] - 128) / 128.0
            # L2-normalize
            norm = np.linalg.norm(result[i])
            if norm > 0:
                result[i] /= norm
        return result

    def warmup(self, samples: Sequence[str]) -> int:
        return len(list(samples))

    def close(self) -> None:
        pass


class FakeClusterer:
    """Fake clusterer that assigns all items to n_clusters groups."""

    def __init__(self, n_clusters: int = 3) -> None:
        self.n_clusters = n_clusters

    def cluster(self, embeddings, *, min_cluster_size, min_samples):
        n = embeddings.shape[0]
        labels = np.array([i % self.n_clusters for i in range(n)], dtype=int)
        probs = np.ones(n, dtype=float)
        return ClusterResult(
            labels,
            probs,
            False,
            HDBSCANSettings(
                min_cluster_size=min_cluster_size,
                min_samples=min_samples,
            ),
        )

    def optimize_clustering(
        self, embeddings, *, min_cluster_size_range, min_samples_range, **kwargs
    ):
        return self.cluster(
            embeddings,
            min_cluster_size=min_cluster_size_range[0],
            min_samples=min_samples_range[0],
        )

    def subcluster_other(self, embeddings, token_counts=None):
        return self.cluster(
            embeddings, min_cluster_size=3, min_samples=2
        )

    def recursive_cluster(self, embeddings, labels, probabilities, token_counts):
        return labels, probabilities


@pytest.fixture
def fake_embedder() -> HashEmbedder:
    """Deterministic fake embedder for unit tests."""
    return HashEmbedder(dim=64)


@pytest.fixture
def fake_clusterer() -> FakeClusterer:
    """Fake clusterer returning n_clusters groups."""
    return FakeClusterer(n_clusters=3)


@pytest.fixture
def test_settings() -> Settings:
    """Minimal Settings instance for unit tests."""
    return Settings(model_id="fake")


def make_cluster_document(
    article_id: str = "art-1",
    paragraph_text: str = "x" * 80,
    n_paragraphs: int = 1,
) -> ClusterDocument:
    """Factory for ClusterDocument with valid defaults."""
    return ClusterDocument(
        article_id=article_id,
        title="Test Article",
        paragraphs=[paragraph_text] * n_paragraphs,
    )


def make_evidence_request(
    n_docs: int = 5,
    job_id: str = "test-job",
    genre: str = "tech",
) -> EvidenceRequest:
    """Factory for EvidenceRequest with valid defaults."""
    return EvidenceRequest(
        job_id=job_id,
        genre=genre,
        documents=[
            make_cluster_document(article_id=f"art-{i}")
            for i in range(n_docs)
        ],
        constraints=EvidenceConstraints(),
    )
