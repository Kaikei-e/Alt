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
                n_neighbors=30,
                min_dist=0.0,
                n_components=min(embeddings.shape[1], 50),
                metric="cosine",
                random_state=42,
            )
            reduced = reducer.fit_transform(embeddings)

        clusterer = hdbscan.HDBSCAN(
            min_cluster_size=max(2, min_cluster_size),
            min_samples=max(1, min_samples),
            metric="euclidean",
            cluster_selection_method="eom",
            gen_min_span_tree=True,
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
    ) -> ClusterResult:
        """Perform grid search to find best clustering parameters based on DBCV."""
        best_score = -1.0
        best_result = None

        # If embeddings are empty or invalid, return default empty result immediately
        # by calling cluster with default params
        if embeddings.size == 0 or not np.isfinite(embeddings).all():
             return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        for mcs in min_cluster_size_range:
            for ms in min_samples_range:
                # Skip invalid combinations if any
                if ms >= mcs:
                    continue

                result = self.cluster(
                    embeddings,
                    min_cluster_size=mcs,
                    min_samples=ms,
                )

                # Prefer higher score. If score is same, prefer larger min_cluster_size (more stability)
                if result.dbcv_score > best_score:
                    best_score = result.dbcv_score
                    best_result = result

        if best_result is None:
            # Fallback to default
            return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        return best_result
