"""Unit tests for domain/selectors.py dedup and MMR selection."""

from __future__ import annotations

import numpy as np
import pytest

from recap_subworker.domain.selectors import (
    _prune_duplicates_brute,
    mmr_select,
    prune_duplicates,
)


class TestPruneDuplicates:
    def test_empty_input(self):
        emb = np.array([]).reshape(0, 64)
        indices, removed = prune_duplicates(emb, 0.9)
        assert indices == []
        assert removed == 0

    def test_single_item(self):
        emb = np.random.randn(1, 64).astype(np.float32)
        emb /= np.linalg.norm(emb)
        indices, removed = prune_duplicates(emb, 0.9)
        assert indices == [0]
        assert removed == 0

    def test_identical_vectors_removed(self):
        vec = np.random.randn(1, 64).astype(np.float32)
        vec /= np.linalg.norm(vec)
        emb = np.vstack([vec, vec, vec])
        indices, removed = prune_duplicates(emb, 0.99)
        assert len(indices) == 1
        assert removed == 2

    def test_orthogonal_vectors_kept(self):
        emb = np.eye(3, dtype=np.float32)
        indices, removed = prune_duplicates(emb, 0.5)
        assert len(indices) == 3
        assert removed == 0

    def test_near_duplicates_at_threshold(self):
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
        indices_low, removed_low = prune_duplicates(emb, 0.5)
        assert removed_low == 1
        assert len(indices_low) == 2

    def test_brute_force_deterministic(self):
        """Brute-force keeps the first item of a duplicate pair."""
        rng = np.random.RandomState(42)
        emb = rng.randn(10, 32).astype(np.float32)
        norms = np.linalg.norm(emb, axis=1, keepdims=True)
        emb /= norms
        result1, _ = _prune_duplicates_brute(emb, 0.9)
        result2, _ = _prune_duplicates_brute(emb, 0.9)
        assert result1 == result2


class TestMMRSelect:
    def test_empty_input(self):
        emb = np.array([]).reshape(0, 64)
        assert mmr_select(emb, 5, 0.5) == []

    def test_k_zero(self):
        emb = np.random.randn(10, 64).astype(np.float32)
        assert mmr_select(emb, 0, 0.5) == []

    def test_k_larger_than_n(self):
        emb = np.eye(3, dtype=np.float32)
        selected = mmr_select(emb, 10, 0.5)
        assert len(selected) == 3

    def test_selects_k_items(self):
        emb = np.eye(10, dtype=np.float32)
        selected = mmr_select(emb, 5, 0.5)
        assert len(selected) == 5
        assert len(set(selected)) == 5  # all unique

    def test_high_lambda_prefers_relevance(self):
        """With lambda=1.0 (no diversity penalty), should select items
        most similar to centroid."""
        rng = np.random.RandomState(42)
        emb = rng.randn(20, 32).astype(np.float32)
        norms = np.linalg.norm(emb, axis=1, keepdims=True)
        emb /= norms
        selected = mmr_select(emb, 5, lambda_param=1.0)
        assert len(selected) == 5

    def test_low_lambda_prefers_diversity(self):
        """With lambda=0.0 (max diversity), should spread selections."""
        rng = np.random.RandomState(42)
        emb = rng.randn(20, 32).astype(np.float32)
        norms = np.linalg.norm(emb, axis=1, keepdims=True)
        emb /= norms
        selected = mmr_select(emb, 5, lambda_param=0.0)
        assert len(selected) == 5
