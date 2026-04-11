"""Common helpers for Graph Boost tuning scripts."""

from __future__ import annotations

from dataclasses import dataclass
from typing import NamedTuple, Sequence, Tuple

import pathlib

import pandas as pd
from sklearn.metrics import accuracy_score
from skopt import gp_minimize
from skopt.space import Integer, Real


@dataclass(frozen=True)
class GraphBoostParams:
    """Parameter triple that controls Graph Boost filtering."""

    graph_margin: float
    boost_threshold: float
    tag_count_threshold: int


HistoryEntry = Tuple[float, GraphBoostParams]


class OptimizationSummary(NamedTuple):
    """Aggregated results from a gp_minimize run."""

    best_params: GraphBoostParams
    best_accuracy: float
    history: Tuple[HistoryEntry, ...]


def find_latest_snapshot(directory: pathlib.Path) -> pathlib.Path:
    """Return the most recent CSV/Parquet snapshot in the given directory."""

    if not directory.exists():
        raise FileNotFoundError(f"{directory} does not exist")
    candidates = sorted(directory.glob("*"), reverse=True)
    for candidate in candidates:
        if candidate.suffix.lower() in {".parquet", ".pq", ".csv"}:
            return candidate
    raise FileNotFoundError(f"snapshot not found in {directory}")


def load_snapshot(path: pathlib.Path) -> pd.DataFrame:
    """Load a snapshot from disk and return a pandas DataFrame."""

    if path.suffix.lower() == ".csv":
        return pd.read_csv(path)
    if path.suffix.lower() in {".parquet", ".pq"}:
        return pd.read_parquet(path)
    raise ValueError(f"unsupported snapshot format: {path.suffix}")


def prepare_dataframe(df: pd.DataFrame) -> pd.DataFrame:
    """Clean snapshot fields and add a label column for Graph Boost."""

    df = df.dropna(subset=["margin", "top_boost", "tag_count", "strategy"]).copy()
    df = df[df["strategy"].isin({"graph_boost", "weighted_score"})]
    df = df.assign(
        label=df["strategy"] == "graph_boost",
        margin=df["margin"].astype(float),
        top_boost=df["top_boost"].astype(float),
        tag_count=df["tag_count"].astype(int),
    )
    return df


def _objective(params: Sequence[float], df: pd.DataFrame) -> float:
    graph_margin, boost_threshold, tag_count_min = params
    # top_boost がすべて 0 の場合は boost_threshold 条件を無視
    has_boost_values = (df["top_boost"] > 0).any()
    if has_boost_values:
        preds = (
            (df["margin"] >= graph_margin)
            & (df["top_boost"] >= boost_threshold)
            & (df["tag_count"] >= int(round(tag_count_min)))
        )
    else:
        # top_boost がすべて 0 の場合は boost_threshold 条件をスキップ
        preds = (
            (df["margin"] >= graph_margin)
            & (df["tag_count"] >= int(round(tag_count_min)))
        )
    accuracy = accuracy_score(df["label"], preds)
    return 1.0 - accuracy


def _params_from_raw(raw: Sequence[float]) -> GraphBoostParams:
    return GraphBoostParams(
        graph_margin=float(raw[0]),
        boost_threshold=float(raw[1]),
        tag_count_threshold=int(round(raw[2])),
    )


def run_bayes_optimization(
    df: pd.DataFrame, iterations: int, seed: int
) -> OptimizationSummary:
    """Execute gp_minimize over the Graph Boost snapshot."""

    space = [
        Real(0.05, 0.25, name="graph_margin"),
        Real(0.0, 5.0, name="boost_threshold"),
        Integer(0, 10, name="tag_count_threshold"),
    ]

    result = gp_minimize(
        func=lambda params: _objective(params, df),
        dimensions=space,
        n_calls=iterations,
        random_state=seed,
        acq_func="EI",
    )

    history = tuple(
        (1.0 - score, _params_from_raw(params))
        for score, params in zip(result.func_vals, result.x_iters)
    )

    summary = OptimizationSummary(
        best_params=_params_from_raw(result.x),
        best_accuracy=1.0 - result.fun,
        history=history,
    )
    return summary

