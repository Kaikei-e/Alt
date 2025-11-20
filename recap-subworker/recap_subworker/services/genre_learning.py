"""Graph Boost / Genre learning helpers extracted from genre-classifier."""

from __future__ import annotations

import statistics
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Iterable, Sequence

import numpy as np
import pandas as pd
from sklearn.cluster import KMeans
from sklearn.metrics import accuracy_score, silhouette_score
from sklearn.preprocessing import StandardScaler
from skopt import gp_minimize
from skopt.space import Integer, Real
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession

DEFAULT_GRAPH_MARGIN = 0.15
DEFAULT_SNAPSHOT_HOURS = 24
DEFAULT_SNAPSHOT_LIMIT = 5000
DEFAULT_CLUSTER_GENRES = ["society_justice", "art_culture"]
DEFAULT_CLUSTER_MAX_K = 6
DEFAULT_CLUSTER_MIN_SAMPLES = 10
DEFAULT_BAYES_ITERATIONS = 30
DEFAULT_BAYES_SEED = 42
DEFAULT_BAYES_MIN_SAMPLES = 100


def _coerce_json(value: Any) -> Any:
    if value is None:
        return None
    if isinstance(value, str):
        try:
            import json

            return json.loads(value)
        except (ValueError, TypeError):
            return None
    return value


def _ensure_dict(value: Any) -> dict[str, Any]:
    parsed = _coerce_json(value)
    if isinstance(parsed, dict):
        return parsed
    return {}


def _ensure_list(value: Any) -> list[dict[str, Any]]:
    parsed = _coerce_json(value)
    if isinstance(parsed, list):
        return [item for item in parsed if isinstance(item, dict)]
    return []


def _ensure_confidence(value: Any) -> float | None:
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _compute_boosted_scores(candidates: list[dict[str, Any]]) -> tuple[float, float, int]:
    boosted: list[tuple[float, float]] = []
    for candidate in candidates:
        score = float(candidate.get("score") or 0.0)
        boost = float(candidate.get("graph_boost") or 0.0)
        boosted.append((score + boost, boost))
    if not boosted:
        return 0.0, 0.0, 0
    boosted.sort(key=lambda value: value[0], reverse=True)
    top_score, top_boost = boosted[0]
    second_score = boosted[1][0] if len(boosted) > 1 else top_score
    margin = top_score - second_score
    return margin, top_boost, len(boosted)


def _format_timestamp(value: Any) -> str:
    if isinstance(value, datetime):
        return value.astimezone(timezone.utc).isoformat()
    if value is None:
        return ""
    return str(value)


def build_graph_boost_snapshot_entries(
    rows: Sequence[dict[str, Any]],
    graph_margin: float = DEFAULT_GRAPH_MARGIN,
) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for row in rows:
        candidates = _ensure_list(row.get("coarse_candidates"))
        margin, top_boost, candidate_count = _compute_boosted_scores(candidates)
        tag_profile = _ensure_dict(row.get("tag_profile"))
        raw_top_tags = _ensure_list(tag_profile.get("top_tags"))
        tag_labels = [
            tag.get("label")
            for tag in raw_top_tags
            if isinstance(tag.get("label"), str)
        ]
        refine_decision = _ensure_dict(row.get("refine_decision"))
        entry = {
            "job_id": str(row.get("job_id") or ""),
            "article_id": row.get("article_id") or "",
            "created_at": _format_timestamp(row.get("created_at")),
            "final_genre": refine_decision.get("final_genre") or "",
            "strategy": refine_decision.get("strategy") or "",
            "margin": round(margin, 6),
            "top_boost": round(top_boost, 6),
            "graph_boost_available": bool(margin >= graph_margin and top_boost > 0.0),
            "tag_count": len(tag_labels),
            "candidate_count": candidate_count,
            "tag_entropy": float(tag_profile.get("entropy"))
            if isinstance(tag_profile.get("entropy"), (int, float))
            else None,
            "top_tags": tag_labels,
            "confidence": _ensure_confidence(refine_decision.get("confidence")),
        }
        entries.append(entry)
    return entries


