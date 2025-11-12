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
                empty, empty, False, HDBSCANSettings(min_cluster_size=0, min_samples=0)
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
        )
        clusterer.fit(reduced)
        labels = clusterer.labels_
        probs = clusterer.probabilities_
        if (labels >= 0).sum() == 0:
            labels = np.arange(embeddings.shape[0], dtype=int)
            probs = np.ones_like(labels, dtype=float)
            use_umap = False

        return ClusterResult(
            labels=labels,
            probabilities=probs,
            used_umap=use_umap,
            params=HDBSCANSettings(
                min_cluster_size=min_cluster_size,
                min_samples=min_samples,
            ),
        )
