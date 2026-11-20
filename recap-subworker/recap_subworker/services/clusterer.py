"""Clustering helpers wrapping UMAP and HDBSCAN."""

from __future__ import annotations

import concurrent.futures
import threading
import time
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from typing import TYPE_CHECKING, NamedTuple

import numpy as np
import structlog
from sklearn.cluster import HDBSCAN, BisectingKMeans, KMeans, MiniBatchKMeans
from sklearn.metrics import silhouette_score

from ..domain.models import HDBSCANSettings
from ..infra.config import Settings

if TYPE_CHECKING:
    import optuna

_LOGGER = structlog.get_logger(__name__)
_MIN_CLUSTER_COUNT = 2
_MIN_UMAP_POINTS = 3
_SMALL_DATASET_SAMPLE_THRESHOLD = 100

_NATIVE_CLUSTERING_LOCK = threading.Lock()
_ABANDONED_HDBSCAN_FUTURE: concurrent.futures.Future | None = None
_ABANDONED_HDBSCAN_LOCK = threading.Lock()


def _set_abandoned_hdbscan_future(future: concurrent.futures.Future) -> None:
    with _ABANDONED_HDBSCAN_LOCK:
        global _ABANDONED_HDBSCAN_FUTURE  # noqa: PLW0603 - per-process singleton tracking abandoned HDBSCAN future
        _ABANDONED_HDBSCAN_FUTURE = future

    def _on_done(fut: concurrent.futures.Future) -> None:
        with _ABANDONED_HDBSCAN_LOCK:
            global _ABANDONED_HDBSCAN_FUTURE  # noqa: PLW0603 - per-process singleton tracking abandoned HDBSCAN future
            if _ABANDONED_HDBSCAN_FUTURE is fut:
                _ABANDONED_HDBSCAN_FUTURE = None

    future.add_done_callback(_on_done)


def _is_abandoned_hdbscan_running() -> bool:
    with _ABANDONED_HDBSCAN_LOCK:
        global _ABANDONED_HDBSCAN_FUTURE  # noqa: PLW0603 - per-process singleton tracking abandoned HDBSCAN future
        if _ABANDONED_HDBSCAN_FUTURE is not None:
            if not _ABANDONED_HDBSCAN_FUTURE.done():
                return True
            _ABANDONED_HDBSCAN_FUTURE = None
    return False


@contextmanager
def _guard_native_clustering() -> Iterator[None]:
    # WHY: Held by the waiting thread across the native section so timeouts release
    # cleanly and abandoned background threads cannot acquire or deadlock the lock.
    wait_start = time.monotonic()
    if not _NATIVE_CLUSTERING_LOCK.acquire(blocking=False):
        _LOGGER.debug(
            "clustering_lock.waiting",
            message="Waiting for native clustering lock",
        )
        _NATIVE_CLUSTERING_LOCK.acquire()
        wait_duration = time.monotonic() - wait_start
        if wait_duration > 1.0:
            _LOGGER.info(
                "clustering_lock.acquired",
                wait_duration_seconds=wait_duration,
                message="Native clustering lock acquired after wait exceeding 1 s",
            )
    try:
        yield
    finally:
        _NATIVE_CLUSTERING_LOCK.release()


def compute_knn_faiss(embeddings: np.ndarray, n_neighbors: int) -> tuple[np.ndarray, np.ndarray]:
    """
    Compute k-nearest neighbors using FAISS instead of pynndescent.

    This avoids the integer overflow bug in pynndescent 0.6.0.

    Args:
        embeddings: (N, D) float32 array of embeddings
        n_neighbors: Number of neighbors to find

    Returns:
        (knn_indices, knn_dists): Arrays of shape (N, n_neighbors)
    """
    import faiss  # noqa: PLC0415 - heavy faiss/OpenMP import deferred until k-NN is invoked

    _n_samples, dim = embeddings.shape

    # Ensure float32 for FAISS
    embeddings_f32 = embeddings.astype(np.float32)

    # Copy to avoid modifying original (normalize_L2 is in-place)
    embeddings_norm = embeddings_f32.copy()

    # L2 normalize for cosine similarity via inner product
    faiss.normalize_L2(embeddings_norm)

    # Use IndexFlatIP (inner product = cosine similarity after normalization)
    index = faiss.IndexFlatIP(dim)
    index.add(embeddings_norm)

    # Search (includes self as nearest neighbor)
    similarities, indices = index.search(embeddings_norm, n_neighbors)

    # Convert similarity to distance (1 - similarity)
    # Clip to ensure non-negative distances
    distances = np.clip(1.0 - similarities, 0.0, 2.0)

    return indices, distances


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
    dbcv_score: float | None = None
    silhouette_score: float | None = None
    used_fallback: bool = False  # True if MiniBatchKMeans fallback was used


