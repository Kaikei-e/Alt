"""Selection utilities for deduplication and MMR."""

from __future__ import annotations

from typing import Iterable, Sequence

import numpy as np


def prune_duplicates(embeddings: np.ndarray, threshold: float) -> tuple[list[int], int]:
    """Return indices to keep after removing near-duplicates."""

    if embeddings.size == 0:
        return [], 0
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
