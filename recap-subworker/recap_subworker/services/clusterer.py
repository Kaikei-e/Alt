"""Clustering helpers wrapping UMAP and HDBSCAN."""

from __future__ import annotations

from dataclasses import dataclass

import numpy as np
from sklearn.cluster import HDBSCAN, BisectingKMeans
from sklearn.metrics import silhouette_score

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
    silhouette_score: float = 0.0


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
        hdbscan_cluster_selection_epsilon: float | None = None,
        hdbscan_cluster_selection_method: str | None = None,
        hdbscan_allow_single_cluster: bool | None = None,
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

        # Force UMAP if enabled, otherwise use threshold-based auto-enable
        use_umap = bool(
            self.settings.enable_umap_force
            or (
                self.settings.enable_umap_auto
                and embeddings.shape[0] >= self.settings.umap_threshold_sentences
            )
        )
        reduced = embeddings
        if use_umap:
            from umap import UMAP  # lazy import

            n_data_points = embeddings.shape[0]
            requested_n_neighbors = umap_n_neighbors or self.settings.umap_n_neighbors
            # UMAP requires n_neighbors < N (number of data points)
            # Adjust n_neighbors to be at most N-1, and at least 2 for meaningful results
            adjusted_n_neighbors = max(2, min(requested_n_neighbors, n_data_points - 1))

            # If we have very few data points, skip UMAP to avoid issues
            if n_data_points < 3:
                use_umap = False
            else:
                reducer = UMAP(
                    n_components=umap_n_components or self.settings.umap_n_components,
                    n_neighbors=adjusted_n_neighbors,
                    metric="cosine",
                    min_dist=umap_min_dist or self.settings.umap_min_dist,
                    random_state=42,  # reproducible
                    n_jobs=1,
                )
                reduced = reducer.fit_transform(embeddings)

        # HDBSCAN (using sklearn.cluster.HDBSCAN)
        clusterer = HDBSCAN(
            min_cluster_size=min_cluster_size if min_cluster_size > 0 else self.settings.hdbscan_min_cluster_size,
            min_samples=min_samples if min_samples > 0 else self.settings.hdbscan_min_samples,
            metric="euclidean" if use_umap else "euclidean",
            cluster_selection_epsilon=hdbscan_cluster_selection_epsilon if hdbscan_cluster_selection_epsilon is not None else 0.0,
            allow_single_cluster=hdbscan_allow_single_cluster if hdbscan_allow_single_cluster is not None else False,
            cluster_selection_method=hdbscan_cluster_selection_method or self.settings.hdbscan_cluster_selection_method,
        )
        clusterer.fit(reduced)
        labels = clusterer.labels_
        probs = clusterer.probabilities_
        if (labels >= 0).sum() == 0:
            labels = np.arange(embeddings.shape[0], dtype=int)
            probs = np.ones_like(labels, dtype=float)
            use_umap = False

        # sklearn.cluster.HDBSCAN does not provide relative_validity_ (DBCV score)
        # Set to 0.0 as a placeholder
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
            silhouette_score=self._calculate_silhouette(embeddings, labels),
        )

    def _calculate_silhouette(self, embeddings: np.ndarray, labels: np.ndarray) -> float:
        try:
            # Silhouette score requires at least 2 distinct labels
            unique_labels = set(labels)
            # Filter out noise points (-1) for silhouette calculation
            # This is a common practice as noise points don't belong to any cluster
            # and can skew the score.
            non_noise_indices = labels != -1
            filtered_embeddings = embeddings[non_noise_indices]
            filtered_labels = labels[non_noise_indices]

            if len(set(filtered_labels)) < 2 or len(filtered_labels) < 2:
                return 0.0

            return float(silhouette_score(filtered_embeddings, filtered_labels))
        except Exception:
            return 0.0

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
        """
        Hyperparameter search for HDBSCAN (and UMAP).
        Uses silhouette score to select the best configuration.
        """
        n_data_points = embeddings.shape[0]
        if min_cluster_size_range is None:
            # Plan: [4, 6, 8, 10, 12]. Adjusted slightly to include 3 for smaller datasets per docs/heuristics.
            min_cluster_size_range = [3, 4, 6, 8, 10, 12]
        if min_samples_range is None:
            # Plan: [1, 2, 4, 6]. None usually defaults to min_cluster_size, but we want explicit control.
            min_samples_range = [1, 2, 4, 6]
        if umap_n_neighbors_range is None:
            umap_n_neighbors_range = [10, 15, 30]
        if umap_n_components_range is None:
            umap_n_components_range = [8]

        best_score = -2.0  # Silhouette score ranges from -1 to 1, start lower
        best_result = None

        # Pre-validation
        if embeddings.size == 0 or not np.isfinite(embeddings).all():
             return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        n_data_points = embeddings.shape[0]

        # 1. Determine which UMAP configs to run (neighbors x components)
        # We run UMAP first for each config, then run HDBSCAN variations on the *reduced* data.

        for n_neighbors in umap_n_neighbors_range:
            for n_components in umap_n_components_range:
                # Run HDBSCAN grid on this UMAP output

                # To optimize: In a real high-perf scenario we'd cache the UMAP result here.
                # Since 'cluster' method encapsulates UMAP + HDBSCAN, we are re-running UMAP.
                # However, UMAP is stochastic unless random_state is fixed (which it is in our code).
                # For now, we accept the overhead as we move to Rust later, or we could refactor.
                # Refactoring `cluster` to accept `pre_reduced` would be cleaner but let's stick to the interface for now
                # to avoid breaking changes in other calls. We rely on the fact that this is "optimization" mode.

                for mcs in min_cluster_size_range:
                    # Filter mcs based on dataset size
                    if mcs >= n_data_points:
                        continue

                    for ms in min_samples_range:
                        # ms can be None -> same as mcs. But our list has ints.
                        current_ms = ms if ms is not None else mcs

                        # Skip invalid
                        if current_ms > mcs:
                            continue

                        # Perform clustering
                        result = self.cluster(
                            embeddings,
                            min_cluster_size=mcs,
                            min_samples=current_ms,
                            umap_n_neighbors=n_neighbors,
                            umap_n_components=n_components,
                            # Plan item 3: epsilon=0.5 for merging close clusters
                            hdbscan_cluster_selection_epsilon=0.5,
                            hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
                            hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
                        )

                        # Optimization metric: silhouette score
                        # Note: sklearn.cluster.HDBSCAN does not provide DBCV (relative_validity_),
                        # so we use silhouette score instead for parameter optimization
                        score = result.silhouette_score

                        # Tie-breaking logic:
                        # 1. Higher silhouette score (better cluster separation)
                        # 2. If equal, prefer larger min_cluster_size (more stable, fewer micro-clusters)
                        if score > best_score:
                            best_score = score
                            best_result = result
                        elif score == best_score:
                            if best_result and mcs > best_result.params.min_cluster_size:
                                best_result = result

        if best_result is None:
            # Fallback for very small data or failed searches
            best_result = self.cluster(embeddings, min_cluster_size=max(3, n_data_points // 5), min_samples=1)

        # Recursive step to break down large clusters
        if token_counts is not None and best_result.labels.size > 0:
            new_labels, new_probs = self.recursive_cluster(
                embeddings, best_result.labels, best_result.probabilities, token_counts
            )
            best_result.labels = new_labels
            best_result.probabilities = new_probs

        return best_result

    def recursive_cluster(
        self,
        embeddings: np.ndarray,
        labels: np.ndarray,
        probabilities: np.ndarray,
        token_counts: np.ndarray,
    ) -> tuple[np.ndarray, np.ndarray]:
        """
        Recursively split clusters that exceed the max token budget.

        Args:
            embeddings: (N, D) float array
            labels: (N,) int array of cluster IDs
            probabilities: (N,) float array
            token_counts: (N,) int array of token estimates per sentence

        Returns:
            (new_labels, new_probabilities)
        """
        if not self.settings.clustering_recursive_enabled:
            return labels, probabilities

        max_tokens = self.settings.clustering_max_tokens_per_cluster
        min_split_size = self.settings.clustering_min_split_size

        # Working copies
        current_labels = labels.copy()
        current_probs = probabilities.copy()

        # Queue of cluster IDs to check: only non-noise clusters
        # We use a set to avoid re-checking just-split clusters immediately unless necessary,
        # but a simple iterative approach over unique labels is safer to prevent infinite loops.
        # However, for true recursion, we can use a stack or just loop until stable.
        # To avoid infinite loops, we'll limit depth or passes.

        # Multi-pass approach:
        # Pass 1: Check all initial clusters.
        # Pass 2: Check newly created clusters.
        # ...
        # Limit to max 3 passes to prevent excessive fragmentation.

        for _pass in range(3):
            unique_labels = set(np.unique(current_labels))
            unique_labels.discard(-1)

            splits_performed = 0

            # Sort labels to process deterministic order
            for cluster_id in sorted(unique_labels):
                mask = current_labels == cluster_id
                cluster_size = mask.sum()

                if cluster_size < min_split_size:
                    continue

                cluster_tokens = token_counts[mask].sum()

                if cluster_tokens > max_tokens:
                    # Time to split!
                    sub_embeddings = embeddings[mask]

                    # Bisect into 2
                    splitter = BisectingKMeans(
                        n_clusters=2,
                        random_state=42,
                        bisecting_strategy="largest_cluster"
                    )
                    sub_labels = splitter.fit_predict(sub_embeddings)

                    # New labels:
                    # 0 -> stays as cluster_id
                    # 1 -> gets a new ID (max_label + 1)
                    # Note: We need to be careful not to reuse an existing ID.

                    new_id = current_labels.max() + 1

                    # Map sub_labels to real labels
                    # sub_label 0 => cluster_id
                    # sub_label 1 => new_id

                    # Create the new label array fragment
                    new_fragment = np.where(sub_labels == 1, new_id, cluster_id)

                    # Update main arrays
                    current_labels[mask] = new_fragment

                    # K-Means is "hard" clustering, so probability is effectively 1.0 for these items
                    # This overrides HDBSCAN's soft probability
                    current_probs[mask] = 1.0

                    splits_performed += 1

            if splits_performed == 0:
                break

        return current_labels, current_probs

    def subcluster_other(self, embeddings: np.ndarray, token_counts: np.ndarray | None = None) -> ClusterResult:
        """
        Specialized clustering for 'Other' genre.
        Deep search with smaller parameters to break down large blobs.
        """
        result = self.optimize_clustering(
            embeddings,
            min_cluster_size_range=[3, 4, 5],
            min_samples_range=[1, 2],
            umap_n_neighbors_range=[10, 15],
            umap_n_components_range=[5, 8],
            hdbscan_cluster_selection_method="leaf",
            hdbscan_allow_single_cluster=True,
        )

        if token_counts is not None and result.labels.size > 0:
            new_labels, new_probs = self.recursive_cluster(
                embeddings, result.labels, result.probabilities, token_counts
            )
            result.labels = new_labels
            result.probabilities = new_probs

        return result
