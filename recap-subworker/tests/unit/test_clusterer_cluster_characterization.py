"""Characterization tests for Clusterer.cluster."""

from __future__ import annotations

from typing import ClassVar
from unittest.mock import MagicMock, patch

import numpy as np

from recap_subworker.infra.config import Settings
from recap_subworker.services import clusterer as clusterer_module
from recap_subworker.services.clusterer import Clusterer

EMBEDDING_DIM = 4
MIN_POINTS_TWO = 2
SIX_POINTS = 6
EIGHT_POINTS = 8
SETTINGS_DEFAULT_MCS = 3
SETTINGS_DEFAULT_MS = 2
FAKE_MIN_SAMPLES_ZERO = 0
FAKE_MIN_CLUSTER_SIZE_ZERO = 0
NOISE_MIN_POINTS = 4
NOISE_MAX_CLUSTERS_ONE = 1
NOISE_MAX_CLUSTERS_THREE = 3
TIED_SILHOUETTE = 0.42
EXPECTED_FIRST_TIE_K = 2
EXPECTED_CLUSTER_COUNT = 2
APPLY_CALL_INDEX_SECOND = 2
APPLY_CALL_INDEX_THIRD = 3


def _non_zero_embeddings(n_points: int) -> np.ndarray:
    rng = np.random.default_rng(42)
    return rng.uniform(0.5, 1.5, size=(n_points, EMBEDDING_DIM))


def _make_settings() -> MagicMock:
    settings = MagicMock(spec=Settings)
    settings.enable_umap_force = False
    settings.enable_umap_auto = False
    settings.umap_threshold_sentences = 100
    settings.umap_n_neighbors = 15
    settings.umap_n_components = 5
    settings.umap_min_dist = 0.0
    settings.hdbscan_min_cluster_size = SETTINGS_DEFAULT_MCS
    settings.hdbscan_min_samples = SETTINGS_DEFAULT_MS
    settings.hdbscan_cluster_selection_method = "eom"
    settings.hdbscan_timeout_seconds = 30
    settings.noise_recluster_enabled = False
    settings.noise_recluster_min_points = NOISE_MIN_POINTS
    settings.noise_recluster_max_clusters = NOISE_MAX_CLUSTERS_THREE
    return settings


class _FakeHDBSCAN:
    last_init_kwargs: ClassVar[dict[str, object]] = {}
    labels_to_return: ClassVar[np.ndarray] = np.array([], dtype=int)
    probs_to_return: ClassVar[np.ndarray] = np.array([], dtype=float)

    def __init__(self, **kwargs: object) -> None:
        _FakeHDBSCAN.last_init_kwargs = dict(kwargs)

    def fit(self, features: np.ndarray) -> _FakeHDBSCAN:
        self.labels_ = _FakeHDBSCAN.labels_to_return.copy()
        self.probabilities_ = _FakeHDBSCAN.probs_to_return.copy()
        return self


def test_cluster_skips_umap_when_force_enabled_but_fewer_than_three_points() -> None:
    settings = _make_settings()
    settings.enable_umap_force = True
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(MIN_POINTS_TWO)
    _FakeHDBSCAN.labels_to_return = np.array([0, 1], dtype=int)
    _FakeHDBSCAN.probs_to_return = np.array([1.0, 1.0], dtype=float)

    with (
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch("recap_subworker.services.clusterer.compute_knn_faiss") as mock_knn,
        patch.object(clusterer, "_calculate_dbcv", return_value=0.5),
        patch.object(clusterer, "_calculate_silhouette", return_value=0.6),
    ):
        result = clusterer.cluster(embeddings, min_cluster_size=MIN_POINTS_TWO, min_samples=1)

    mock_knn.assert_not_called()
    assert result.used_umap is False
    np.testing.assert_array_equal(result.labels, [0, 1])


def test_cluster_umap_faiss_failure_falls_back_to_no_umap() -> None:
    settings = _make_settings()
    settings.enable_umap_force = True
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(SIX_POINTS)
    _FakeHDBSCAN.labels_to_return = np.array([0, 0, 0, 1, 1, 1], dtype=int)
    _FakeHDBSCAN.probs_to_return = np.ones(SIX_POINTS, dtype=float)

    with (
        patch(
            "recap_subworker.services.clusterer.compute_knn_faiss",
            side_effect=RuntimeError("faiss boom"),
        ),
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch.object(clusterer, "_calculate_dbcv", return_value=0.3),
        patch.object(clusterer, "_calculate_silhouette", return_value=0.4),
        patch.object(clusterer_module._LOGGER, "warning") as mock_warning,
    ):
        result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)

    assert result.used_umap is False
    mock_warning.assert_called_once()
    assert mock_warning.call_args.args[0] == "faiss_knn_failed_fallback_no_umap"
    assert mock_warning.call_args.kwargs["n_samples"] == SIX_POINTS


