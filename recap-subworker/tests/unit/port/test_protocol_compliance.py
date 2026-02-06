"""Tests verifying that gateway implementations satisfy port protocols."""

from __future__ import annotations

import numpy as np
import pytest

from recap_subworker.port.embedder import EmbedderPort


class TestEmbedderPortCompliance:
    """Verify the HashEmbedder fixture satisfies EmbedderPort protocol."""

    def test_hash_embedder_is_embedder_port(self, fake_embedder):
        assert isinstance(fake_embedder, EmbedderPort)

    def test_encode_returns_ndarray(self, fake_embedder):
        result = fake_embedder.encode(["hello", "world"])
        assert isinstance(result, np.ndarray)
        assert result.shape == (2, 64)

    def test_warmup_returns_int(self, fake_embedder):
        count = fake_embedder.warmup(["test"])
        assert isinstance(count, int)
        assert count == 1

    def test_close_is_callable(self, fake_embedder):
        fake_embedder.close()  # should not raise


class TestFakeClustererShape:
    """Verify FakeClusterer returns correct shapes."""

    def test_cluster_returns_correct_labels(self, fake_clusterer):
        emb = np.random.randn(10, 64).astype(np.float32)
        result = fake_clusterer.cluster(emb, min_cluster_size=3, min_samples=2)
        assert result.labels.shape == (10,)
        assert result.probabilities.shape == (10,)

    def test_optimize_delegates_to_cluster(self, fake_clusterer):
        emb = np.random.randn(10, 64).astype(np.float32)
        result = fake_clusterer.optimize_clustering(
            emb,
            min_cluster_size_range=(3, 10),
            min_samples_range=(2, 5),
        )
        assert result.labels.shape == (10,)
