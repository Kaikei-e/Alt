"""Characterization tests for Clusterer._optimize_clustering_optuna."""

from __future__ import annotations

import types
from collections.abc import Callable
from dataclasses import dataclass, field
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from recap_subworker.domain.models import HDBSCANSettings
from recap_subworker.infra.config import Settings
from recap_subworker.services import clusterer as clusterer_module
from recap_subworker.services.clusterer import Clusterer, ClusterResult

EMBEDDING_DIM = 4
DEFAULT_FALLBACK_MCS = 5
DEFAULT_FALLBACK_MS = 2
TWENTY_POINTS = 20
THIRTY_POINTS = 30
ADJUSTED_MCS_MIN = 2
ADJUSTED_MCS_MAX = 5
SAFE_NEIGHBOR_FIVE = 5
SAFE_NEIGHBOR_TEN = 10
UNSAFE_NEIGHBOR_THIRTY = 30
COMPONENT_FIVE = 5
COMPONENT_EIGHT = 8
TRIAL_NUMBER = 3
BAYES_TIMEOUT_SECONDS = 12
BAYES_TRIALS = 4
SILHOUETTE_VAL = 0.5
DBCV_VAL = 0.25
EXPECTED_COMPOSITE_SCORE = 0.6 * SILHOUETTE_VAL + 0.4 * DBCV_VAL


@dataclass
class _FakeTrial:
    number: int = 0
    int_calls: list[tuple[str, int, int]] = field(default_factory=list)
    cat_calls: list[tuple[str, list[int]]] = field(default_factory=list)
    fail_on_cluster: bool = False

    def suggest_int(self, name: str, low: int, high: int) -> int:
        self.int_calls.append((name, low, high))
        return low

    def suggest_categorical(self, name: str, choices: list[int]) -> int:
        self.cat_calls.append((name, list(choices)))
        return choices[0]


class _FakeStudy:
    def __init__(self, trial: _FakeTrial) -> None:
        self._trial = trial
        self.best_params: dict[str, int] = {}
        self.optimize_calls: list[dict[str, int | None]] = []
        self.last_objective_value: float | None = None

    def optimize(
        self,
        func: Callable[[_FakeTrial], float],
        *,
        n_trials: int,
        timeout: int | None = None,
    ) -> None:
        self.optimize_calls.append({"n_trials": n_trials, "timeout": timeout})
        self.last_objective_value = func(self._trial)
        self.best_params = {
            "min_cluster_size": self._trial.int_calls[0][1],
            "min_samples": self._trial.int_calls[1][1],
        }
        for name, choices in self._trial.cat_calls:
            self.best_params[name] = choices[0]


def _make_fake_optuna(study: _FakeStudy) -> tuple[types.ModuleType, types.ModuleType]:
    optuna_mod = types.ModuleType("optuna")
    samplers_mod = types.ModuleType("optuna.samplers")

    def create_study(*, direction: str, sampler: object) -> _FakeStudy:
        return study

    class _FakeTPESampler:
        def __init__(self, *, seed: int) -> None:
            self.seed = seed

    optuna_mod.create_study = create_study  # type: ignore[attr-defined]
    samplers_mod.TPESampler = _FakeTPESampler  # type: ignore[attr-defined]
    optuna_mod.samplers = samplers_mod  # type: ignore[attr-defined]
    return optuna_mod, samplers_mod


def _make_settings(*, timeout: int | None = None) -> MagicMock:
    settings = MagicMock(spec=Settings)
    settings.use_bayes_opt = True
    settings.bayes_opt_trials = BAYES_TRIALS
    settings.bayes_opt_timeout_seconds = timeout
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
    n_points: int = 10,
    silhouette: float | None = SILHOUETTE_VAL,
    dbcv: float | None = DBCV_VAL,
) -> ClusterResult:
    return ClusterResult(
        labels=np.zeros(n_points, dtype=int),
        probabilities=np.ones(n_points, dtype=float),
        used_umap=False,
        params=HDBSCANSettings(min_cluster_size=mcs, min_samples=ms),
        dbcv_score=dbcv,
        silhouette_score=silhouette,
    )


def test_optuna_import_error_falls_back_to_grid() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((10, EMBEDDING_DIM), dtype=float)
    grid_result = _make_result(mcs=3, ms=1, n_points=10)

    with (
        patch.dict("sys.modules", {"optuna": None, "optuna.samplers": None}),
        patch.object(clusterer, "_optimize_clustering_grid", return_value=grid_result) as mock_grid,
    ):
        result = clusterer.optimize_clustering(embeddings)

    assert result is grid_result
    mock_grid.assert_called_once()


