"""Unit tests for domain/selectors.py dedup and MMR selection."""

from __future__ import annotations

import numpy as np

from recap_subworker.domain.selectors import (
    _prune_duplicates_brute,
    mmr_select,
    prune_duplicates,
)


class TestPruneDuplicates:
    def test_empty_input(self) -> None:
        emb = np.array([]).reshape(0, 64)
        indices, removed = prune_duplicates(emb, 0.9)
        assert indices == []
        assert removed == 0

    def test_single_item(self) -> None:
        emb = np.random.randn(1, 64).astype(np.float32)
        emb /= np.linalg.norm(emb)
        indices, removed = prune_duplicates(emb, 0.9)
        assert indices == [0]
        assert removed == 0

    def test_identical_vectors_removed(self) -> None:
        vec = np.random.randn(1, 64).astype(np.float32)
        vec /= np.linalg.norm(vec)
        sources = [vec, vec, vec]
        emb = np.vstack(sources)
        indices, removed = prune_duplicates(emb, 0.99)
        assert len(indices) == 1
        assert removed == len(sources) - 1

    def test_orthogonal_vectors_kept(self) -> None:
        emb = np.eye(3, dtype=np.float32)
        indices, removed = prune_duplicates(emb, 0.5)
        assert len(indices) == emb.shape[0]
        assert removed == 0

    def test_near_duplicates_at_threshold(self) -> None:
        # Create two vectors with known cosine similarity
        v1 = np.array([1.0, 0.0, 0.0], dtype=np.float32)
        # Make v2 clearly different from v1 (cos sim ~0.707)
        v2 = np.array([1.0, 1.0, 0.0], dtype=np.float32)
        v2 /= np.linalg.norm(v2)
        v3 = np.array([0.0, 1.0, 0.0], dtype=np.float32)
        emb = np.vstack([v1, v2, v3])

        # High threshold (0.9): v1-v2 sim ~0.707 < 0.9, so no removal
        _, removed_high = prune_duplicates(emb, 0.9)
        assert removed_high == 0

        # Low threshold (0.5): v1-v2 sim ~0.707 > 0.5, so v2 removed
        expected_remaining = 2
        indices_low, removed_low = prune_duplicates(emb, 0.5)
        assert removed_low == 1
        assert len(indices_low) == expected_remaining

    def test_brute_force_deterministic(self) -> None:
        """Brute-force keeps the first item of a duplicate pair."""
        rng = np.random.RandomState(42)
        emb = rng.randn(10, 32).astype(np.float32)
        norms = np.linalg.norm(emb, axis=1, keepdims=True)
        emb /= norms
        result1, _ = _prune_duplicates_brute(emb, 0.9)
        result2, _ = _prune_duplicates_brute(emb, 0.9)
        assert result1 == result2


class TestMMRSelect:
    def test_empty_input(self) -> None:
        emb = np.array([]).reshape(0, 64)
        assert mmr_select(emb, 5, 0.5) == []

    def test_k_zero(self) -> None:
        emb = np.random.randn(10, 64).astype(np.float32)
        assert mmr_select(emb, 0, 0.5) == []

    def test_k_larger_than_n(self) -> None:
        emb = np.eye(3, dtype=np.float32)
        selected = mmr_select(emb, 10, 0.5)
        assert len(selected) == emb.shape[0]

    def test_selects_k_items(self) -> None:
        emb = np.eye(10, dtype=np.float32)
        top_k = 5
        selected = mmr_select(emb, top_k, 0.5)
        assert len(selected) == top_k
        assert len(set(selected)) == top_k  # all unique

    def test_high_lambda_prefers_relevance(self) -> None:
        """With lambda=1.0 (no diversity penalty), should select items
        most similar to centroid."""
        rng = np.random.RandomState(42)
        emb = rng.randn(20, 32).astype(np.float32)
        norms = np.linalg.norm(emb, axis=1, keepdims=True)
        emb /= norms
        top_k = 5
        selected = mmr_select(emb, top_k, lambda_param=1.0)
        assert len(selected) == top_k

    def test_low_lambda_prefers_diversity(self) -> None:
        """With lambda=0.0 (max diversity), should spread selections."""
        rng = np.random.RandomState(42)
        emb = rng.randn(20, 32).astype(np.float32)
        norms = np.linalg.norm(emb, axis=1, keepdims=True)
        emb /= norms
        top_k = 5
        selected = mmr_select(emb, top_k, lambda_param=0.0)
        assert len(selected) == top_k
