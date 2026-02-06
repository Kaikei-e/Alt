"""PostgreSQL gateway — implements DatabasePort for recap-db."""

from datetime import datetime
from typing import Any
from uuid import UUID

import asyncpg
import structlog

logger = structlog.get_logger()


class PostgresGateway:
    """asyncpg-based implementation of DatabasePort."""

    def __init__(self, pool: asyncpg.Pool) -> None:
        self._pool = pool

    async def fetch_recent_jobs(
        self, days: int, status: str = "completed"
    ) -> list[dict[str, Any]]:
        query = """
            SELECT job_id, kicked_at, status, last_stage, note, updated_at
            FROM recap_jobs
            WHERE kicked_at >= NOW() - make_interval(days => $1)
            AND status = $2
            ORDER BY kicked_at DESC
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, days, status)
            return [dict(row) for row in rows]

    async def fetch_job_articles(self, job_id: UUID) -> list[dict[str, Any]]:
        query = """
            SELECT article_id, title, LEFT(fulltext_html, 500) AS fulltext_html,
                   published_at, source_url, lang_hint
            FROM recap_job_articles
            WHERE job_id = $1
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_outputs(self, job_id: UUID) -> list[dict[str, Any]]:
        query = """
            SELECT genre, response_id, title_ja, summary_ja, bullets_ja, body_json,
                   created_at, updated_at
            FROM recap_outputs
            WHERE job_id = $1
            ORDER BY genre
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_stage_logs(self, job_id: UUID) -> list[dict[str, Any]]:
        query = """
            SELECT stage, status, started_at, finished_at, message
            FROM recap_job_stage_logs
            WHERE job_id = $1
            ORDER BY started_at
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_stage_logs_batch(
        self, job_ids: list[UUID]
    ) -> dict[UUID, list[dict[str, Any]]]:
        query = """
            SELECT job_id, stage, status, started_at, finished_at, message
            FROM recap_job_stage_logs
            WHERE job_id = ANY($1)
            ORDER BY job_id, started_at
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, job_ids)

        result: dict[UUID, list[dict[str, Any]]] = {}
        for row in rows:
            jid = row["job_id"]
            if jid not in result:
                result[jid] = []
            result[jid].append(dict(row))
        return result

    async def fetch_preprocess_metrics(self, job_id: UUID) -> dict[str, Any] | None:
        query = """
            SELECT total_articles_fetched, articles_processed, articles_dropped_empty,
                   total_characters, avg_chars_per_article, languages_detected
            FROM recap_preprocess_metrics
            WHERE job_id = $1
        """
        async with self._pool.acquire() as conn:
            row = await conn.fetchrow(query, job_id)
            return dict(row) if row else None

    async def fetch_preprocess_metrics_batch(
        self, job_ids: list[UUID]
    ) -> dict[UUID, dict[str, Any]]:
        query = """
            SELECT job_id, total_articles_fetched, articles_processed,
                   articles_dropped_empty, total_characters, avg_chars_per_article,
                   languages_detected
            FROM recap_preprocess_metrics
            WHERE job_id = ANY($1)
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, job_ids)

        return {row["job_id"]: dict(row) for row in rows}

    async def fetch_subworker_runs(self, job_id: UUID) -> list[dict[str, Any]]:
        query = """
            SELECT id as run_id, genre, status, cluster_count, started_at, finished_at,
                   request_payload, response_payload, error_message
            FROM recap_subworker_runs
            WHERE job_id = $1
            ORDER BY genre
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_clusters_for_run(self, run_id: UUID) -> list[dict[str, Any]]:
        query = """
            SELECT cluster_id, size, label, top_terms, stats
            FROM recap_subworker_clusters
            WHERE run_id = $1
            ORDER BY cluster_id
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, run_id)
            return [dict(row) for row in rows]

    async def fetch_genre_learning_results(
        self, job_id: UUID
    ) -> list[dict[str, Any]]:
        query = """
            SELECT article_id, coarse_candidates, refine_decision,
                   tag_profile, graph_context
            FROM recap_genre_learning_results
            WHERE job_id = $1
        """
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_evaluation_by_id(
        self, evaluation_id: UUID
    ) -> dict[str, Any] | None:
        query = """
            SELECT evaluation_id, evaluation_type, job_ids, metrics, created_at
            FROM recap_evaluation_runs
            WHERE evaluation_id = $1
        """
        async with self._pool.acquire() as conn:
            row = await conn.fetchrow(query, evaluation_id)
            return dict(row) if row else None

    async def fetch_evaluation_history(
        self, evaluation_type: str | None = None, limit: int = 30
    ) -> list[dict[str, Any]]:
        if evaluation_type:
            query = """
                SELECT evaluation_id, evaluation_type, job_ids, metrics, created_at
                FROM recap_evaluation_runs
                WHERE evaluation_type = $1
                ORDER BY created_at DESC
                LIMIT $2
            """
            params: list[Any] = [evaluation_type, limit]
        else:
            query = """
                SELECT evaluation_id, evaluation_type, job_ids, metrics, created_at
                FROM recap_evaluation_runs
                ORDER BY created_at DESC
                LIMIT $1
            """
            params = [limit]

        async with self._pool.acquire() as conn:
            rows = await conn.fetch(query, *params)
            return [dict(row) for row in rows]

    async def save_evaluation_run(
        self,
        evaluation_id: UUID,
        evaluation_type: str,
        job_ids: list[UUID],
        metrics: dict[str, Any],
        created_at: datetime,
    ) -> None:
        query = """
            INSERT INTO recap_evaluation_runs
            (evaluation_id, evaluation_type, job_ids, metrics, created_at)
            VALUES ($1, $2, $3, $4, $5)
        """
        async with self._pool.acquire() as conn:
            await conn.execute(
                query, evaluation_id, evaluation_type, job_ids, metrics, created_at
            )