class ClusterBuilder:
    def __init__(self, max_clusters: int = DEFAULT_CLUSTER_MAX_K, random_state: int = 42):
        self.max_clusters = max(2, max_clusters)
        self.random_state = random_state

    def build(
        self,
        entries: Sequence[dict[str, Any]],
        genres: Sequence[str],
        min_samples: int = DEFAULT_CLUSTER_MIN_SAMPLES,
    ) -> dict[str, Any] | None:
        genre_map: dict[str, list[dict[str, Any]]] = {genre: [] for genre in genres}
        for entry in entries:
            final_genre = entry.get("final_genre")
            if final_genre in genre_map:
                genre_map[final_genre].append(entry)

        summaries: list[dict[str, Any]] = []
        total_clustered = 0
        for genre, samples in genre_map.items():
            if len(samples) < min_samples:
                continue
            cluster_summary = self._cluster_genre(genre, samples)
            if cluster_summary:
                summaries.append(cluster_summary)
                total_clustered += len(samples)

        if not summaries:
            return None

        now = datetime.now(timezone.utc)
        draft_id = f"graph-boost-reorg-{now.strftime('%Y%m%dT%H%M%SZ')}"
        return {
            "draft_id": draft_id,
            "description": "Auto-generated genre reorganization draft based on Graph Boost snapshots.",
            "source": "recap-subworker",
            "generated_at": now.isoformat(),
            "total_entries": total_clustered,
            "genres": summaries,
        }

    def _cluster_genre(
        self,
        genre: str,
        entries: list[dict[str, Any]],
    ) -> dict[str, Any] | None:
        feature_matrix: list[list[float]] = []
        for entry in entries:
            feature_matrix.append(
                [
                    float(entry.get("margin") or 0.0),
                    float(entry.get("top_boost") or 0.0),
                    float(entry.get("tag_count") or 0),
                    float(entry.get("candidate_count") or 0),
                    float(entry.get("tag_entropy") or 0.0),
                    1.0 if entry.get("graph_boost_available") else 0.0,
                ]
            )

        if len(feature_matrix) < 2:
            return None

        features = np.asarray(feature_matrix, dtype=float)
        scaled = StandardScaler().fit_transform(features)

        max_k = min(self.max_clusters, len(entries) - 1)
        best_k = 1
        best_score = -1.0
        for k in range(2, max_k + 1):
            kmeans = KMeans(n_clusters=k, random_state=self.random_state, n_init=10)
            labels = kmeans.fit_predict(scaled)
            if len(set(labels)) < 2:
                continue
            try:
                score = silhouette_score(scaled, labels)
            except ValueError:
                continue
            if score > best_score:
                best_score = score
                best_k = k

        if best_k > 1:
            final_model = KMeans(n_clusters=best_k, random_state=self.random_state, n_init=10)
            labels = final_model.fit_predict(scaled)
        else:
            labels = [0] * len(entries)

        clusters: list[dict[str, Any]] = []
        for label in sorted(set(labels)):
            cluster_entries = [
                entry
                for entry, assigned in zip(entries, labels)
                if assigned == label
            ]
            if not cluster_entries:
                continue
            clusters.append(self._summarize_cluster(genre, label, cluster_entries))

        if not clusters:
            return None

        return {
            "genre": genre,
            "sample_size": len(entries),
            "cluster_count": len(clusters),
            "clusters": clusters,
        }

    def _summarize_cluster(
        self,
        genre: str,
        label: int,
        entries: list[dict[str, Any]],
    ) -> dict[str, Any]:
        margins = [float(entry.get("margin") or 0.0) for entry in entries]
        top_boosts = [float(entry.get("top_boost") or 0.0) for entry in entries]
        tag_counts = [float(entry.get("tag_count") or 0) for entry in entries]
        entropies = [float(entry.get("tag_entropy") or 0.0) for entry in entries]
        graph_boost_flags = [1.0 if entry.get("graph_boost_available") else 0.0 for entry in entries]

        tag_counter: Counter[str] = Counter()
        for entry in entries:
            for tag in entry.get("top_tags", []):
                if isinstance(tag, str):
                    tag_counter[tag] += 1

        representative = sorted(
            entries,
            key=lambda row: float(row.get("margin") or 0.0),
            reverse=True,
        )[:3]

        return {
            "cluster_id": f"{genre}-cluster-{label}",
            "label": f"Cluster {label + 1}",
            "count": len(entries),
            "margin_mean": float(statistics.mean(margins)),
            "margin_std": float(statistics.pstdev(margins)),
            "top_boost_mean": float(statistics.mean(top_boosts)),
            "graph_boost_available_ratio": sum(graph_boost_flags) / len(graph_boost_flags),
            "tag_count_mean": float(statistics.mean(tag_counts)),
            "tag_entropy_mean": float(statistics.mean(entropies)),
            "top_terms": [term for term, _ in tag_counter.most_common(5)],
            "representative": representative,
        }


@dataclass(frozen=True)
class GraphBoostParams:
    """Parameter triple that controls Graph Boost filtering."""

    graph_margin: float
    boost_threshold: float
    tag_count_threshold: int


def _prepare_dataframe_from_entries(entries: list[dict[str, Any]]) -> pd.DataFrame:
    """Convert entries list to pandas DataFrame for Bayes optimization."""
    if not entries:
        return pd.DataFrame()

    df = pd.DataFrame(entries)
    # Filter out rows with missing required fields
    df = df.dropna(subset=["margin", "top_boost", "tag_count", "strategy"]).copy()
    df = df[df["strategy"].isin({"graph_boost", "weighted_score"})]
    df = df.assign(
        label=df["strategy"] == "graph_boost",
        margin=df["margin"].astype(float),
        top_boost=df["top_boost"].astype(float),
        tag_count=df["tag_count"].astype(int),
    )
    return df


