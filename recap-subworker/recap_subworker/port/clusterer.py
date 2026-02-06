"""Clusterer port: Protocol for clustering embeddings."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol, runtime_checkable

import numpy as np

from ..domain.models import HDBSCANSettings


@dataclass
class ClusterResult:
    """Result of a clustering operation."""

    labels: np.ndarray
    probabilities: np.ndarray
    used_umap: bool
    params: HDBSCANSettings
    dbcv_score: float = 0.0
    silhouette_score: float = 0.0
    used_fallback: bool = False


@runtime_checkable
class ClustererPort(Protocol):
    """Port for clustering embedding vectors."""

    def cluster(
        self,
        embeddings: np.ndarray,
        *,
        min_cluster_size: int,
        min_samples: int,
        umap_n_neighbors: int | None = None,
        umap_n_components: int | None = None,
        umap_min_dist: float | None = None,
        hdbscan_cluster_selection_epsilon: float | None = None,
        hdbscan_cluster_selection_method: str | None = None,
        hdbscan_allow_single_cluster: bool | None = None,
    ) -> ClusterResult:
        """Cluster embeddings using dimensionality reduction + density-based clustering.

        Args:
            embeddings: (N, D) float32 array of normalized embeddings.
            min_cluster_size: Minimum cluster size for HDBSCAN.
            min_samples: Minimum samples for HDBSCAN core points.
            umap_n_neighbors: UMAP neighborhood size (None to skip UMAP).
            umap_n_components: UMAP target dimensions.
            umap_min_dist: UMAP minimum distance parameter.
            hdbscan_cluster_selection_epsilon: HDBSCAN epsilon parameter.
            hdbscan_cluster_selection_method: HDBSCAN selection method (eom/leaf).
            hdbscan_allow_single_cluster: Allow single cluster result.

        Returns:
            ClusterResult with labels, probabilities, and quality scores.
        """
        ...

    def optimize_clustering(
        self,
        embeddings: np.ndarray,
        *,
        min_cluster_size_range: list[int] | None = None,
        min_samples_range: list[int | None] | None = None,
        umap_n_neighbors_range: list[int | None] | None = None,
        umap_n_components_range: list[int | None] | None = None,
        hdbscan_cluster_selection_method: str = "eom",
        hdbscan_allow_single_cluster: bool = False,
        token_counts: np.ndarray | None = None,
    ) -> ClusterResult:
        """Hyperparameter search for optimal clustering configuration.

        Args:
            embeddings: (N, D) float32 array.
            min_cluster_size_range: Candidate min_cluster_size values.
            min_samples_range: Candidate min_samples values.
            umap_n_neighbors_range: Candidate UMAP n_neighbors values.
            umap_n_components_range: Candidate UMAP n_components values.
            hdbscan_cluster_selection_method: Selection method for all trials.
            hdbscan_allow_single_cluster: Allow single cluster for all trials.
            token_counts: Per-sentence token counts for recursive splitting.

        Returns:
            Best ClusterResult found during search.
        """
        ...

    def subcluster_other(
        self,
        embeddings: np.ndarray,
        token_counts: np.ndarray | None = None,
    ) -> ClusterResult:
        """Specialized clustering for 'Other' genre with deeper search.

        Args:
            embeddings: (N, D) float32 array.
            token_counts: Per-sentence token counts for recursive splitting.

        Returns:
            ClusterResult optimized for heterogeneous content.
        """
        ...

    def recursive_cluster(
        self,
        embeddings: np.ndarray,
        labels: np.ndarray,
        probabilities: np.ndarray,
        token_counts: np.ndarray,
    ) -> tuple[np.ndarray, np.ndarray]:
        """Recursively split clusters exceeding the max token budget.

        Args:
            embeddings: (N, D) float32 array.
            labels: (N,) int array of cluster IDs.
            probabilities: (N,) float array.
            token_counts: (N,) int array of per-sentence token estimates.

        Returns:
            (new_labels, new_probabilities) after recursive splitting.
        """
        ...