def test_cluster_non_positive_hdbscan_params_use_settings_defaults_for_fit() -> None:
    settings = _make_settings()
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(SIX_POINTS)
    _FakeHDBSCAN.labels_to_return = np.array([0, 0, 0, 1, 1, 1], dtype=int)
    _FakeHDBSCAN.probs_to_return = np.ones(SIX_POINTS, dtype=float)

    with (
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch.object(clusterer, "_calculate_dbcv", return_value=None),
        patch.object(clusterer, "_calculate_silhouette", return_value=None),
    ):
        result = clusterer.cluster(
            embeddings,
            min_cluster_size=FAKE_MIN_CLUSTER_SIZE_ZERO,
            min_samples=FAKE_MIN_SAMPLES_ZERO,
        )

    assert _FakeHDBSCAN.last_init_kwargs["min_cluster_size"] == SETTINGS_DEFAULT_MCS
    assert _FakeHDBSCAN.last_init_kwargs["min_samples"] == SETTINGS_DEFAULT_MS
    assert result.params.min_cluster_size == FAKE_MIN_CLUSTER_SIZE_ZERO
    assert result.params.min_samples == FAKE_MIN_SAMPLES_ZERO


def test_cluster_all_noise_labels_reassigns_sequential_ids_and_clears_umap() -> None:
    settings = _make_settings()
    settings.enable_umap_force = True
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(SIX_POINTS)
    _FakeHDBSCAN.labels_to_return = np.full(SIX_POINTS, -1, dtype=int)
    _FakeHDBSCAN.probs_to_return = np.zeros(SIX_POINTS, dtype=float)

    fake_umap_instance = MagicMock()
    fake_umap_instance.fit_transform.return_value = embeddings

    with (
        patch(
            "recap_subworker.services.clusterer.compute_knn_faiss",
            return_value=(np.zeros((SIX_POINTS, 2), dtype=int), np.zeros((SIX_POINTS, 2))),
        ),
        patch("umap.UMAP", return_value=fake_umap_instance),
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch.object(clusterer, "_calculate_dbcv", return_value=None),
        patch.object(clusterer, "_calculate_silhouette", return_value=None),
    ):
        result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)

    assert result.used_umap is False
    np.testing.assert_array_equal(result.labels, np.arange(SIX_POINTS, dtype=int))
    np.testing.assert_array_equal(result.probabilities, np.ones(SIX_POINTS, dtype=float))


def test_cluster_noise_recluster_skips_when_max_k_below_two() -> None:
    settings = _make_settings()
    settings.noise_recluster_enabled = True
    settings.noise_recluster_min_points = NOISE_MIN_POINTS
    settings.noise_recluster_max_clusters = NOISE_MAX_CLUSTERS_ONE
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(SIX_POINTS)
    _FakeHDBSCAN.labels_to_return = np.array([-1, -1, -1, -1, 0, 0], dtype=int)
    _FakeHDBSCAN.probs_to_return = np.ones(SIX_POINTS, dtype=float)

    with (
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch("recap_subworker.services.clusterer.KMeans") as mock_kmeans,
        patch.object(clusterer, "_calculate_dbcv", return_value=None),
        patch.object(clusterer, "_calculate_silhouette", return_value=None),
    ):
        result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)

    mock_kmeans.assert_not_called()
    np.testing.assert_array_equal(result.labels, [-1, -1, -1, -1, 0, 0])


def test_cluster_noise_recluster_equal_silhouette_keeps_first_k() -> None:
    settings = _make_settings()
    settings.noise_recluster_enabled = True
    settings.noise_recluster_min_points = NOISE_MIN_POINTS
    settings.noise_recluster_max_clusters = NOISE_MAX_CLUSTERS_THREE
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(EIGHT_POINTS)
    _FakeHDBSCAN.labels_to_return = np.array([-1, -1, -1, -1, -1, -1, 0, 0], dtype=int)
    _FakeHDBSCAN.probs_to_return = np.ones(EIGHT_POINTS, dtype=float)

    fit_calls: list[int] = []

    class _FakeKMeans:
        def __init__(self, *, n_clusters: int, random_state: int, n_init: int) -> None:
            self.n_clusters = n_clusters

        def fit_predict(self, data: np.ndarray) -> np.ndarray:
            fit_calls.append(self.n_clusters)
            return np.array([i % self.n_clusters for i in range(len(data))], dtype=int)

    with (
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch("recap_subworker.services.clusterer.KMeans", _FakeKMeans),
        patch(
            "recap_subworker.services.clusterer.silhouette_score",
            return_value=TIED_SILHOUETTE,
        ),
        patch.object(clusterer, "_calculate_dbcv", return_value=0.2),
        patch.object(clusterer, "_calculate_silhouette", return_value=0.3),
    ):
        result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)

    assert fit_calls == [EXPECTED_FIRST_TIE_K, NOISE_MAX_CLUSTERS_THREE, EXPECTED_FIRST_TIE_K]
    assert set(result.labels[:SIX_POINTS]) == {1, 2}