def _objective_bayes(params: Sequence[float], df: pd.DataFrame) -> float:
    """Objective function for Bayes optimization (minimize 1 - accuracy)."""
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
    """Convert raw optimization parameters to GraphBoostParams."""
    return GraphBoostParams(
        graph_margin=float(raw[0]),
        boost_threshold=float(raw[1]),
        tag_count_threshold=int(round(raw[2])),
    )


def run_bayes_optimization(
    df: pd.DataFrame, iterations: int, seed: int
) -> tuple[GraphBoostParams, float]:
    """Execute gp_minimize over the Graph Boost entries.

    Returns:
        Tuple of (best_params, best_accuracy)
    """
    space = [
        Real(0.05, 0.25, name="graph_margin"),
        Real(0.0, 5.0, name="boost_threshold"),
        Integer(0, 10, name="tag_count_threshold"),
    ]

    result = gp_minimize(
        func=lambda params: _objective_bayes(params, df),
        dimensions=space,
        n_calls=iterations,
        random_state=seed,
        acq_func="EI",
    )

    best_params = _params_from_raw(result.x)
    best_accuracy = 1.0 - result.fun
    return best_params, best_accuracy


@dataclass
class GenreLearningSummary:
    total_records: int
    graph_boost_count: int
    graph_boost_percentage: float
    avg_margin: float | None
    avg_top_boost: float | None
    avg_confidence: float | None
    tag_coverage_pct: float
    graph_margin_reference: float
    boost_threshold_reference: float | None = None
    tag_count_threshold_reference: int | None = None
    accuracy_estimate: float | None = None


@dataclass
class GenreLearningResult:
    summary: GenreLearningSummary
    entries: list[dict[str, Any]]
    cluster_draft: dict[str, Any] | None


