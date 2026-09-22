"""Story clustering service for cross-cutting topic cards.

Implements agglomerative clustering with pairwise cosine distance,
time decay penalty, and re-normalized centroid computation.
"""

from __future__ import annotations

import math
from collections import Counter
from datetime import UTC, datetime

import numpy as np
from pydantic import BaseModel, Field, field_validator
from sklearn.cluster import AgglomerativeClustering

VALID_LINKAGES = {"average", "complete", "single"}


class ClusterStoryItem(BaseModel):
    """Single item for story clustering."""

    id: str = Field(..., description="UUID identifier of the feed or article item")
    embedding: list[float] = Field(..., description="Dense vector embedding")
    published_at: datetime = Field(
        ...,
        description="RFC3339 timestamp when the item was published",
    )
    language: str | None = Field(
        default=None,
        description="Optional language code of item (e.g. 'ja', 'en')",
    )

    @field_validator("published_at")
    @classmethod
    def validate_published_at(cls, v: datetime) -> datetime:
        if v.tzinfo is None:
            return v.replace(tzinfo=UTC)
        return v


class StoryClusterParams(BaseModel):
    """Clustering hyperparameters."""

    threshold: float = Field(
        default=0.78,
        description="Similarity merge threshold in (0, 1]. distance_threshold = 1 - threshold",
    )
    linkage: str = Field(
        default="average",
        description="Agglomerative clustering linkage: 'average', 'complete', or 'single'",
    )
    time_decay_per_day: float = Field(
        default=0.02,
        description="Distance penalty added per day difference between publication timestamps",
    )
    min_cluster_size: int = Field(
        default=1,
        description="Minimum cluster size to keep (singletons are preserved when 1)",
    )
    min_cluster_size_by_language: dict[str, int] | None = Field(
        default=None,
        description="Optional per-language minimum cluster sizes, e.g. {'ja': 2, 'en': 3}",
    )

    @field_validator("threshold")
    @classmethod
    def validate_threshold(cls, v: float) -> float:
        if math.isnan(v) or v <= 0.0 or v > 1.0:
            raise ValueError("threshold must be in (0, 1]")
        return v

    @field_validator("linkage")
    @classmethod
    def validate_linkage(cls, v: str) -> str:
        if v not in VALID_LINKAGES:
            raise ValueError(f"unknown linkage: '{v}'. Must be one of {sorted(VALID_LINKAGES)}")
        return v

    @field_validator("time_decay_per_day")
    @classmethod
    def validate_time_decay(cls, v: float) -> float:
        if math.isnan(v) or v < 0.0:
            raise ValueError("time_decay_per_day must be >= 0")
        return v

    @field_validator("min_cluster_size")
    @classmethod
    def validate_min_cluster_size(cls, v: int) -> int:
        if v < 1:
            raise ValueError("min_cluster_size must be >= 1")
        return v

    @field_validator("min_cluster_size_by_language")
    @classmethod
    def validate_min_cluster_size_by_language(
        cls, v: dict[str, int] | None
    ) -> dict[str, int] | None:
        if v is not None:
            for lang, size in v.items():
                if size < 1:
                    raise ValueError(f"min_cluster_size for '{lang}' must be >= 1")
        return v


class ClusterStoriesRequest(BaseModel):
    """Request payload for /v1/cluster-stories."""

    items: list[ClusterStoryItem] = Field(
        ...,
        description="Items with embeddings and publication timestamps to cluster",
    )
    params: StoryClusterParams = Field(
        default_factory=StoryClusterParams,
        description="Clustering parameters",
    )

    @field_validator("items")
    @classmethod
    def validate_items(cls, items: list[ClusterStoryItem]) -> list[ClusterStoryItem]:
        if not items:
            raise ValueError("items must contain at least 1 item")

        expected_dim = len(items[0].embedding)
        if expected_dim == 0:
            raise ValueError("embedding dimension must be greater than 0")

        for idx, item in enumerate(items):
            if len(item.embedding) != expected_dim:
                raise ValueError(
                    f"embedding dimension mismatch at index {idx}: expected {expected_dim}, "
                    f"got {len(item.embedding)}"
                )
            if not all(math.isfinite(x) for x in item.embedding):
                raise ValueError(f"embedding at index {idx} contains non-finite values")

        return items


class ClusterOutput(BaseModel):
    """Single output cluster."""

    cluster_id: int
    member_ids: list[str]
    centroid: list[float]


class ClusterStoriesResponse(BaseModel):
    """Response payload for /v1/cluster-stories."""

    clusters: list[ClusterOutput]
    params: StoryClusterParams


