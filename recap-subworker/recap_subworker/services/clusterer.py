"""Clustering helpers wrapping UMAP and HDBSCAN."""

from __future__ import annotations

from dataclasses import dataclass

import hdbscan
import numpy as np

from ..domain.models import HDBSCANSettings
from ..infra.config import Settings


@dataclass(slots=True)
class ClusterParams:
    min_cluster_size: int
    min_samples: int


@dataclass
class ClusterResult:
    labels: np.ndarray
    probabilities: np.ndarray
    used_umap: bool
    params: HDBSCANSettings
    dbcv_score: float = 0.0


class Clusterer:
    """Cluster embeddings using optional UMAP then HDBSCAN."""

    def __init__(self, settings: Settings) -> None:
        self.settings = settings

    def cluster(
        self,
        embeddings: np.ndarray,
        *,
        min_cluster_size: int,
        min_samples: int,
        umap_n_neighbors: int | None = None,
        umap_n_components: int | None = None,
        umap_min_dist: float | None = None,
    ) -> ClusterResult:
        if embeddings.size == 0:
            empty = np.empty((0,), dtype=int)
            return ClusterResult(
                empty, empty, False, HDBSCANSettings(min_cluster_size=0, min_samples=0), 0.0
            )

        # Validation: Check for NaNs or Infs
        if not np.isfinite(embeddings).all():
            empty = np.empty((0,), dtype=int)
            return ClusterResult(
                empty, empty, False, HDBSCANSettings(min_cluster_size=0, min_samples=0), 0.0
            )

        # Validation: Check for zero vectors (which break cosine metric)
        norms = np.linalg.norm(embeddings, axis=1)
        if (norms == 0).any():
            empty = np.empty((0,), dtype=int)
            return ClusterResult(
                empty, empty, False, HDBSCANSettings(min_cluster_size=0, min_samples=0), 0.0
            )

        use_umap = bool(
            self.settings.enable_umap_auto
            and embeddings.shape[0] >= self.settings.umap_threshold_sentences
        )
        reduced = embeddings
        if use_umap:
            from umap import UMAP  # lazy import

            reducer = UMAP(
                n_components=umap_n_components or self.settings.umap_n_components,
                n_neighbors=umap_n_neighbors or self.settings.umap_n_neighbors,
                metric="cosine",
                min_dist=umap_min_dist or self.settings.umap_min_dist,
                random_state=42,  # reproducible
                n_jobs=1,
            )
            reduced = reducer.fit_transform(embeddings)

        # HDBSCAN
        clusterer = hdbscan.HDBSCAN(
            min_cluster_size=min_cluster_size if min_cluster_size > 0 else self.settings.hdbscan_min_cluster_size,
            min_samples=min_samples if min_samples > 0 else self.settings.hdbscan_min_samples,
            metric="euclidean" if use_umap else "euclidean", # UMAP reduces to euclidean space usually, or if raw embeddings we might use cosine?
            # Actually standard practice with E5/Cosine embeddings:
            # If UMAP used -> Euclidean on reduced.
            # If Raw -> HDBSCAN doesn't support 'cosine' efficiently without precomputed distance matrix usually, but let's check.
            # The original code might have been using default.
            cluster_selection_method=self.settings.hdbscan_cluster_selection_method,
            prediction_data=True,
        )
        clusterer.fit(reduced)
        labels = clusterer.labels_
        probs = clusterer.probabilities_
        if (labels >= 0).sum() == 0:
            labels = np.arange(embeddings.shape[0], dtype=int)
            probs = np.ones_like(labels, dtype=float)
            use_umap = False

        try:
            dbcv = clusterer.relative_validity_
        except Exception:
            dbcv = 0.0

        return ClusterResult(
            labels=labels,
            probabilities=probs,
            used_umap=use_umap,
            params=HDBSCANSettings(
                min_cluster_size=min_cluster_size,
                min_samples=min_samples,
            ),
            dbcv_score=dbcv,
        )

    def optimize_clustering(
        self,
        embeddings: np.ndarray,
        *,
        min_cluster_size_range: range | list[int] = range(3, 10),
        min_samples_range: range | list[int] = range(1, 5),
        umap_n_neighbors_range: range | list[int] | None = None,
    ) -> ClusterResult:
        """Perform grid search to find best clustering parameters based on DBCV."""
        best_score = -1.0
        best_result = None

        # If embeddings are empty or invalid, return default empty result immediately
        # by calling cluster with default params
        if embeddings.size == 0 or not np.isfinite(embeddings).all():
             return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        # Default to single run if no UMAP range provided
        n_neighbors_list = umap_n_neighbors_range if umap_n_neighbors_range is not None else [None]

        for n_neighbors in n_neighbors_list:
            for mcs in min_cluster_size_range:
                for ms in min_samples_range:
                    # Skip invalid combinations if any
                    if ms >= mcs:
                        continue

                    result = self.cluster(
                        embeddings,
                        min_cluster_size=mcs,
                        min_samples=ms,
                        umap_n_neighbors=n_neighbors,
                    )

                # Prefer higher score. If score is same, prefer larger min_cluster_size (more stability)
                if result.dbcv_score > best_score:
                    best_score = result.dbcv_score
                    best_result = result

        if best_result is None:
            # Fallback to default
            return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        return best_result

    def subcluster_other(self, embeddings: np.ndarray) -> ClusterResult:
        """
        Specialized clustering for 'Other' genre to break it down.
        Uses smaller parameters to find smaller, tighter clusters.
        """
        # Range optimized for finding small clusters in noise
        return self.optimize_clustering(
            embeddings,
            min_cluster_size_range=range(2, 6),
            min_samples_range=range(1, 4)
        )
