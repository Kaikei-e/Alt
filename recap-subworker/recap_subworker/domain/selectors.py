"""Selection utilities for deduplication and MMR."""

from __future__ import annotations

import time

import numpy as np
import structlog

from ..infra.telemetry import DEDUP_SECONDS, FAISS_DEDUP_ITEMS

_LOGGER = structlog.get_logger(__name__)

# Threshold above which FAISS-based dedup is used instead of brute-force
_FAISS_THRESHOLD_N = 500


def prune_duplicates(embeddings: np.ndarray, threshold: float) -> tuple[list[int], int]:
    """Return indices to keep after removing near-duplicates.

    For small inputs (N < 500), uses brute-force cosine similarity matrix.
    For large inputs (N >= 500), uses FAISS IndexFlatIP with range_search
    for O(N log N) performance instead of O(N^2).
    """
    if embeddings.size == 0:
        return [], 0

    n = embeddings.shape[0]

    start = time.perf_counter()
    if n >= _FAISS_THRESHOLD_N:
        result = _prune_duplicates_faiss(embeddings, threshold)
        DEDUP_SECONDS.labels(method="faiss").observe(time.perf_counter() - start)
        FAISS_DEDUP_ITEMS.observe(n)
        return result
    result = _prune_duplicates_brute(embeddings, threshold)
    DEDUP_SECONDS.labels(method="brute").observe(time.perf_counter() - start)
    return result


def _prune_duplicates_brute(embeddings: np.ndarray, threshold: float) -> tuple[list[int], int]:
    """Brute-force O(N^2) deduplication via full similarity matrix."""
    sim_matrix = embeddings @ embeddings.T
    keep = np.ones(sim_matrix.shape[0], dtype=bool)
    removed = 0
    for i in range(sim_matrix.shape[0]):
        if not keep[i]:
            continue
        duplicates = np.where(sim_matrix[i, i + 1 :] >= threshold)[0]
        for offset in duplicates:
            j = i + 1 + offset
            if keep[j]:
                keep[j] = False
                removed += 1
    indices = [idx for idx, flag in enumerate(keep) if flag]
    return indices, removed


def _prune_duplicates_faiss(embeddings: np.ndarray, threshold: float) -> tuple[list[int], int]:
    """FAISS-based deduplication using IndexFlatIP + range_search.

    Embeddings must be L2-normalized (which they are in our pipeline).
    Inner product of normalized vectors equals cosine similarity.
    """
    try:
        import faiss
    except ImportError:
        _LOGGER.warning("faiss not available, falling back to brute-force dedup")
        return _prune_duplicates_brute(embeddings, threshold)

    n = embeddings.shape[0]
    dim = embeddings.shape[1]

    # Ensure contiguous float32 for FAISS
    emb = np.ascontiguousarray(embeddings, dtype=np.float32)

    # Build inner-product index (cosine sim for normalized vectors)
    index = faiss.IndexFlatIP(dim)
    index.add(emb)

    # range_search returns all pairs with similarity >= threshold
    # FAISS range_search uses radius as a lower bound for IP
    lims, D, I = index.range_search(emb, threshold)

    keep = np.ones(n, dtype=bool)
    removed = 0

    for i in range(n):
        if not keep[i]:
            continue
        # Neighbors of i with similarity >= threshold
        start, end = int(lims[i]), int(lims[i + 1])
        for k in range(start, end):
            j = int(I[k])
            # Only remove later items (j > i) to maintain deterministic order
            if j > i and keep[j]:
                keep[j] = False
                removed += 1

    indices = [idx for idx, flag in enumerate(keep) if flag]
    return indices, removed


def mmr_select(embeddings: np.ndarray, k: int, lambda_param: float) -> list[int]:
    """Select representative indices using Maximal Marginal Relevance."""

    if embeddings.size == 0 or k == 0:
        return []
    k = min(k, embeddings.shape[0])
    centroid = embeddings.mean(axis=0, keepdims=True)
    centroid_similarity = (embeddings @ centroid.T).flatten()
    selected: list[int] = []
    candidate_indices = list(range(embeddings.shape[0]))

    while candidate_indices and len(selected) < k:
        mmr_scores = []
        for idx in candidate_indices:
            diversity_penalty = 0.0
            if selected:
                diversity_penalty = np.max(embeddings[idx] @ embeddings[selected].T)
            score = lambda_param * centroid_similarity[idx] - (1 - lambda_param) * diversity_penalty
            mmr_scores.append(score)
        best_idx = np.argmax(mmr_scores)
        selected_idx = candidate_indices.pop(int(best_idx))
        selected.append(selected_idx)
    return selected