def test_optuna_prevalidation_invalid_input_calls_cluster_directly() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.array([[1.0, np.inf, 0.0, 0.0], [0.0, 1.0, 0.0, 0.0]])
    study = _FakeStudy(_FakeTrial())
    optuna_mod, samplers_mod = _make_fake_optuna(study)
    expected = _make_result(mcs=DEFAULT_FALLBACK_MCS, ms=DEFAULT_FALLBACK_MS, n_points=0)

    with (
        patch.dict("sys.modules", {"optuna": optuna_mod, "optuna.samplers": samplers_mod}),
        patch.object(clusterer, "cluster", return_value=expected) as mock_cluster,
    ):
        result = clusterer.optimize_clustering(embeddings)

    assert result is expected
    assert study.optimize_calls == []
    mock_cluster.assert_called_once_with(
        embeddings,
        min_cluster_size=DEFAULT_FALLBACK_MCS,
        min_samples=DEFAULT_FALLBACK_MS,
    )


def test_optuna_adjusts_empty_valid_mcs_and_none_min_samples() -> None:
    clusterer = Clusterer(_make_settings(timeout=BAYES_TIMEOUT_SECONDS))
    embeddings = np.ones((TWENTY_POINTS, EMBEDDING_DIM), dtype=float)
    trial = _FakeTrial()
    study = _FakeStudy(trial)
    optuna_mod, samplers_mod = _make_fake_optuna(study)

    def fake_cluster(
        emb: np.ndarray, *, min_cluster_size: int, min_samples: int, **kwargs: object
    ) -> ClusterResult:
        return _make_result(mcs=min_cluster_size, ms=min_samples, n_points=emb.shape[0])

    with (
        patch.dict("sys.modules", {"optuna": optuna_mod, "optuna.samplers": samplers_mod}),
        patch.object(clusterer, "cluster", side_effect=fake_cluster),
        patch.object(clusterer_module._LOGGER, "warning") as mock_warning,
    ):
        clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[TWENTY_POINTS + 5],
            min_samples_range=[None],
            umap_n_neighbors_range=[SAFE_NEIGHBOR_FIVE, SAFE_NEIGHBOR_TEN],
            umap_n_components_range=[COMPONENT_FIVE, COMPONENT_EIGHT],
        )

    assert study.optimize_calls == [{"n_trials": BAYES_TRIALS, "timeout": BAYES_TIMEOUT_SECONDS}]
    assert study.last_objective_value == pytest.approx(EXPECTED_COMPOSITE_SCORE)
    assert ("min_cluster_size", ADJUSTED_MCS_MIN, ADJUSTED_MCS_MAX) in trial.int_calls
    assert ("min_samples", 1, ADJUSTED_MCS_MIN) in trial.int_calls
    assert ("umap_n_neighbors", [SAFE_NEIGHBOR_FIVE]) in trial.cat_calls or (
        ("umap_n_neighbors", [SAFE_NEIGHBOR_FIVE]) not in trial.cat_calls
        and not any(name == "umap_n_neighbors" for name, _ in trial.cat_calls)
    )
    assert ("umap_n_components", [COMPONENT_FIVE, COMPONENT_EIGHT]) in trial.cat_calls
    warning_events = [c.args[0] for c in mock_warning.call_args_list]
    assert "min_cluster_size_range_adjusted" in warning_events


