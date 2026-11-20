"""Characterization tests for Clusterer._optimize_clustering_grid."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from recap_subworker.domain.models import HDBSCANSettings
from recap_subworker.infra.config import Settings
from recap_subworker.services.clusterer import Clusterer, ClusterResult

DEFAULT_FALLBACK_MCS = 5
DEFAULT_FALLBACK_MS = 2
SMALL_FALLBACK_MCS_FOR_TWO = 3
TWENTY_POINTS = 20
TWENTY_POINT_FALLBACK_MCS = 4
FALLBACK_MIN_SAMPLES = 1
EQUAL_SCORE_SILHOUETTE = 0.5
EQUAL_SCORE_DBCV = 0.5
HIGHER_SCORE_SILHOUETTE = 0.9
LARGER_MCS = 6
SMALLER_MCS = 4
LARGER_MS = 3
SMALLER_MS = 1
FIRST_NEIGHBOR = 10
SECOND_NEIGHBOR = 15
COMPONENT_EIGHT = 8
EMBEDDING_DIM = 4


def _make_settings() -> MagicMock:
    settings = MagicMock(spec=Settings)
    settings.use_bayes_opt = False
    settings.enable_umap_force = False
    settings.enable_umap_auto = False
    settings.noise_recluster_enabled = False
    settings.hdbscan_min_cluster_size = 5
    settings.hdbscan_min_samples = 2
    settings.hdbscan_cluster_selection_method = "eom"
    settings.hdbscan_timeout_seconds = 300
    settings.clustering_recursive_enabled = False
    return settings


def _make_result(
    *,
    mcs: int,
    ms: int,
    silhouette: float | None = EQUAL_SCORE_SILHOUETTE,
    dbcv: float | None = EQUAL_SCORE_DBCV,
    n_points: int = 10,
) -> ClusterResult:
    resolved_labels = np.zeros(n_points, dtype=int)
    return ClusterResult(
        labels=resolved_labels,
        probabilities=np.ones(resolved_labels.size, dtype=float),
        used_umap=False,
        params=HDBSCANSettings(min_cluster_size=mcs, min_samples=ms),
        dbcv_score=dbcv,
        silhouette_score=silhouette,
    )


def test_grid_prevalidation_non_finite_calls_cluster_directly() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.array([[1.0, np.nan, 0.0, 0.0], [0.0, 1.0, 0.0, 0.0]])
    expected = _make_result(mcs=DEFAULT_FALLBACK_MCS, ms=DEFAULT_FALLBACK_MS, n_points=0)

    with patch.object(clusterer, "cluster", return_value=expected) as mock_cluster:
        result = clusterer.optimize_clustering(embeddings)

    assert result is expected
    mock_cluster.assert_called_once_with(
        embeddings,
        min_cluster_size=DEFAULT_FALLBACK_MCS,
        min_samples=DEFAULT_FALLBACK_MS,
    )


def test_grid_none_min_samples_resolves_to_min_cluster_size() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((10, EMBEDDING_DIM), dtype=float)
    recorded: list[tuple[int, int]] = []

    def fake_cluster(
        emb: np.ndarray,
        *,
        min_cluster_size: int,
        min_samples: int,
        **kwargs: object,
    ) -> ClusterResult:
        recorded.append((min_cluster_size, min_samples))
        return _make_result(mcs=min_cluster_size, ms=min_samples, n_points=emb.shape[0])

    with patch.object(clusterer, "cluster", side_effect=fake_cluster):
        result = clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[SMALLER_MCS],
            min_samples_range=[None],
            umap_n_neighbors_range=[FIRST_NEIGHBOR],
            umap_n_components_range=[COMPONENT_EIGHT],
        )

    assert recorded == [(SMALLER_MCS, SMALLER_MCS)]
    assert result.params.min_cluster_size == SMALLER_MCS
    assert result.params.min_samples == SMALLER_MCS


def test_grid_evaluation_order_and_tie_breaking() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((10, EMBEDDING_DIM), dtype=float)
    call_sequence: list[tuple[int | None, int | None, int, int]] = []
    results_by_call: dict[tuple[int | None, int | None, int, int], ClusterResult] = {}

    def fake_cluster(
        emb: np.ndarray,
        *,
        min_cluster_size: int,
        min_samples: int,
        umap_n_neighbors: int | None = None,
        umap_n_components: int | None = None,
        **kwargs: object,
    ) -> ClusterResult:
        key = (umap_n_neighbors, umap_n_components, min_cluster_size, min_samples)
        call_sequence.append(key)
        res = _make_result(
            mcs=min_cluster_size,
            ms=min_samples,
            silhouette=EQUAL_SCORE_SILHOUETTE,
            dbcv=EQUAL_SCORE_DBCV,
            n_points=emb.shape[0],
        )
        results_by_call[key] = res
        return res

    with patch.object(clusterer, "cluster", side_effect=fake_cluster):
        result = clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[LARGER_MCS, SMALLER_MCS],
            min_samples_range=[SMALLER_MS, LARGER_MS],
            umap_n_neighbors_range=[FIRST_NEIGHBOR, SECOND_NEIGHBOR],
            umap_n_components_range=[COMPONENT_EIGHT],
        )

    # Exact nested loop order: n_neighbors -> n_components -> mcs -> ms
    expected_sequence = [
        (FIRST_NEIGHBOR, COMPONENT_EIGHT, LARGER_MCS, SMALLER_MS),
        (FIRST_NEIGHBOR, COMPONENT_EIGHT, LARGER_MCS, LARGER_MS),
        (FIRST_NEIGHBOR, COMPONENT_EIGHT, SMALLER_MCS, SMALLER_MS),
        (FIRST_NEIGHBOR, COMPONENT_EIGHT, SMALLER_MCS, LARGER_MS),
        (SECOND_NEIGHBOR, COMPONENT_EIGHT, LARGER_MCS, SMALLER_MS),
        (SECOND_NEIGHBOR, COMPONENT_EIGHT, LARGER_MCS, LARGER_MS),
        (SECOND_NEIGHBOR, COMPONENT_EIGHT, SMALLER_MCS, SMALLER_MS),
        (SECOND_NEIGHBOR, COMPONENT_EIGHT, SMALLER_MCS, LARGER_MS),
    ]
    assert call_sequence == expected_sequence
    # On equal score: larger mcs wins, then larger ms wins, and on exact tie first wins
    expected_winner = results_by_call[(FIRST_NEIGHBOR, COMPONENT_EIGHT, LARGER_MCS, LARGER_MS)]
    assert result is expected_winner


def test_grid_higher_score_beats_larger_mcs() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((10, EMBEDDING_DIM), dtype=float)

    def fake_cluster(
        emb: np.ndarray,
        *,
        min_cluster_size: int,
        min_samples: int,
        **kwargs: object,
    ) -> ClusterResult:
        sil = HIGHER_SCORE_SILHOUETTE if min_cluster_size == SMALLER_MCS else EQUAL_SCORE_SILHOUETTE
        return _make_result(
            mcs=min_cluster_size,
            ms=min_samples,
            silhouette=sil,
            dbcv=EQUAL_SCORE_DBCV,
            n_points=emb.shape[0],
        )

    with patch.object(clusterer, "cluster", side_effect=fake_cluster):
        result = clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[LARGER_MCS, SMALLER_MCS],
            min_samples_range=[SMALLER_MS],
            umap_n_neighbors_range=[FIRST_NEIGHBOR],
            umap_n_components_range=[COMPONENT_EIGHT],
        )

    assert result.params.min_cluster_size == SMALLER_MCS


@pytest.mark.parametrize(
    ("n_points", "mcs_range", "expected_mcs"),
    [
        (2, None, SMALL_FALLBACK_MCS_FOR_TWO),
        (TWENTY_POINTS, [TWENTY_POINTS + 5], TWENTY_POINT_FALLBACK_MCS),
    ],
)
def test_grid_empty_search_triggers_fallback_cluster(
    n_points: int,
    mcs_range: list[int] | None,
    expected_mcs: int,
) -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((n_points, EMBEDDING_DIM), dtype=float)
    fallback_result = _make_result(mcs=expected_mcs, ms=FALLBACK_MIN_SAMPLES, n_points=n_points)

    with patch.object(clusterer, "cluster", return_value=fallback_result) as mock_cluster:
        result = clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=mcs_range,
        )

    assert result is fallback_result
    mock_cluster.assert_called_once_with(
        embeddings,
        min_cluster_size=expected_mcs,
        min_samples=FALLBACK_MIN_SAMPLES,
    )


def test_grid_applies_recursive_cluster_when_token_counts_provided() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((10, EMBEDDING_DIM), dtype=float)
    token_counts = np.full(10, 50, dtype=int)
    initial_result = _make_result(mcs=SMALLER_MCS, ms=SMALLER_MS, n_points=10)
    split_labels = np.array([0, 0, 0, 0, 0, 1, 1, 1, 1, 1], dtype=int)
    split_probs = np.ones(10, dtype=float)

    with (
        patch.object(clusterer, "cluster", return_value=initial_result),
        patch.object(
            clusterer, "recursive_cluster", return_value=(split_labels, split_probs)
        ) as mock_recursive,
    ):
        result = clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[SMALLER_MCS],
            min_samples_range=[SMALLER_MS],
            umap_n_neighbors_range=[FIRST_NEIGHBOR],
            umap_n_components_range=[COMPONENT_EIGHT],
            token_counts=token_counts,
        )

    mock_recursive.assert_called_once()
    np.testing.assert_array_equal(result.labels, split_labels)
    np.testing.assert_array_equal(result.probabilities, split_probs)


def test_grid_skips_recursive_cluster_when_labels_empty() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((10, EMBEDDING_DIM), dtype=float)
    token_counts = np.full(10, 50, dtype=int)
    empty_result = _make_result(
        mcs=SMALLER_MCS,
        ms=SMALLER_MS,
        n_points=0,
    )

    with (
        patch.object(clusterer, "cluster", return_value=empty_result),
        patch.object(clusterer, "recursive_cluster") as mock_recursive,
    ):
        clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[SMALLER_MCS],
            min_samples_range=[SMALLER_MS],
            umap_n_neighbors_range=[FIRST_NEIGHBOR],
            umap_n_components_range=[COMPONENT_EIGHT],
            token_counts=token_counts,
        )

    mock_recursive.assert_not_called()