@dataclass(frozen=True, slots=True)
class _OptimizeConfig:
    """Search configuration passed from optimize_clustering to grid/Optuna backends."""

    min_cluster_size_range: list[int] | None = None
    min_samples_range: list[int | None] | None = None
    umap_n_neighbors_range: list[int | None] | None = None
    umap_n_components_range: list[int | None] | None = None
    hdbscan_cluster_selection_method: str = "eom"
    hdbscan_allow_single_cluster: bool = False
    token_counts: np.ndarray | None = None


@dataclass(frozen=True, slots=True)
class _ResolvedSearchRanges:
    """Concrete hyperparameter candidate lists with defaults applied."""

    min_cluster_size_range: list[int]
    min_samples_range: list[int | None]
    umap_n_neighbors_range: list[int | None]
    umap_n_components_range: list[int | None]


@dataclass(frozen=True, slots=True)
class _GridCandidate:
    """Single hyperparameter combination evaluated during grid search."""

    n_neighbors: int | None
    n_components: int | None
    min_cluster_size: int
    min_samples: int


class _BestGridResult(NamedTuple):
    score: float
    result: ClusterResult | None


class _GridEvaluation(NamedTuple):
    result: ClusterResult
    score: float
    min_cluster_size: int
    min_samples: int


@dataclass(frozen=True, slots=True)
class _OptunaSearchSpace:
    """Bounded Optuna parameter ranges derived from dataset size."""

    mcs_min: int
    mcs_max: int
    ms_max: int
    umap_n_neighbors_list: list[int]
    umap_n_components_list: list[int]