def test_cluster_noise_recluster_handles_search_exception_and_single_label() -> None:
    settings = _make_settings()
    settings.noise_recluster_enabled = True
    settings.noise_recluster_min_points = NOISE_MIN_POINTS
    settings.noise_recluster_max_clusters = NOISE_MAX_CLUSTERS_THREE
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(EIGHT_POINTS)
    _FakeHDBSCAN.labels_to_return = np.array([-1, -1, -1, -1, -1, -1, 0, 0], dtype=int)
    _FakeHDBSCAN.probs_to_return = np.ones(EIGHT_POINTS, dtype=float)

    fit_calls: list[int] = []

    class _FakeKMeans:
        def __init__(self, *, n_clusters: int, random_state: int, n_init: int) -> None:
            self.n_clusters = n_clusters

        def fit_predict(self, data: np.ndarray) -> np.ndarray:
            fit_calls.append(self.n_clusters)
            if self.n_clusters == NOISE_MAX_CLUSTERS_THREE:
                msg = "k=3 search failed"
                raise ValueError(msg)
            return np.zeros(len(data), dtype=int)

    with (
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch("recap_subworker.services.clusterer.KMeans", _FakeKMeans),
        patch("recap_subworker.services.clusterer.silhouette_score") as mock_sil,
        patch.object(clusterer, "_calculate_dbcv", return_value=0.2),
        patch.object(clusterer, "_calculate_silhouette", return_value=0.3),
        patch.object(clusterer_module._LOGGER, "debug") as mock_debug,
    ):
        result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)

    mock_sil.assert_not_called()
    assert fit_calls == [EXPECTED_FIRST_TIE_K, NOISE_MAX_CLUSTERS_THREE, EXPECTED_FIRST_TIE_K]
    debug_events = [c.args[0] for c in mock_debug.call_args_list]
    assert "noise_recluster.kmeans_k_search_failed" in debug_events
    assert (result.labels[:SIX_POINTS] == 1).all()


def test_cluster_noise_recluster_apply_failure_keeps_noise_labels() -> None:
    settings = _make_settings()
    settings.noise_recluster_enabled = True
    settings.noise_recluster_min_points = NOISE_MIN_POINTS
    settings.noise_recluster_max_clusters = NOISE_MAX_CLUSTERS_THREE
    clusterer = Clusterer(settings)
    embeddings = _non_zero_embeddings(SIX_POINTS)
    _FakeHDBSCAN.labels_to_return = np.array([-1, -1, -1, -1, 0, 0], dtype=int)
    _FakeHDBSCAN.probs_to_return = np.ones(SIX_POINTS, dtype=float)

    call_idx = 0

    class _FakeKMeansFailApply:
        def __init__(self, *, n_clusters: int, random_state: int, n_init: int) -> None:
            self.n_clusters = n_clusters

        def fit_predict(self, data: np.ndarray) -> np.ndarray:
            nonlocal call_idx
            call_idx += 1
            if call_idx == APPLY_CALL_INDEX_SECOND:
                msg = "apply failed"
                raise RuntimeError(msg)
            return np.array([0, 0, 1, 1], dtype=int)

    with (
        patch("recap_subworker.services.clusterer.HDBSCAN", _FakeHDBSCAN),
        patch("recap_subworker.services.clusterer.KMeans", _FakeKMeansFailApply),
        patch("recap_subworker.services.clusterer.silhouette_score", return_value=0.5),
        patch.object(clusterer, "_calculate_dbcv", return_value=0.2),
        patch.object(clusterer, "_calculate_silhouette", return_value=0.3),
        patch.object(clusterer_module._LOGGER, "warning") as mock_warning,
    ):
        result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)

    np.testing.assert_array_equal(result.labels, [-1, -1, -1, -1, 0, 0])
    mock_warning.assert_called_once()
    assert mock_warning.call_args.args[0] == "noise_recluster.kmeans_apply_failed"
