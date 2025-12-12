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

        # Noise reclustering: attempt to cluster noise points (-1) using KMeans
        if self.settings.noise_recluster_enabled:
            noise_mask = labels == -1
            n_noise = noise_mask.sum()

            if n_noise >= self.settings.noise_recluster_min_points:
                noise_embeddings = reduced[noise_mask]

                # Determine optimal number of clusters for noise points
                # Use silhouette score to select k
                max_k = min(
                    self.settings.noise_recluster_max_clusters,
                    n_noise // max(2, min_cluster_size)
                )

                if max_k >= 2:
                    best_k = 2
                    best_sil = -1.0

                    for k in range(2, max_k + 1):
                        try:
                            from sklearn.cluster import KMeans
                            kmeans = KMeans(n_clusters=k, random_state=42, n_init=10)
                            kmeans_labels = kmeans.fit_predict(noise_embeddings)

                            # Calculate silhouette for this k
                            if len(set(kmeans_labels)) >= 2 and len(kmeans_labels) >= 2:
                                sil = silhouette_score(noise_embeddings, kmeans_labels)
                                if sil > best_sil:
                                    best_sil = sil
                                    best_k = k
                        except Exception:
                            continue

                    # Apply best k clustering
                    if best_k >= 2:
                        try:
                            from sklearn.cluster import KMeans
                            kmeans = KMeans(n_clusters=best_k, random_state=42, n_init=10)
                            noise_labels = kmeans.fit_predict(noise_embeddings)

                            # Assign new cluster IDs (starting from max existing label + 1)
                            max_existing_label = labels.max() if labels.size > 0 else -1
                            base_id = max_existing_label + 1
                            new_noise_labels = base_id + noise_labels

                            # Update labels and probabilities
                            labels[noise_mask] = new_noise_labels
                            probs[noise_mask] = 1.0  # Hard clustering
                        except Exception:
                            pass  # If reclustering fails, keep noise as -1

        # Calculate DBCV score using the reduced space (or embeddings if UMAP not used)
        # This ensures consistency with the space HDBSCAN actually operated on
        dbcv = self._calculate_dbcv(reduced, labels)

        # Recalculate silhouette after potential noise reclustering
        sil_score = self._calculate_silhouette(reduced, labels)

        return ClusterResult(
            labels=labels,
            probabilities=probs,
            used_umap=use_umap,
            params=HDBSCANSettings(
                min_cluster_size=min_cluster_size,
                min_samples=min_samples,
            ),
            dbcv_score=dbcv,
            silhouette_score=sil_score,
        )

    def _calculate_dbcv(self, X: np.ndarray, labels: np.ndarray) -> float:
        """
        Calculate DBCV (Density-Based Clustering Validation) score using hdbscan.validity.validity_index.

        Args:
            X: Embedding space (reduced if UMAP was used, otherwise original embeddings)
            labels: Cluster labels from HDBSCAN (-1 indicates noise)

        Returns:
            DBCV score (typically in [-1, 1] range), or 0.0 if calculation fails
        """
        try:
            from hdbscan.validity import validity_index
        except ImportError:
            # Fallback if hdbscan is not available (e.g., build issues)
            return 0.0

        try:
            # Filter out noise points (-1) for DBCV calculation
            mask = labels != -1
            if mask.sum() < 2:
                return 0.0

            filtered_X = X[mask]
            filtered_labels = labels[mask]

            # DBCV requires at least 2 distinct clusters
            if len(set(filtered_labels)) < 2:
                return 0.0

            dbcv = float(validity_index(filtered_X, filtered_labels, metric='euclidean'))

            # Ensure result is finite (handle NaN/Inf)
            if not np.isfinite(dbcv):
                return 0.0

            return dbcv
        except Exception:
            # Any exception during calculation returns 0.0
            return 0.0

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
        Uses composite score (0.6*silhouette + 0.4*DBCV) to select the best configuration.
        If use_bayes_opt is True, uses Optuna for Bayesian optimization.
        """
        # Use Optuna if enabled
        if self.settings.use_bayes_opt:
            return self._optimize_clustering_optuna(
                embeddings,
                min_cluster_size_range=min_cluster_size_range,
                min_samples_range=min_samples_range,
                umap_n_neighbors_range=umap_n_neighbors_range,
                umap_n_components_range=umap_n_components_range,
                hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
                hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
                token_counts=token_counts,
            )

        # Use grid search implementation
        return self._optimize_clustering_grid(
            embeddings,
            min_cluster_size_range=min_cluster_size_range,
            min_samples_range=min_samples_range,
            umap_n_neighbors_range=umap_n_neighbors_range,
            umap_n_components_range=umap_n_components_range,
            hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
            hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
            token_counts=token_counts,
        )

    def _optimize_clustering_grid(
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
        Grid search hyperparameter optimization (original implementation).
        Extracted to allow Optuna fallback.
        """
        n_data_points = embeddings.shape[0]
        if min_cluster_size_range is None:
            min_cluster_size_range = [3, 4, 6, 8, 10, 12]
        if min_samples_range is None:
            min_samples_range = [1, 2, 4, 6]
        if umap_n_neighbors_range is None:
            umap_n_neighbors_range = [10, 15, 30]
        if umap_n_components_range is None:
            umap_n_components_range = [8]

        best_score = -2.0
        best_result = None

        # Pre-validation
        if embeddings.size == 0 or not np.isfinite(embeddings).all():
             return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        n_data_points = embeddings.shape[0]

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

                        # Optimization metric: composite score (0.6 * silhouette + 0.4 * DBCV)
                        # Both scores are typically in [-1, 1] range, so weighted sum is reasonable
                        score = 0.6 * result.silhouette_score + 0.4 * result.dbcv_score

                        # Tie-breaking logic:
                        # 1. Higher composite score (better cluster separation and density validity)
                        # 2. If equal, prefer larger min_cluster_size (more stable, fewer micro-clusters)
                        # 3. If still equal, prefer larger min_samples (better noise resistance)
                        if score > best_score:
                            best_score = score
                            best_result = result
                        elif score == best_score:
                            if best_result:
                                if mcs > best_result.params.min_cluster_size:
                                    best_result = result
                                elif mcs == best_result.params.min_cluster_size:
                                    if current_ms > best_result.params.min_samples:
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

        # Calculate dynamic thresholds if enabled
        if self.settings.recursive_dynamic_thresholds:
            # Calculate median token count per sentence (representative sentence length)
            m = float(np.median(token_counts))

            # Calculate cluster sizes (number of sentences per cluster)
            unique_labels = set(labels)
            unique_labels.discard(-1)
            cluster_sizes = []
            for lbl in unique_labels:
                mask = labels == lbl
                cluster_sizes.append(mask.sum())

            if len(cluster_sizes) > 0:
                # Representative cluster size (median)
                s50 = float(np.median(cluster_sizes))

                # Dynamic max_tokens: median_sentence_tokens * 1.5 * median_cluster_size
                # This scales with both sentence length and typical cluster size
                dynamic_max_tokens = int(m * 1.5 * s50)

                # Clamp to floor and ceiling
                max_tokens = max(
                    self.settings.recursive_max_tokens_floor,
                    min(dynamic_max_tokens, self.settings.recursive_max_tokens_ceil)
                )

                # Dynamic min_split_size: 10th percentile of cluster sizes, but at least 5
                min_split_size = max(
                    5,
                    int(np.percentile(cluster_sizes, 10)) if len(cluster_sizes) > 0 else 5
                )
            else:
                # Fallback to settings if no clusters
                max_tokens = self.settings.clustering_max_tokens_per_cluster
                min_split_size = self.settings.clustering_min_split_size
        else:
            # Use fixed thresholds from settings
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

    def _optimize_clustering_optuna(
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
        Bayesian hyperparameter optimization using Optuna (TPE sampler).
        Uses composite score (0.6*silhouette + 0.4*DBCV) as the objective.
        """
        try:
            import optuna
            from optuna.samplers import TPESampler
        except ImportError:
            # Fallback to grid search if Optuna is not available
            return self._optimize_clustering_grid(
                embeddings,
                min_cluster_size_range=min_cluster_size_range,
                min_samples_range=min_samples_range,
                umap_n_neighbors_range=umap_n_neighbors_range,
                umap_n_components_range=umap_n_components_range,
                hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
                hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
                token_counts=token_counts,
            )

        n_data_points = embeddings.shape[0]

        # Pre-validation
        if embeddings.size == 0 or not np.isfinite(embeddings).all():
            return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        # Set default ranges if not provided
        if min_cluster_size_range is None:
            min_cluster_size_range = [3, 4, 6, 8, 10, 12]
        if min_samples_range is None:
            min_samples_range = [1, 2, 4, 6]
        if umap_n_neighbors_range is None:
            umap_n_neighbors_range = [10, 15, 30]
        if umap_n_components_range is None:
            umap_n_components_range = [8]

        # Determine valid ranges
        mcs_min = min(mcs for mcs in min_cluster_size_range if mcs < n_data_points)
        mcs_max = max(mcs for mcs in min_cluster_size_range if mcs < n_data_points)
        ms_max = max(ms if ms is not None else mcs_max for ms in min_samples_range)

        # Convert UMAP ranges to lists for categorical suggestion
        umap_n_neighbors_list = [n for n in umap_n_neighbors_range if n is not None]
        umap_n_components_list = [n for n in umap_n_components_range if n is not None]

        def objective(trial):
            # Suggest hyperparameters
            mcs = trial.suggest_int('min_cluster_size', mcs_min, mcs_max)
            # Ensure min_samples <= min_cluster_size
            ms_upper = min(ms_max, mcs)
            ms = trial.suggest_int('min_samples', 1, ms_upper)

            # UMAP parameters (categorical if multiple options, otherwise fixed)
            if len(umap_n_neighbors_list) > 1:
                n_neighbors = trial.suggest_categorical('umap_n_neighbors', umap_n_neighbors_list)
            else:
                n_neighbors = umap_n_neighbors_list[0] if umap_n_neighbors_list else None

            if len(umap_n_components_list) > 1:
                n_components = trial.suggest_categorical('umap_n_components', umap_n_components_list)
            else:
                n_components = umap_n_components_list[0] if umap_n_components_list else None

            # Perform clustering
            result = self.cluster(
                embeddings,
                min_cluster_size=mcs,
                min_samples=ms,
                umap_n_neighbors=n_neighbors,
                umap_n_components=n_components,
                hdbscan_cluster_selection_epsilon=0.5,
                hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
                hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
            )

            # Return composite score (to maximize)
            return 0.6 * result.silhouette_score + 0.4 * result.dbcv_score

        # Create study with TPE sampler and seed for reproducibility
        sampler = TPESampler(seed=42)
        study = optuna.create_study(direction='maximize', sampler=sampler)

        # Optimize with timeout if specified
        timeout = self.settings.bayes_opt_timeout_seconds
        n_trials = self.settings.bayes_opt_trials

        if timeout is not None:
            study.optimize(objective, n_trials=n_trials, timeout=timeout)
        else:
            study.optimize(objective, n_trials=n_trials)

        # Get best parameters and run final clustering
        best_params = study.best_params
        best_result = self.cluster(
            embeddings,
            min_cluster_size=best_params['min_cluster_size'],
            min_samples=best_params['min_samples'],
            umap_n_neighbors=best_params.get('umap_n_neighbors'),
            umap_n_components=best_params.get('umap_n_components'),
            hdbscan_cluster_selection_epsilon=0.5,
            hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
            hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
        )

        # Recursive step to break down large clusters
        if token_counts is not None and best_result.labels.size > 0:
            new_labels, new_probs = self.recursive_cluster(
                embeddings, best_result.labels, best_result.probabilities, token_counts
            )
            best_result.labels = new_labels
            best_result.probabilities = new_probs

        return best_result

    def _optimize_clustering_grid(
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
        Grid search hyperparameter optimization (original implementation).
        Extracted to allow Optuna fallback.
        """
        n_data_points = embeddings.shape[0]
        if min_cluster_size_range is None:
            min_cluster_size_range = [3, 4, 6, 8, 10, 12]
        if min_samples_range is None:
            min_samples_range = [1, 2, 4, 6]
        if umap_n_neighbors_range is None:
            umap_n_neighbors_range = [10, 15, 30]
        if umap_n_components_range is None:
            umap_n_components_range = [8]

        best_score = -2.0
        best_result = None

        # Pre-validation
        if embeddings.size == 0 or not np.isfinite(embeddings).all():
             return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        n_data_points = embeddings.shape[0]

        for n_neighbors in umap_n_neighbors_range:
            for n_components in umap_n_components_range:
                for mcs in min_cluster_size_range:
                    if mcs >= n_data_points:
                        continue

                    for ms in min_samples_range:
                        current_ms = ms if ms is not None else mcs

                        if current_ms > mcs:
                            continue

                        result = self.cluster(
                            embeddings,
                            min_cluster_size=mcs,
                            min_samples=current_ms,
                            umap_n_neighbors=n_neighbors,
                            umap_n_components=n_components,
                            hdbscan_cluster_selection_epsilon=0.5,
                            hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
                            hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
                        )

                        score = 0.6 * result.silhouette_score + 0.4 * result.dbcv_score

                        if score > best_score:
                            best_score = score
                            best_result = result
                        elif score == best_score:
                            if best_result:
                                if mcs > best_result.params.min_cluster_size:
                                    best_result = result
                                elif mcs == best_result.params.min_cluster_size:
                                    if current_ms > best_result.params.min_samples:
                                        best_result = result

        if best_result is None:
            best_result = self.cluster(embeddings, min_cluster_size=max(3, n_data_points // 5), min_samples=1)

        if token_counts is not None and best_result.labels.size > 0:
            new_labels, new_probs = self.recursive_cluster(
                embeddings, best_result.labels, best_result.probabilities, token_counts
            )
            best_result.labels = new_labels
            best_result.probabilities = new_probs

        return best_result

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
