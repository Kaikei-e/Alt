"""HDBSCAN clusterer gateway implementing ClustererPort.

Delegates to the existing services/clusterer.py Clusterer class and
re-exports ClusterResult from the port layer for unified typing.
The original Clusterer class retains all UMAP, HDBSCAN, Optuna,
noise reclustering, and recursive splitting logic.
"""

from __future__ import annotations

import numpy as np

from ..infra.config import Settings
from ..port.clusterer import ClusterResult
from ..services.clusterer import Clusterer


class HdbscanClustererGateway:
    """Gateway wrapping the existing Clusterer for ClustererPort compliance."""

    def __init__(self, settings: Settings) -> None:
        self._clusterer = Clusterer(settings)

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
        result = self._clusterer.cluster(
            embeddings,
            min_cluster_size=min_cluster_size,
            min_samples=min_samples,
            umap_n_neighbors=umap_n_neighbors,
            umap_n_components=umap_n_components,
            umap_min_dist=umap_min_dist,
            hdbscan_cluster_selection_epsilon=hdbscan_cluster_selection_epsilon,
            hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
            hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
        )
        return ClusterResult(
            labels=result.labels,
            probabilities=result.probabilities,
            used_umap=result.used_umap,
            params=result.params,
            dbcv_score=result.dbcv_score,
            silhouette_score=result.silhouette_score,
            used_fallback=result.used_fallback,
        )

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
        result = self._clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=min_cluster_size_range,
            min_samples_range=min_samples_range,
            umap_n_neighbors_range=umap_n_neighbors_range,
            umap_n_components_range=umap_n_components_range,
            hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
            hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
            token_counts=token_counts,
        )
        return ClusterResult(
            labels=result.labels,
            probabilities=result.probabilities,
            used_umap=result.used_umap,
            params=result.params,
            dbcv_score=result.dbcv_score,
            silhouette_score=result.silhouette_score,
            used_fallback=result.used_fallback,
        )

    def subcluster_other(
        self,
        embeddings: np.ndarray,
        token_counts: np.ndarray | None = None,
    ) -> ClusterResult:
        result = self._clusterer.subcluster_other(embeddings, token_counts=token_counts)
        return ClusterResult(
            labels=result.labels,
            probabilities=result.probabilities,
            used_umap=result.used_umap,
            params=result.params,
            dbcv_score=result.dbcv_score,
            silhouette_score=result.silhouette_score,
            used_fallback=result.used_fallback,
        )

    def recursive_cluster(
        self,
        embeddings: np.ndarray,
        labels: np.ndarray,
        probabilities: np.ndarray,
        token_counts: np.ndarray,
    ) -> tuple[np.ndarray, np.ndarray]:
        return self._clusterer.recursive_cluster(
            embeddings, labels, probabilities, token_counts
        )