class GenreLearningService:
    def __init__(
        self,
        session: AsyncSession,
        graph_margin: float = DEFAULT_GRAPH_MARGIN,
        cluster_genres: Sequence[str] | None = None,
        bayes_enabled: bool = True,
        bayes_iterations: int = DEFAULT_BAYES_ITERATIONS,
        bayes_seed: int = DEFAULT_BAYES_SEED,
        bayes_min_samples: int = DEFAULT_BAYES_MIN_SAMPLES,
    ) -> None:
        self.session = session
        self.graph_margin = graph_margin
        self.cluster_genres = cluster_genres or DEFAULT_CLUSTER_GENRES
        self.bayes_enabled = bayes_enabled
        self.bayes_iterations = bayes_iterations
        self.bayes_seed = bayes_seed
        self.bayes_min_samples = bayes_min_samples

    async def fetch_snapshot_rows(
        self,
        hours: int = DEFAULT_SNAPSHOT_HOURS,
        limit: int = DEFAULT_SNAPSHOT_LIMIT,
    ) -> list[dict[str, Any]]:
        import structlog
        logger = structlog.get_logger(__name__)

        logger.debug(
            "fetching snapshot rows",
            hours=hours,
            limit=limit,
        )
        query = text(
            """
            SELECT job_id,
                   article_id,
                   created_at,
                   refine_decision,
                   coarse_candidates,
                   tag_profile
            FROM recap_genre_learning_results
            WHERE created_at > NOW() - INTERVAL '1 hour' * :hours
            ORDER BY created_at DESC
            LIMIT :limit
            """
        )
        result = await self.session.execute(query, {"hours": hours, "limit": limit})
        rows = [dict(row) for row in result.mappings().all()]
        logger.info(
            "fetched snapshot rows",
            row_count=len(rows),
            hours=hours,
            limit=limit,
        )
        return rows

    async def generate_learning_result(
        self,
        hours: int = DEFAULT_SNAPSHOT_HOURS,
        limit: int = DEFAULT_SNAPSHOT_LIMIT,
    ) -> GenreLearningResult:
        import structlog
        logger = structlog.get_logger(__name__)

        rows = await self.fetch_snapshot_rows(hours=hours, limit=limit)

        if not rows:
            logger.warning(
                "no snapshot rows found",
                hours=hours,
                limit=limit,
            )
            # Return empty result
            empty_summary = GenreLearningSummary(
                total_records=0,
                graph_boost_count=0,
                graph_boost_percentage=0.0,
                avg_margin=None,
                avg_top_boost=None,
                avg_confidence=None,
                tag_coverage_pct=0.0,
                graph_margin_reference=self.graph_margin,
                boost_threshold_reference=None,
                tag_count_threshold_reference=None,
                accuracy_estimate=None,
            )
            return GenreLearningResult(
                summary=empty_summary,
                entries=[],
                cluster_draft=None,
            )

        logger.debug(
            "building graph boost snapshot entries",
            row_count=len(rows),
        )
        entries = build_graph_boost_snapshot_entries(rows, self.graph_margin)
        logger.debug(
            "summarizing entries",
            entry_count=len(entries),
        )
        summary = self._summarize_entries(entries)

        # Run Bayes optimization if enabled and sufficient samples
        if self.bayes_enabled and len(entries) >= self.bayes_min_samples:
            logger.info(
                "running Bayes optimization",
                entry_count=len(entries),
                iterations=self.bayes_iterations,
            )
            try:
                df = _prepare_dataframe_from_entries(entries)
                if len(df) >= self.bayes_min_samples:
                    best_params, best_accuracy = run_bayes_optimization(
                        df, self.bayes_iterations, self.bayes_seed
                    )
                    # Check if top_boost is all zeros
                    has_boost_values = (df["top_boost"] > 0).any()
                    if not has_boost_values:
                        logger.warning(
                            "top_boost is all zeros, fixing boost_threshold to 0",
                        )
                        best_params = GraphBoostParams(
                            graph_margin=best_params.graph_margin,
                            boost_threshold=0.0,
                            tag_count_threshold=best_params.tag_count_threshold,
                        )

                    summary.boost_threshold_reference = best_params.boost_threshold
                    summary.tag_count_threshold_reference = best_params.tag_count_threshold
                    summary.accuracy_estimate = best_accuracy
                    # Update graph_margin_reference with optimized value
                    summary.graph_margin_reference = best_params.graph_margin

                    logger.info(
                        "Bayes optimization completed",
                        graph_margin=best_params.graph_margin,
                        boost_threshold=best_params.boost_threshold,
                        tag_count_threshold=best_params.tag_count_threshold,
                        accuracy_estimate=best_accuracy,
                    )
                else:
                    logger.warning(
                        "insufficient samples for Bayes optimization after filtering",
                        filtered_count=len(df),
                        min_samples=self.bayes_min_samples,
                    )
            except Exception as exc:
                logger.error(
                    "Bayes optimization failed",
                    error=str(exc),
                    error_type=type(exc).__name__,
                    exc_info=True,
                )
                # Continue with default values (graph_margin_reference only)
        else:
            if not self.bayes_enabled:
                logger.debug("Bayes optimization disabled")
            else:
                logger.debug(
                    "insufficient samples for Bayes optimization",
                    entry_count=len(entries),
                    min_samples=self.bayes_min_samples,
                )

        logger.debug(
            "building cluster draft",
            cluster_genres=self.cluster_genres,
        )
        cluster_draft = ClusterBuilder().build(
            entries,
            genres=self.cluster_genres,
            min_samples=DEFAULT_CLUSTER_MIN_SAMPLES,
        )
        if cluster_draft:
            logger.debug(
                "cluster draft created",
                draft_id=cluster_draft.get("draft_id"),
                total_entries=cluster_draft.get("total_entries"),
            )
        else:
            logger.debug("no cluster draft created (insufficient samples)")

        return GenreLearningResult(summary=summary, entries=entries, cluster_draft=cluster_draft)

    def _summarize_entries(self, entries: list[dict[str, Any]]) -> GenreLearningSummary:
        total = len(entries)
        graph_boost_count = sum(1 for entry in entries if entry.get("strategy") == "graph_boost")
        graph_boost_percentage = (graph_boost_count / total * 100) if total else 0.0
        margins = [float(entry.get("margin") or 0.0) for entry in entries]
        top_boosts = [float(entry.get("top_boost") or 0.0) for entry in entries]
        confidences = [entry.get("confidence") for entry in entries if entry.get("confidence") is not None]
        tag_counts = [entry.get("tag_count") or 0 for entry in entries]
        tag_coverage_pct = (
            (sum(1 for count in tag_counts if count > 0) / total) * 100 if total else 0.0
        )
        return GenreLearningSummary(
            total_records=total,
            graph_boost_count=graph_boost_count,
            graph_boost_percentage=round(graph_boost_percentage, 2),
            avg_margin=float(statistics.mean(margins)) if margins else None,
            avg_top_boost=float(statistics.mean(top_boosts)) if top_boosts else None,
            avg_confidence=float(statistics.mean(confidences)) if confidences else None,
            tag_coverage_pct=round(tag_coverage_pct, 2),
            graph_margin_reference=self.graph_margin,
            boost_threshold_reference=None,  # Will be set by Bayes optimization if enabled
            tag_count_threshold_reference=None,  # Will be set by Bayes optimization if enabled
            accuracy_estimate=None,  # Will be set by Bayes optimization if enabled
        )