class StoryClustererService:
    """Service performing agglomerative clustering on story embeddings."""

    @staticmethod
    def _effective_min_cluster_size(
        member_indices: list[int],
        items: list[ClusterStoryItem],
        params: StoryClusterParams,
    ) -> int:
        """Resolve minimum cluster size based on majority language of members."""
        if not params.min_cluster_size_by_language:
            return params.min_cluster_size

        # Lower-case the keys of min_cluster_size_by_language when comparing
        min_by_lang = {k.strip().lower(): v for k, v in params.min_cluster_size_by_language.items()}

        langs: list[str] = []
        for idx in member_indices:
            lang = items[idx].language
            if lang and lang.strip():
                langs.append(lang.strip().lower())
        if not langs:
            return params.min_cluster_size

        # Canonical tie-break: count descending, then language alphabetically ascending
        counts = Counter(langs)
        majority_lang = min(counts.keys(), key=lambda lang: (-counts[lang], lang))
        return min_by_lang.get(majority_lang, params.min_cluster_size)

    def cluster_stories(
        self,
        items: list[ClusterStoryItem],
        params: StoryClusterParams,
    ) -> ClusterStoriesResponse:
        """Cluster story items based on embedding cosine distance and publication time decay.

        Args:
            items: Non-empty list of items with embeddings and publication times.
            params: Clustering parameters.

        Returns:
            ClusterStoriesResponse containing sorted clusters and echoed effective params.
        """
        n = len(items)
        if n == 0:
            return ClusterStoriesResponse(clusters=[], params=params)

        raw_embeddings = np.array([item.embedding for item in items], dtype=np.float32)
        norms = np.linalg.norm(raw_embeddings, axis=1, keepdims=True)
        norms = np.where(norms == 0.0, 1.0, norms)
        normed_embeddings = raw_embeddings / norms

        if n == 1:
            cluster = ClusterOutput(
                cluster_id=0,
                member_ids=[items[0].id],
                centroid=normed_embeddings[0].tolist(),
            )
            min_size = self._effective_min_cluster_size([0], items, params)
            clusters = [cluster] if min_size <= 1 else []
            return ClusterStoriesResponse(clusters=clusters, params=params)

        # 1. Pairwise cosine distance: d = 1 - cos on normalized embeddings
        cos_sim = np.dot(normed_embeddings, normed_embeddings.T)
        cos_sim = np.clip(cos_sim, -1.0, 1.0)
        cos_dist = 1.0 - cos_sim

        # 2. Time penalty: time_decay_per_day * |Δdays|, keeping >= 0 (no upper clip at 1.0)
        timestamps = np.array([item.published_at.timestamp() for item in items], dtype=np.float64)
        delta_seconds = np.abs(timestamps[:, None] - timestamps[None, :])
        delta_days = delta_seconds / 86400.0

        time_penalty = params.time_decay_per_day * delta_days
        dist_matrix = cos_dist + time_penalty
        dist_matrix = np.maximum(dist_matrix, 0.0)

        # Ensure exact symmetry and 0 diagonal
        dist_matrix = (dist_matrix + dist_matrix.T) / 2.0
        np.fill_diagonal(dist_matrix, 0.0)

        # 3. Agglomerative clustering with distance_threshold = 1 - threshold
        distance_threshold = 1.0 - params.threshold
        clustering = AgglomerativeClustering(
            n_clusters=None,
            metric="precomputed",
            linkage=params.linkage,
            distance_threshold=distance_threshold,
        )
        labels = clustering.fit_predict(dist_matrix)

        # 4. Group members by cluster label
        groups: dict[int, list[int]] = {}
        for idx, label in enumerate(labels):
            groups.setdefault(int(label), []).append(idx)

        # 5. Extract clusters: singletons are size-1 clusters (no noise bucket)
        raw_clusters: list[tuple[list[str], list[float]]] = []
        for member_indices in groups.values():
            effective_min = self._effective_min_cluster_size(member_indices, items, params)
            if len(member_indices) < effective_min:
                continue

            member_ids = sorted(items[idx].id for idx in member_indices)

            # Centroid = mean of member embeddings re-normalised
            member_vecs = normed_embeddings[member_indices]
            mean_vec = np.mean(member_vecs, axis=0)
            c_norm = np.linalg.norm(mean_vec)
            centroid = (mean_vec / c_norm).tolist() if c_norm > 0.0 else mean_vec.tolist()

            raw_clusters.append((member_ids, centroid))

        # 6. Sort clusters by size desc then by smallest member id asc
        raw_clusters.sort(key=lambda c: (-len(c[0]), c[0][0]))

        # 7. Assign sequential cluster_id
        output_clusters = [
            ClusterOutput(
                cluster_id=i,
                member_ids=member_ids,
                centroid=centroid,
            )
            for i, (member_ids, centroid) in enumerate(raw_clusters)
        ]

        return ClusterStoriesResponse(
            clusters=output_clusters,
            params=params,
        )
