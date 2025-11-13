#!/usr/bin/env python3
"""Generate tag-to-genre priors for Recap Worker."""

from __future__ import annotations

import argparse
import json
import logging
import os
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Iterable, Sequence

import psycopg2
import psycopg2.extras
from psycopg2.extensions import connection as PGConnection


@dataclass
class LearningRow:
    """Projection of recap_genre_learning_results."""

    genre: str
    tags: list[dict[str, Any]]
    updated_at: datetime


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


def normalize(value: str | None) -> str:
    if not value:
        return ""
    return value.strip().lower()


def aggregate_tag_edges(
    rows: Sequence[LearningRow],
    *,
    max_tags: int,
    min_confidence: float,
    min_support: int,
) -> list[EdgePayload]:
    """Turn per-article tag profiles into aggregated edges."""

    stats: dict[tuple[str, str], _EdgeAccumulator] = {}
    for row in rows:
        genre = normalize(row.genre) or "other"
        tags = row.tags or []
        for tag in tags[:max(0, max_tags)]:
            label = normalize(tag.get("label"))
            if not label:
                continue
            confidence = float(tag.get("confidence") or 0.0)
            if confidence < min_confidence:
                continue
            key = (genre, label)
            acc = stats.setdefault(key, _EdgeAccumulator())
            acc.update(confidence, row.updated_at)

    edges: list[EdgePayload] = []
    for (genre, label), acc in stats.items():
        if acc.sample_size < max(1, min_support):
            continue
        avg_conf = acc.weight_sum / acc.sample_size
        weight = max(0.0, min(1.0, round(avg_conf, 6)))
        edges.append(
            EdgePayload(
                genre=genre,
                tag=label,
                weight=weight,
                sample_size=acc.sample_size,
                last_observed_at=acc.last_observed_at,
            )
        )

    edges.sort(key=lambda edge: (edge.genre, -edge.weight, edge.tag))
    return edges


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build rolling tag-label graph inside recap-db"
    )
    parser.add_argument(
        "--dsn",
        default=os.getenv("RECAP_DB_DSN"),
        help="PostgreSQL DSN pointing at recap-db (can use RECAP_DB_DSN)",
    )
    parser.add_argument(
        "--windows",
        default="7,30",
        help="Comma-separated lookback windows in days (default: 7,30)",
    )
    parser.add_argument(
        "--max-tags",
        type=int,
        default=6,
        help="Maximum number of tags per article to consider (default: 6)",
    )
    parser.add_argument(
        "--min-confidence",
        type=float,
        default=0.55,
        help="Minimum tag confidence required to contribute (default: 0.55)",
    )
    parser.add_argument(
        "--min-support",
        type=int,
        default=3,
        help="Minimum article count required for an edge (default: 3)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Compute weights without mutating the database",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Enable verbose logging",
    )
    return parser.parse_args()


def _connect(dsn: str) -> PGConnection:
    conn = psycopg2.connect(dsn)
    conn.autocommit = False
    return conn


def _fetch_learning_rows(conn: PGConnection, days: int) -> list[LearningRow]:
    query = """
        SELECT
            LOWER(COALESCE(refine_decision->>'final_genre', 'other')) AS genre,
            (tag_profile->'top_tags') AS tags_json,
            updated_at
        FROM recap_genre_learning_results
        WHERE updated_at >= NOW() - (($1)::text || ' days')::interval
    """
    rows: list[LearningRow] = []
    with conn.cursor(cursor_factory=psycopg2.extras.DictCursor) as cur:
        cur.execute(query, (str(days),))
        for record in cur.fetchall():
            tags_raw = record["tags_json"]
            tags: list[dict[str, Any]]
            if isinstance(tags_raw, str):
                try:
                    tags = json.loads(tags_raw)
                except json.JSONDecodeError:
                    tags = []
            else:
                tags = tags_raw or []
            if not isinstance(tags, list):
                tags = []
            rows.append(
                LearningRow(
                    genre=record["genre"],
                    tags=tags,
                    updated_at=_ensure_timezone(record["updated_at"]),
                )
            )
    return rows


def _ensure_timezone(value: datetime | None) -> datetime:
    if value is None:
        return datetime.now(tz=timezone.utc)
    if value.tzinfo is None:
        return value.replace(tzinfo=timezone.utc)
    return value.astimezone(timezone.utc)


def _upsert_edges(
    conn: PGConnection,
    window_label: str,
    edges: Iterable[EdgePayload],
    refresh_ts: datetime,
    dry_run: bool,
) -> int:
    edge_list = list(edges)
    if dry_run:
        logging.info(
            "[DRY RUN] would upsert %d edges for window %s", len(edge_list), window_label
        )
        return len(edge_list)

    if not edge_list:
        logging.warning(
            "No edges generated for %s – keeping previous snapshot intact", window_label
        )
        return 0

    payloads = [
        {
            "window_label": window_label,
            "genre": e.genre,
            "tag": e.tag,
            "weight": e.weight,
            "sample_size": e.sample_size,
            "last_observed_at": e.last_observed_at,
            "refresh_ts": refresh_ts,
        }
        for e in edge_list
    ]

    insert_sql = """
        INSERT INTO tag_label_graph (
            window_label, genre, tag, weight, sample_size, last_observed_at, updated_at
        ) VALUES (
            %(window_label)s, %(genre)s, %(tag)s, %(weight)s, %(sample_size)s,
            %(last_observed_at)s, %(refresh_ts)s
        )
        ON CONFLICT (window_label, genre, tag) DO UPDATE SET
            weight = EXCLUDED.weight,
            sample_size = EXCLUDED.sample_size,
            last_observed_at = COALESCE(EXCLUDED.last_observed_at, tag_label_graph.last_observed_at),
            updated_at = EXCLUDED.updated_at
    """

    with conn.cursor() as cur:
        psycopg2.extras.execute_batch(cur, insert_sql, payloads, page_size=500)
        cur.execute(
            "DELETE FROM tag_label_graph WHERE window_label = %s AND updated_at < %s",
            (window_label, refresh_ts),
        )
        conn.commit()

    logging.info("Upserted %d edges for window %s", len(edge_list), window_label)
    return len(edge_list)


def main() -> int:
    args = _parse_args()
    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    if not args.dsn:
        logging.error("RECAP_DB_DSN must be provided via --dsn or environment")
        return 1

    try:
        windows = [int(item.strip()) for item in args.windows.split(",") if item.strip()]
    except ValueError as exc:
        logging.error("Invalid --windows value: %s", exc)
        return 1

    if not windows:
        logging.error("At least one window duration is required")
        return 1

    conn = _connect(args.dsn)
    try:
        for days in windows:
            logging.info("Building graph for last %d days", days)
            rows = _fetch_learning_rows(conn, days)
            logging.info("Loaded %d learning rows", len(rows))
            edges = aggregate_tag_edges(
                rows,
                max_tags=args.max_tags,
                min_confidence=args.min_confidence,
                min_support=args.min_support,
            )
            refresh_ts = datetime.now(tz=timezone.utc)
            _upsert_edges(
                conn,
                window_label=f"{days}d",
                edges=edges,
                refresh_ts=refresh_ts,
                dry_run=args.dry_run,
            )
    finally:
        conn.close()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
