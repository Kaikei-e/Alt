"""Tag-label graph builder service for recap-subworker."""

from __future__ import annotations

import json
import math
import asyncio
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Sequence

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession

import structlog

logger = structlog.get_logger(__name__)


@dataclass
class EdgePayload:
    """Aggregated tag-to-genre weight."""

    genre: str
    tag: str
    weight: float
    sample_size: int
    last_observed_at: datetime | None


@dataclass
class _EdgeAccumulator:
    weight_sum: float = 0.0
    sample_size: int = 0
    last_observed_at: datetime | None = None

    def update(self, confidence: float, observed_at: datetime) -> None:
        self.weight_sum += confidence
        self.sample_size += 1
        if self.last_observed_at is None or observed_at > self.last_observed_at:
            self.last_observed_at = observed_at


class TagLabelGraphBuilder:
    """Builds tag-label graph from recap_genre_learning_results."""

    def __init__(
        self,
        session: AsyncSession,
        max_tags: int = 6,
        min_confidence: float = 0.3,
        min_support: int = 3,
    ) -> None:
        self.session = session
        self.max_tags = max_tags
        self.min_confidence = min_confidence
        self.min_support = min_support

    async def build_graph(
        self,
        window_days: int,
        window_label: str | None = None,
    ) -> int:
        """Build tag-label graph for specified window."""
        if window_label is None:
            window_label = f"{window_days}d"

        logger.info(
            "building tag-label graph",
            window_days=window_days,
            window_label=window_label,
        )

        # Fetch learning rows
        rows = await self._fetch_learning_rows(window_days)
        logger.info("fetched learning rows", row_count=len(rows))

        if not rows:
            logger.warning("no learning rows found, skipping graph build")
            return 0

        # Aggregate edges
        loop = asyncio.get_running_loop()
        edges = await loop.run_in_executor(None, self._aggregate_edges, rows)
        logger.info("aggregated edges", edge_count=len(edges))

        if not edges:
            logger.warning("no edges generated, skipping upsert")
            return 0

        # Upsert to database
        count = await self._upsert_edges(window_label, edges)
        logger.info(
            "tag-label graph built successfully",
            window_label=window_label,
            edge_count=count,
        )
        return count

    async def _fetch_learning_rows(
        self, days: int
    ) -> list[dict[str, Any]]:
        """Fetch genre learning results with tag profiles.

        Note: Uses published_at (not updated_at) to align with recap-worker's
        7-day window and ensure consistency with fetch_snapshot_rows.
        """
        query = text("""
            SELECT
                LOWER(COALESCE(rglr.refine_decision->>'final_genre', 'other')) AS genre,
                (rglr.tag_profile->'top_tags') AS tags_json,
                rglr.updated_at,
                rja.published_at
            FROM recap_genre_learning_results rglr
            INNER JOIN recap_job_articles rja
                ON rglr.job_id = rja.job_id
                AND rglr.article_id = rja.article_id
            WHERE rja.published_at > NOW() - INTERVAL '1 day' * :days
        """)
        result = await self.session.execute(query, {"days": days})
        return [dict(row._mapping) for row in result.all()]

    def _aggregate_edges(
        self, rows: Sequence[dict[str, Any]]
    ) -> list[EdgePayload]:
        """Aggregate tag-genre co-occurrences into weighted edges."""
        stats: dict[tuple[str, str], _EdgeAccumulator] = defaultdict(
            _EdgeAccumulator
        )

        for row in rows:
            genre = (row.get("genre") or "other").strip().lower()
            tags_raw = row.get("tags_json") or []
            updated_at = row.get("updated_at") or datetime.now(timezone.utc)

            # Handle JSON string if needed
            if isinstance(tags_raw, str):
                try:
                    tags_raw = json.loads(tags_raw)
                except json.JSONDecodeError:
                    tags_raw = []

            if not isinstance(tags_raw, list):
                continue

            for tag in tags_raw[: self.max_tags]:
                if not isinstance(tag, dict):
                    continue
                label = (tag.get("label") or "").strip().lower()
                if not label:
                    continue
                confidence = float(tag.get("confidence") or 0.0)
                if confidence < self.min_confidence:
                    continue

                stats[(genre, label)].update(confidence, updated_at)

        # Calculate total articles and tag frequencies for IDF weighting
        total_articles = len(rows)
        tag_frequencies: dict[str, int] = defaultdict(int)
        for (_, label), acc in stats.items():
            tag_frequencies[label] += acc.sample_size

        max_tag_freq = max(tag_frequencies.values()) if tag_frequencies else 1

        edges: list[EdgePayload] = []
        for (genre, label), acc in stats.items():
            if acc.sample_size < self.min_support:
                continue
            avg_conf = acc.weight_sum / acc.sample_size

            # Apply IDF weighting: rare tags get higher weight
            # IDF = log(total_articles / (articles_with_tag + 1))
            # Normalize to 0.0-1.0 range
            tag_freq = tag_frequencies.get(label, 1)
            idf = math.log(total_articles / (tag_freq + 1) + 1)
            max_idf = math.log(total_articles + 1)
            idf_weight = idf / max_idf if max_idf > 0 else 1.0

            # Combine average confidence with IDF weight
            # Formula: weight = avg_conf * (0.7 + 0.3 * idf_weight)
            # This gives 70% weight to confidence, 30% to IDF
            weight = avg_conf * (0.7 + 0.3 * idf_weight)
            weight = max(0.0, min(1.0, round(weight, 6)))

            edges.append(
                EdgePayload(
                    genre=genre,
                    tag=label,
                    weight=weight,
                    sample_size=acc.sample_size,
                    last_observed_at=acc.last_observed_at,
                )
            )

        edges.sort(key=lambda e: (e.genre, -e.weight, e.tag))
        return edges

    async def _upsert_edges(
        self,
        window_label: str,
        edges: list[EdgePayload],
    ) -> int:
        """Upsert edges to tag_label_graph table."""
        refresh_ts = datetime.now(timezone.utc)

        for edge in edges:
            await self.session.execute(
                text("""
                    INSERT INTO tag_label_graph
                        (window_label, genre, tag, weight, sample_size,
                         last_observed_at, updated_at)
                    VALUES
                        (:window_label, :genre, :tag, :weight, :sample_size,
                         :last_observed_at, :refresh_ts)
                    ON CONFLICT (window_label, genre, tag) DO UPDATE SET
                        weight = EXCLUDED.weight,
                        sample_size = EXCLUDED.sample_size,
                        last_observed_at = COALESCE(
                            EXCLUDED.last_observed_at,
                            tag_label_graph.last_observed_at
                        ),
                        updated_at = EXCLUDED.updated_at
                """),
                {
                    "window_label": window_label,
                    "genre": edge.genre,
                    "tag": edge.tag,
                    "weight": edge.weight,
                    "sample_size": edge.sample_size,
                    "last_observed_at": edge.last_observed_at,
                    "refresh_ts": refresh_ts,
                },
            )

        # Delete stale edges
        await self.session.execute(
            text("""
                DELETE FROM tag_label_graph
                WHERE window_label = :window_label
                  AND updated_at < :refresh_ts
            """),
            {"window_label": window_label, "refresh_ts": refresh_ts},
        )

        await self.session.commit()
        return len(edges)