def test_optuna_categorical_neighbors_and_components_and_token_counts() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((THIRTY_POINTS, EMBEDDING_DIM), dtype=float)
    token_counts = np.full(THIRTY_POINTS, 25, dtype=int)
    trial = _FakeTrial()
    study = _FakeStudy(trial)
    optuna_mod, samplers_mod = _make_fake_optuna(study)
    split_labels = np.array([0] * 15 + [1] * 15, dtype=int)
    split_probs = np.ones(THIRTY_POINTS, dtype=float)

    def fake_cluster(
        emb: np.ndarray, *, min_cluster_size: int, min_samples: int, **kwargs: object
    ) -> ClusterResult:
        return _make_result(mcs=min_cluster_size, ms=min_samples, n_points=emb.shape[0])

    with (
        patch.dict("sys.modules", {"optuna": optuna_mod, "optuna.samplers": samplers_mod}),
        patch.object(clusterer, "cluster", side_effect=fake_cluster),
        patch.object(
            clusterer, "recursive_cluster", return_value=(split_labels, split_probs)
        ) as mock_recursive,
    ):
        result = clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[3, 6],
            min_samples_range=[1, 2],
            umap_n_neighbors_range=[SAFE_NEIGHBOR_FIVE, SAFE_NEIGHBOR_TEN, UNSAFE_NEIGHBOR_THIRTY],
            umap_n_components_range=[COMPONENT_FIVE, COMPONENT_EIGHT],
            token_counts=token_counts,
        )

    assert (
        "umap_n_neighbors",
        [SAFE_NEIGHBOR_FIVE, SAFE_NEIGHBOR_TEN],
    ) in trial.cat_calls
    assert ("umap_n_components", [COMPONENT_FIVE, COMPONENT_EIGHT]) in trial.cat_calls
    mock_recursive.assert_called_once()
    np.testing.assert_array_equal(result.labels, split_labels)
    np.testing.assert_array_equal(result.probabilities, split_probs)


def test_optuna_empty_umap_lists_pass_none_to_cluster() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((THIRTY_POINTS, EMBEDDING_DIM), dtype=float)
    trial = _FakeTrial()
    study = _FakeStudy(trial)
    optuna_mod, samplers_mod = _make_fake_optuna(study)
    recorded_umap_args: list[tuple[int | None, int | None]] = []

    def fake_cluster(
        emb: np.ndarray,
        *,
        min_cluster_size: int,
        min_samples: int,
        umap_n_neighbors: int | None = None,
        umap_n_components: int | None = None,
        **kwargs: object,
    ) -> ClusterResult:
        recorded_umap_args.append((umap_n_neighbors, umap_n_components))
        return _make_result(mcs=min_cluster_size, ms=min_samples, n_points=emb.shape[0])

    with (
        patch.dict("sys.modules", {"optuna": optuna_mod, "optuna.samplers": samplers_mod}),
        patch.object(clusterer, "cluster", side_effect=fake_cluster),
    ):
        clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[3, 6],
            min_samples_range=[1, 2],
            umap_n_neighbors_range=[SAFE_NEIGHBOR_FIVE],
            umap_n_components_range=[None],
        )

    assert recorded_umap_args == [(SAFE_NEIGHBOR_FIVE, None), (None, None)]


def test_optuna_trial_exception_logs_warning_and_returns_neg_inf() -> None:
    clusterer = Clusterer(_make_settings())
    embeddings = np.ones((THIRTY_POINTS, EMBEDDING_DIM), dtype=float)
    trial = _FakeTrial(number=TRIAL_NUMBER)
    study = _FakeStudy(trial)
    optuna_mod, samplers_mod = _make_fake_optuna(study)
    call_count = 0

    def fake_cluster(
        emb: np.ndarray, *, min_cluster_size: int, min_samples: int, **kwargs: object
    ) -> ClusterResult:
        nonlocal call_count
        call_count += 1
        if call_count == 1:
            msg = "synthetic trial failure"
            raise ValueError(msg)
        return _make_result(mcs=min_cluster_size, ms=min_samples, n_points=emb.shape[0])

    with (
        patch.dict("sys.modules", {"optuna": optuna_mod, "optuna.samplers": samplers_mod}),
        patch.object(clusterer, "cluster", side_effect=fake_cluster),
        patch.object(clusterer_module._LOGGER, "warning") as mock_warning,
    ):
        clusterer.optimize_clustering(
            embeddings,
            min_cluster_size_range=[3, 6],
            min_samples_range=[1, 2],
            umap_n_neighbors_range=[SAFE_NEIGHBOR_FIVE],
            umap_n_components_range=[COMPONENT_EIGHT],
        )

    assert study.last_objective_value == float("-inf")
    mock_warning.assert_called_once()
    assert mock_warning.call_args.args[0] == "optuna_trial_failed"
    assert mock_warning.call_args.kwargs["trial_number"] == TRIAL_NUMBER
    assert mock_warning.call_args.kwargs["params"] == {
        "min_cluster_size": 3,
        "min_samples": 1,
        "n_neighbors": SAFE_NEIGHBOR_FIVE,
        "n_components": COMPONENT_EIGHT,
    }