def _build_optuna_search_space(
    n_data_points: int,
    ranges: _ResolvedSearchRanges,
) -> _OptunaSearchSpace:
    """Compute bounded Optuna search ranges with fallbacks for small datasets."""
    valid_mcs = [mcs for mcs in ranges.min_cluster_size_range if mcs < n_data_points]
    if not valid_mcs:
        mcs_min = max(2, n_data_points // 10)
        mcs_max = max(mcs_min, n_data_points // 4)
        _LOGGER.warning(
            "min_cluster_size_range_adjusted",
            original_range=ranges.min_cluster_size_range,
            n_data_points=n_data_points,
            adjusted_min=mcs_min,
            adjusted_max=mcs_max,
        )
    else:
        mcs_min = min(valid_mcs)
        mcs_max = max(valid_mcs)
    ms_max = max(ms if ms is not None else mcs_max for ms in ranges.min_samples_range)

    # UMAP pynndescent is unstable when n_neighbors is close to dataset size
    max_safe_neighbors = max(2, n_data_points // 3)
    umap_n_neighbors_list = [
        n for n in ranges.umap_n_neighbors_range if n is not None and n <= max_safe_neighbors
    ]
    if not umap_n_neighbors_list:
        umap_n_neighbors_list = [max_safe_neighbors]
        _LOGGER.info(
            "umap_n_neighbors_adjusted",
            original_range=ranges.umap_n_neighbors_range,
            n_data_points=n_data_points,
            safe_limit=max_safe_neighbors,
        )
    umap_n_components_list = [n for n in ranges.umap_n_components_range if n is not None]
    return _OptunaSearchSpace(
        mcs_min=mcs_min,
        mcs_max=mcs_max,
        ms_max=ms_max,
        umap_n_neighbors_list=umap_n_neighbors_list,
        umap_n_components_list=umap_n_components_list,
    )


def _suggest_optuna_umap_param(
    trial: optuna.Trial,
    name: str,
    choices: list[int],
) -> int | None:
    """Suggest a categorical UMAP parameter when multiple choices exist, else return fixed value."""
    if len(choices) > 1:
        return trial.suggest_categorical(name, choices)
    return choices[0] if choices else None


@dataclass(frozen=True, slots=True)
class _UmapOptions:
    """Optional per-call UMAP hyperparameter overrides."""

    n_neighbors: int | None
    n_components: int | None
    min_dist: float | None


@dataclass(frozen=True, slots=True)
class _HdbscanOptions:
    """Per-call HDBSCAN parameters passed into fallback-guarded execution."""

    min_cluster_size: int
    min_samples: int
    cluster_selection_epsilon: float | None
    cluster_selection_method: str | None
    allow_single_cluster: bool | None


class _HdbscanRunOutcome(NamedTuple):
    labels: np.ndarray
    probs: np.ndarray
    used_umap: bool
    used_fallback: bool


def _empty_cluster_result() -> ClusterResult:
    """Return a zero-length ClusterResult for invalid or empty input."""
    empty = np.empty((0,), dtype=int)
    return ClusterResult(
        empty, empty, False, HDBSCANSettings(min_cluster_size=0, min_samples=0), 0.0
    )


def _validate_cluster_input(embeddings: np.ndarray) -> ClusterResult | None:
    """Return an empty ClusterResult if embeddings are empty, non-finite, or have zero norms."""
    if embeddings.size == 0 or not np.isfinite(embeddings).all():
        return _empty_cluster_result()
    if (np.linalg.norm(embeddings, axis=1) == 0).any():
        return _empty_cluster_result()
    return None


def _find_best_noise_k(noise_embeddings: np.ndarray, max_k: int) -> int:
    """Select the KMeans cluster count for noise points maximizing silhouette score."""
    best_k = _MIN_CLUSTER_COUNT
    best_sil = -1.0

    for k in range(2, max_k + 1):
        try:
            kmeans = KMeans(n_clusters=k, random_state=42, n_init=10)
            kmeans_labels = kmeans.fit_predict(noise_embeddings)

            if (
                len(set(kmeans_labels)) >= _MIN_CLUSTER_COUNT
                and len(kmeans_labels) >= _MIN_CLUSTER_COUNT
            ):
                sil = silhouette_score(noise_embeddings, kmeans_labels)
                if sil > best_sil:
                    best_sil = sil
                    best_k = k
        except (ValueError, RuntimeError) as exc:
            _LOGGER.debug(
                "noise_recluster.kmeans_k_search_failed",
                k=k,
                error=str(exc),
            )
            continue

    return best_k


def _resolve_search_ranges(config: _OptimizeConfig) -> _ResolvedSearchRanges:
    """Apply default search ranges where callers passed None."""
    return _ResolvedSearchRanges(
        min_cluster_size_range=(
            config.min_cluster_size_range
            if config.min_cluster_size_range is not None
            else [3, 4, 6, 8, 10, 12]
        ),
        min_samples_range=(
            config.min_samples_range if config.min_samples_range is not None else [1, 2, 4, 6]
        ),
        umap_n_neighbors_range=(
            config.umap_n_neighbors_range
            if config.umap_n_neighbors_range is not None
            else [10, 15, 30]
        ),
        umap_n_components_range=(
            config.umap_n_components_range if config.umap_n_components_range is not None else [8]
        ),
    )


def _is_invalid_optimization_input(embeddings: np.ndarray) -> bool:
    """Return True when embeddings cannot be searched for cluster hyperparameters."""
    return bool(
        embeddings.size == 0
        or embeddings.shape[0] < _MIN_CLUSTER_COUNT
        or not np.isfinite(embeddings).all()
    )


def _composite_score(result: ClusterResult) -> float:
    """Compute weighted clustering quality score (0.6*silhouette + 0.4*DBCV)."""
    return 0.6 * (result.silhouette_score or 0.0) + 0.4 * (result.dbcv_score or 0.0)


def _select_better_grid_result(
    best: _BestGridResult,
    candidate: _GridEvaluation,
) -> _BestGridResult:
    """Select the higher-scoring grid result, breaking exact ties by larger mcs then ms."""
    if candidate.score > best.score:
        return _BestGridResult(score=candidate.score, result=candidate.result)
    if candidate.score == best.score and best.result:
        better_mcs = candidate.min_cluster_size > best.result.params.min_cluster_size
        same_mcs_better_ms = (
            candidate.min_cluster_size == best.result.params.min_cluster_size
            and candidate.min_samples > best.result.params.min_samples
        )
        if better_mcs or same_mcs_better_ms:
            return _BestGridResult(score=best.score, result=candidate.result)
    return best


class Clusterer:
    """Cluster embeddings using optional UMAP then HDBSCAN."""

    def __init__(self, settings: Settings) -> None:
        self.settings = settings

    def _run_with_timeout(
        self,
        func: Callable[[], tuple[np.ndarray, np.ndarray]],
        timeout_seconds: float | int,
        *,
        track_abandoned: bool = False,
    ) -> tuple[np.ndarray, np.ndarray] | None:
        """
        Run a function with a timeout.

        Args:
            func: Callable returning (labels, probabilities)
            timeout_seconds: Timeout in seconds
            track_abandoned: Whether to track the future if the function times out

        Returns:
            (labels, probabilities) or None if timeout occurred

        Note:
            On timeout this does NOT wait for the runaway HDBSCAN thread to
            finish. A plain ``with ThreadPoolExecutor(...) as executor:``
            block calls ``executor.shutdown(wait=True)`` on exit regardless
            of what happened inside, which would block the caller until the
            thread that just timed out actually completes — defeating the
            purpose of the timeout. We shut down with ``wait=False`` instead;
            the orphaned worker thread keeps running in the background until
            it finishes naturally (or the process exits).
        """
        executor = concurrent.futures.ThreadPoolExecutor(max_workers=1)
        future = executor.submit(func)
        try:
            result = future.result(timeout=timeout_seconds)
        except concurrent.futures.TimeoutError:
            _LOGGER.warning(
                "hdbscan_timeout",
                timeout_seconds=timeout_seconds,
                message="HDBSCAN clustering timed out, will use MiniBatchKMeans fallback",
            )
            if track_abandoned:
                _set_abandoned_hdbscan_future(future)
            executor.shutdown(wait=False, cancel_futures=True)
            return None
        executor.shutdown(wait=False)
        return result

    def _fallback_minibatch_kmeans(
        self,
        embeddings: np.ndarray,
        n_clusters: int | None = None,
    ) -> tuple[np.ndarray, np.ndarray]:
        """
        Fallback clustering using MiniBatchKMeans when HDBSCAN times out.

        MiniBatchKMeans is much faster than standard KMeans and HDBSCAN,
        making it suitable as a fallback for large datasets.

        Args:
            embeddings: (N, D) float array
            n_clusters: Number of clusters. If None, estimate from data size.

        Returns:
            (labels, probabilities) - probabilities are 1.0 for hard clustering
        """
        n_samples = embeddings.shape[0]

        # Estimate number of clusters if not provided
        # Use sqrt(N/2) as a heuristic, capped between 2 and 50
        if n_clusters is None:
            n_clusters = max(2, min(50, int(np.sqrt(n_samples / 2))))

        # Ensure n_clusters doesn't exceed n_samples
        n_clusters = min(n_clusters, n_samples)

        _LOGGER.info(
            "minibatch_kmeans_fallback",
            n_samples=n_samples,
            n_clusters=n_clusters,
        )

        kmeans = MiniBatchKMeans(
            n_clusters=n_clusters,
            random_state=42,
            batch_size=min(1024, n_samples),
            n_init=3,  # Fewer initializations for speed
            max_iter=100,
        )
        labels = kmeans.fit_predict(embeddings)

        # MiniBatchKMeans is hard clustering, so probabilities are 1.0
        probabilities = np.ones(n_samples, dtype=float)

        return labels, probabilities

    def _estimate_optimal_clusters(self, n_samples: int) -> int:
        """
        Estimate optimal number of clusters for MiniBatchKMeans fallback.

        Uses silhouette analysis on a small sample to find a good k.
        For very large datasets, uses heuristics to avoid expensive computation.

        Args:
            n_samples: Number of samples in the dataset

        Returns:
            Estimated optimal number of clusters
        """
        # For small datasets, use simple heuristic
        if n_samples < _SMALL_DATASET_SAMPLE_THRESHOLD:
            return max(2, n_samples // 10)

        # For larger datasets, use sqrt(N/2) heuristic
        # This is a common rule of thumb for clustering
        return max(2, min(50, int(np.sqrt(n_samples / 2))))

    def _maybe_reduce_with_umap(
        self,
        embeddings: np.ndarray,
        options: _UmapOptions,
    ) -> tuple[np.ndarray, bool]:
        """Reduce embeddings with UMAP + FAISS k-NN when enabled and sample count permits."""
        use_umap = bool(
            self.settings.enable_umap_force
            or (
                self.settings.enable_umap_auto
                and embeddings.shape[0] >= self.settings.umap_threshold_sentences
            )
        )
        if not use_umap:
            return embeddings, False

        from umap import (  # noqa: PLC0415 - heavy umap/numba import deferred until UMAP is invoked
            UMAP,
        )

        n_data_points = embeddings.shape[0]
        requested_n_neighbors = options.n_neighbors or self.settings.umap_n_neighbors
        # Safety margin: limit n_neighbors to at most N/3 for stability
        safety_limit = max(2, n_data_points // 3)
        adjusted_n_neighbors = max(2, min(requested_n_neighbors, safety_limit))

        if n_data_points < _MIN_UMAP_POINTS:
            return embeddings, False

        try:
            # Use FAISS for k-NN to avoid pynndescent 0.6.0 integer overflow
            knn_indices, knn_dists = compute_knn_faiss(embeddings, adjusted_n_neighbors)
            reducer = UMAP(
                n_components=options.n_components or self.settings.umap_n_components,
                n_neighbors=adjusted_n_neighbors,
                metric="cosine",
                min_dist=options.min_dist or self.settings.umap_min_dist,
                random_state=42,
                n_jobs=1,
                precomputed_knn=(knn_indices, knn_dists),
            )
            return reducer.fit_transform(embeddings), True
        except Exception as e:  # noqa: BLE001 - FAISS/UMAP failure falls back to non-UMAP clustering
            _LOGGER.warning(
                "faiss_knn_failed_fallback_no_umap",
                error=str(e),
                n_samples=n_data_points,
            )
            return embeddings, False

    def _run_hdbscan_with_fallback(
        self,
        reduced: np.ndarray,
        n_original: int,
        use_umap: bool,
        options: _HdbscanOptions,
    ) -> _HdbscanRunOutcome:
        """Run HDBSCAN with timeout and MiniBatchKMeans fallback when needed."""
        effective_mcs = (
            options.min_cluster_size
            if options.min_cluster_size > 0
            else self.settings.hdbscan_min_cluster_size
        )
        effective_ms = (
            options.min_samples if options.min_samples > 0 else self.settings.hdbscan_min_samples
        )

        if reduced.shape[0] < max(2, effective_mcs, effective_ms):
            labels = np.arange(n_original, dtype=int)
            probs = np.ones_like(labels, dtype=float)
            return _HdbscanRunOutcome(labels, probs, used_umap=False, used_fallback=False)

        def run_hdbscan() -> tuple[np.ndarray, np.ndarray]:
            clusterer = HDBSCAN(
                min_cluster_size=effective_mcs,
                min_samples=effective_ms,
                metric="euclidean",
                cluster_selection_epsilon=options.cluster_selection_epsilon
                if options.cluster_selection_epsilon is not None
                else 0.0,
                allow_single_cluster=options.allow_single_cluster
                if options.allow_single_cluster is not None
                else False,
                cluster_selection_method=options.cluster_selection_method
                or self.settings.hdbscan_cluster_selection_method,
            )
            clusterer.fit(reduced)
            return clusterer.labels_, clusterer.probabilities_

        used_fallback = False
        if _is_abandoned_hdbscan_running():
            _LOGGER.warning(
                "abandoned_hdbscan_still_running",
                reason="abandoned_hdbscan_still_running",
                n_samples=reduced.shape[0],
                min_cluster_size=effective_mcs,
            )
            labels, probs = self._fallback_minibatch_kmeans(reduced)
            used_fallback = True
        else:
            timeout_seconds = self.settings.hdbscan_timeout_seconds
            result = self._run_with_timeout(run_hdbscan, timeout_seconds, track_abandoned=True)
            if result is not None:
                labels, probs = result
            else:
                _LOGGER.warning(
                    "hdbscan_fallback_triggered",
                    timeout_seconds=timeout_seconds,
                    n_samples=reduced.shape[0],
                    min_cluster_size=effective_mcs,
                )
                labels, probs = self._fallback_minibatch_kmeans(reduced)
                used_fallback = True

        if (labels >= 0).sum() == 0:
            labels = np.arange(n_original, dtype=int)
            probs = np.ones_like(labels, dtype=float)
            use_umap = False

        return _HdbscanRunOutcome(labels, probs, used_umap=use_umap, used_fallback=used_fallback)

    def _recluster_noise(
        self,
        reduced: np.ndarray,
        labels: np.ndarray,
        probs: np.ndarray,
        min_cluster_size: int,
    ) -> None:
        """Attempt to cluster HDBSCAN noise points (-1) in place using KMeans."""
        if not self.settings.noise_recluster_enabled:
            return

        noise_mask = labels == -1
        n_noise = noise_mask.sum()
        if n_noise < self.settings.noise_recluster_min_points:
            return

        noise_embeddings = reduced[noise_mask]
        max_k = min(
            self.settings.noise_recluster_max_clusters,
            n_noise // max(2, min_cluster_size),
        )
        if max_k < _MIN_CLUSTER_COUNT:
            return

        best_k = _find_best_noise_k(noise_embeddings, max_k)
        if best_k >= _MIN_CLUSTER_COUNT:
            try:
                kmeans = KMeans(n_clusters=best_k, random_state=42, n_init=10)
                noise_labels = kmeans.fit_predict(noise_embeddings)

                max_existing_label = labels.max() if labels.size > 0 else -1
                base_id = max_existing_label + 1
                labels[noise_mask] = base_id + noise_labels
                probs[noise_mask] = 1.0
            except (ValueError, RuntimeError) as exc:
                _LOGGER.warning(
                    "noise_recluster.kmeans_apply_failed",
                    best_k=best_k,
                    error=str(exc),
                )

    def cluster(  # noqa: PLR0913 - public clustering entry point; callers pass these by keyword
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
        if invalid := _validate_cluster_input(embeddings):
            return invalid

        with _guard_native_clustering():
            reduced, use_umap = self._maybe_reduce_with_umap(
                embeddings,
                _UmapOptions(
                    n_neighbors=umap_n_neighbors,
                    n_components=umap_n_components,
                    min_dist=umap_min_dist,
                ),
            )
            outcome = self._run_hdbscan_with_fallback(
                reduced,
                embeddings.shape[0],
                use_umap,
                _HdbscanOptions(
                    min_cluster_size=min_cluster_size,
                    min_samples=min_samples,
                    cluster_selection_epsilon=hdbscan_cluster_selection_epsilon,
                    cluster_selection_method=hdbscan_cluster_selection_method,
                    allow_single_cluster=hdbscan_allow_single_cluster,
                ),
            )
            self._recluster_noise(reduced, outcome.labels, outcome.probs, min_cluster_size)

            # Calculate DBCV score using the reduced space (or embeddings if UMAP not used)
            dbcv = self._calculate_dbcv(reduced, outcome.labels)
            sil_score = self._calculate_silhouette(reduced, outcome.labels)

            return ClusterResult(
                labels=outcome.labels,
                probabilities=outcome.probs,
                used_umap=outcome.used_umap,
                params=HDBSCANSettings(
                    min_cluster_size=min_cluster_size,
                    min_samples=min_samples,
                ),
                dbcv_score=dbcv,
                silhouette_score=sil_score,
                used_fallback=outcome.used_fallback,
            )

    def _calculate_dbcv(
        self,
        X: np.ndarray,  # noqa: N803  # ML matrix convention
        labels: np.ndarray,
    ) -> float | None:
        """
        Calculate DBCV (Density-Based Clustering Validation) score using hdbscan.validity.validity_index.

        Args:
            X: Embedding space (reduced if UMAP was used, otherwise original embeddings)
            labels: Cluster labels from HDBSCAN (-1 indicates noise)

        Returns:
            DBCV score (typically in [-1, 1] range), or None if the score could
            not be measured (too few clusters/points, or calculation failed) —
            callers must not treat this as a measured 0.0.
        """
        try:
            from hdbscan.validity import (  # noqa: PLC0415 - heavy hdbscan/numba import deferred until DBCV evaluation
                validity_index,
            )
        except ImportError:
            # hdbscan is not available (e.g., build issues) — not measured.
            return None

        try:
            # Filter out noise points (-1) for DBCV calculation
            mask = labels != -1
            if mask.sum() < _MIN_CLUSTER_COUNT:
                return None

            filtered_X = X[mask]  # noqa: N806 - ML feature matrix convention
            filtered_labels = labels[mask]

            # DBCV requires at least 2 distinct clusters
            if len(set(filtered_labels)) < _MIN_CLUSTER_COUNT:
                return None

            dbcv = float(validity_index(filtered_X, filtered_labels, metric="euclidean"))

            # Ensure result is finite (handle NaN/Inf)
            if not np.isfinite(dbcv):
                return None
        except (ValueError, ArithmeticError) as exc:
            _LOGGER.warning(
                "clusterer.dbcv_calculation_failed",
                error=str(exc),
                error_type=type(exc).__name__,
            )
            return None
        else:
            return dbcv

    def _calculate_silhouette(self, embeddings: np.ndarray, labels: np.ndarray) -> float | None:
        """Returns None (not a measured 0.0) when the score can't be computed."""
        try:
            # Silhouette score requires at least 2 distinct labels
            # Filter out noise points (-1) for silhouette calculation
            # This is a common practice as noise points don't belong to any cluster
            # and can skew the score.
            non_noise_indices = labels != -1
            filtered_embeddings = embeddings[non_noise_indices]
            filtered_labels = labels[non_noise_indices]

            if (
                len(set(filtered_labels)) < _MIN_CLUSTER_COUNT
                or len(filtered_labels) < _MIN_CLUSTER_COUNT
            ):
                return None

            return float(silhouette_score(filtered_embeddings, filtered_labels))
        except (ValueError, ArithmeticError) as exc:
            _LOGGER.warning(
                "clusterer.silhouette_calculation_failed",
                error=str(exc),
                error_type=type(exc).__name__,
            )
            return None

    def optimize_clustering(  # noqa: PLR0913 - implements port signature
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
        config = _OptimizeConfig(
            min_cluster_size_range=min_cluster_size_range,
            min_samples_range=min_samples_range,
            umap_n_neighbors_range=umap_n_neighbors_range,
            umap_n_components_range=umap_n_components_range,
            hdbscan_cluster_selection_method=hdbscan_cluster_selection_method,
            hdbscan_allow_single_cluster=hdbscan_allow_single_cluster,
            token_counts=token_counts,
        )
        if self.settings.use_bayes_opt:
            return self._optimize_clustering_optuna(embeddings, config)

        return self._optimize_clustering_grid(embeddings, config)

    def _evaluate_grid_candidate(
        self,
        embeddings: np.ndarray,
        candidate: _GridCandidate,
        config: _OptimizeConfig,
    ) -> _GridEvaluation:
        """Cluster one grid candidate and compute its composite optimization score."""
        result = self.cluster(
            embeddings,
            min_cluster_size=candidate.min_cluster_size,
            min_samples=candidate.min_samples,
            umap_n_neighbors=candidate.n_neighbors,
            umap_n_components=candidate.n_components,
            # Plan item 3: epsilon=0.5 for merging close clusters
            hdbscan_cluster_selection_epsilon=0.5,
            hdbscan_cluster_selection_method=config.hdbscan_cluster_selection_method,
            hdbscan_allow_single_cluster=config.hdbscan_allow_single_cluster,
        )
        return _GridEvaluation(
            result=result,
            score=_composite_score(result),
            min_cluster_size=candidate.min_cluster_size,
            min_samples=candidate.min_samples,
        )

    def _grid_empty_fallback(self, embeddings: np.ndarray, n_data_points: int) -> ClusterResult:
        """Fallback clustering when no grid candidate yields a result."""
        return self.cluster(embeddings, min_cluster_size=max(3, n_data_points // 5), min_samples=1)

    def _apply_recursive_clustering(
        self,
        embeddings: np.ndarray,
        result: ClusterResult,
        token_counts: np.ndarray | None,
    ) -> ClusterResult:
        """Recursively split oversized clusters when token counts are supplied."""
        if token_counts is not None and result.labels.size > 0:
            new_labels, new_probs = self.recursive_cluster(
                embeddings, result.labels, result.probabilities, token_counts
            )
            result.labels = new_labels
            result.probabilities = new_probs
        return result

    def _optimize_clustering_grid(
        self,
        embeddings: np.ndarray,
        config: _OptimizeConfig,
    ) -> ClusterResult:
        """
        Grid search hyperparameter optimization (original implementation).
        Extracted to allow Optuna fallback.
        """
        ranges = _resolve_search_ranges(config)

        # Pre-validation
        if _is_invalid_optimization_input(embeddings):
            return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        n_data_points = embeddings.shape[0]
        best = _BestGridResult(score=-2.0, result=None)

        for n_neighbors in ranges.umap_n_neighbors_range:
            for n_components in ranges.umap_n_components_range:
                # Run HDBSCAN grid on this UMAP output
                for mcs in ranges.min_cluster_size_range:
                    if mcs >= n_data_points:
                        continue

                    for ms in ranges.min_samples_range:
                        current_ms = ms if ms is not None else mcs
                        if current_ms > mcs:
                            continue

                        evaluation = self._evaluate_grid_candidate(
                            embeddings,
                            _GridCandidate(
                                n_neighbors=n_neighbors,
                                n_components=n_components,
                                min_cluster_size=mcs,
                                min_samples=current_ms,
                            ),
                            config,
                        )
                        best = _select_better_grid_result(best, evaluation)

        best_result = (
            best.result
            if best.result is not None
            else self._grid_empty_fallback(embeddings, n_data_points)
        )
        return self._apply_recursive_clustering(embeddings, best_result, config.token_counts)

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
                    min(dynamic_max_tokens, self.settings.recursive_max_tokens_ceil),
                )

                # Dynamic min_split_size: 10th percentile of cluster sizes, but at least 5
                min_split_size = max(
                    5, int(np.percentile(cluster_sizes, 10)) if len(cluster_sizes) > 0 else 5
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
                        n_clusters=2, random_state=42, bisecting_strategy="largest_cluster"
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

    def _evaluate_optuna_trial(
        self,
        embeddings: np.ndarray,
        trial: optuna.Trial,
        space: _OptunaSearchSpace,
        config: _OptimizeConfig,
    ) -> float:
        """Suggest hyperparameters for one Optuna trial and return its composite score."""
        logged_params: dict[str, int | None] = {
            "min_cluster_size": None,
            "min_samples": None,
            "n_neighbors": None,
            "n_components": None,
        }
        try:
            mcs = trial.suggest_int("min_cluster_size", space.mcs_min, space.mcs_max)
            logged_params["min_cluster_size"] = mcs
            ms_upper = min(space.ms_max, mcs)
            ms = trial.suggest_int("min_samples", 1, ms_upper)
            logged_params["min_samples"] = ms
            n_neighbors = _suggest_optuna_umap_param(
                trial, "umap_n_neighbors", space.umap_n_neighbors_list
            )
            logged_params["n_neighbors"] = n_neighbors
            n_components = _suggest_optuna_umap_param(
                trial, "umap_n_components", space.umap_n_components_list
            )
            logged_params["n_components"] = n_components

            result = self.cluster(
                embeddings,
                min_cluster_size=mcs,
                min_samples=ms,
                umap_n_neighbors=n_neighbors,
                umap_n_components=n_components,
                hdbscan_cluster_selection_epsilon=0.5,
                hdbscan_cluster_selection_method=config.hdbscan_cluster_selection_method,
                hdbscan_allow_single_cluster=config.hdbscan_allow_single_cluster,
            )
            return _composite_score(result)
        except (IndexError, ValueError, RuntimeError, TypeError) as e:
            _LOGGER.warning(
                "optuna_trial_failed",
                trial_number=trial.number,
                params=logged_params,
                error=str(e),
            )
            return float("-inf")

    def _optimize_clustering_optuna(
        self,
        embeddings: np.ndarray,
        config: _OptimizeConfig,
    ) -> ClusterResult:
        """
        Bayesian hyperparameter optimization using Optuna (TPE sampler).
        Uses composite score (0.6*silhouette + 0.4*DBCV) as the objective.
        """
        try:
            import optuna  # noqa: PLC0415 - optional heavy optuna import deferred until Bayes optimization
            from optuna.samplers import (  # noqa: PLC0415 - optional heavy optuna import deferred until Bayes optimization
                TPESampler,
            )
        except ImportError:
            return self._optimize_clustering_grid(embeddings, config)

        if _is_invalid_optimization_input(embeddings):
            return self.cluster(embeddings, min_cluster_size=5, min_samples=2)

        ranges = _resolve_search_ranges(config)
        space = _build_optuna_search_space(embeddings.shape[0], ranges)

        def objective(trial: optuna.Trial) -> float:
            return self._evaluate_optuna_trial(embeddings, trial, space, config)

        sampler = TPESampler(seed=42)
        study = optuna.create_study(direction="maximize", sampler=sampler)

        timeout = self.settings.bayes_opt_timeout_seconds
        n_trials = self.settings.bayes_opt_trials
        if timeout is not None:
            study.optimize(objective, n_trials=n_trials, timeout=timeout)
        else:
            study.optimize(objective, n_trials=n_trials)

        best_params = study.best_params
        best_result = self.cluster(
            embeddings,
            min_cluster_size=best_params["min_cluster_size"],
            min_samples=best_params["min_samples"],
            umap_n_neighbors=best_params.get("umap_n_neighbors"),
            umap_n_components=best_params.get("umap_n_components"),
            hdbscan_cluster_selection_epsilon=0.5,
            hdbscan_cluster_selection_method=config.hdbscan_cluster_selection_method,
            hdbscan_allow_single_cluster=config.hdbscan_allow_single_cluster,
        )
        return self._apply_recursive_clustering(embeddings, best_result, config.token_counts)

    def subcluster_other(
        self, embeddings: np.ndarray, token_counts: np.ndarray | None = None
    ) -> ClusterResult:
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
