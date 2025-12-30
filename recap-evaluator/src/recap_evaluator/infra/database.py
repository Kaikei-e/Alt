"""Database connection pool and operations for recap-db."""

from contextlib import asynccontextmanager
from datetime import datetime
from typing import Any
from uuid import UUID

import asyncpg
import structlog

from recap_evaluator.config import settings

logger = structlog.get_logger()


class Database:
    """Async database connection pool manager."""

    def __init__(self) -> None:
        self._pool: asyncpg.Pool | None = None

    async def connect(self) -> None:
        """Create database connection pool."""
        if self._pool is not None:
            return

        self._pool = await asyncpg.create_pool(
            dsn=settings.recap_db_dsn,
            min_size=settings.db_pool_min_size,
            max_size=settings.db_pool_max_size,
        )
        logger.info("Database connection pool created")

    async def disconnect(self) -> None:
        """Close database connection pool."""
        if self._pool is not None:
            await self._pool.close()
            self._pool = None
            logger.info("Database connection pool closed")

    @asynccontextmanager
    async def acquire(self):
        """Acquire a connection from the pool."""
        if self._pool is None:
            msg = "Database pool not initialized. Call connect() first."
            raise RuntimeError(msg)

        async with self._pool.acquire() as conn:
            yield conn

    async def fetch_recent_jobs(
        self,
        days: int = 14,
        status: str = "completed",
    ) -> list[dict[str, Any]]:
        """Fetch recent recap jobs within the specified window."""
        query = """
            SELECT job_id, kicked_at, status, last_stage, note, updated_at
            FROM recap_jobs
            WHERE kicked_at >= NOW() - INTERVAL '%s days'
            AND status = $1
            ORDER BY kicked_at DESC
        """
        async with self.acquire() as conn:
            rows = await conn.fetch(query % days, status)
            return [dict(row) for row in rows]

    async def fetch_job_articles(self, job_id: UUID) -> list[dict[str, Any]]:
        """Fetch articles for a specific job."""
        query = """
            SELECT article_id, title, fulltext_html, published_at, source_url, lang_hint
            FROM recap_job_articles
            WHERE job_id = $1
        """
        async with self.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_preprocess_metrics(self, job_id: UUID) -> dict[str, Any] | None:
        """Fetch preprocessing metrics for a job."""
        query = """
            SELECT total_articles_fetched, articles_processed, articles_dropped_empty,
                   total_characters, avg_chars_per_article, languages_detected
            FROM recap_preprocess_metrics
            WHERE job_id = $1
        """
        async with self.acquire() as conn:
            row = await conn.fetchrow(query, job_id)
            return dict(row) if row else None

    async def fetch_subworker_runs(self, job_id: UUID) -> list[dict[str, Any]]:
        """Fetch subworker run results for a job."""
        query = """
            SELECT run_id, genre, status, cluster_count, started_at, finished_at,
                   request_payload, response_payload, error_message
            FROM recap_subworker_runs
            WHERE job_id = $1
            ORDER BY genre
        """
        async with self.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_clusters_for_run(self, run_id: UUID) -> list[dict[str, Any]]:
        """Fetch cluster details for a subworker run."""
        query = """
            SELECT cluster_id, size, label, top_terms, stats
            FROM recap_subworker_clusters
            WHERE run_id = $1
            ORDER BY cluster_id
        """
        async with self.acquire() as conn:
            rows = await conn.fetch(query, run_id)
            return [dict(row) for row in rows]

    async def fetch_outputs(self, job_id: UUID) -> list[dict[str, Any]]:
        """Fetch recap outputs (summaries) for a job."""
        query = """
            SELECT genre, response_id, title_ja, summary_ja, bullets_ja, body_json,
                   created_at, updated_at
            FROM recap_outputs
            WHERE job_id = $1
            ORDER BY genre
        """
        async with self.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_stage_logs(self, job_id: UUID) -> list[dict[str, Any]]:
        """Fetch stage execution logs for a job."""
        query = """
            SELECT stage, status, started_at, finished_at, message
            FROM recap_job_stage_logs
            WHERE job_id = $1
            ORDER BY started_at
        """
        async with self.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def fetch_genre_learning_results(
        self,
        job_id: UUID,
    ) -> list[dict[str, Any]]:
        """Fetch genre classification learning results for a job."""
        query = """
            SELECT article_id, coarse_candidates, refine_decision,
                   tag_profile, graph_context
            FROM recap_genre_learning_results
            WHERE job_id = $1
        """
        async with self.acquire() as conn:
            rows = await conn.fetch(query, job_id)
            return [dict(row) for row in rows]

    async def save_evaluation_run(
        self,
        evaluation_id: UUID,
        evaluation_type: str,
        job_ids: list[UUID],
        metrics: dict[str, Any],
        created_at: datetime,
    ) -> None:
        """Save evaluation run results."""
        query = """
            INSERT INTO recap_evaluation_runs
            (evaluation_id, evaluation_type, job_ids, metrics, created_at)
            VALUES ($1, $2, $3, $4, $5)
        """
        async with self.acquire() as conn:
            await conn.execute(
                query,
                evaluation_id,
                evaluation_type,
                job_ids,
                metrics,
                created_at,
            )

    async def fetch_evaluation_history(
        self,
        evaluation_type: str | None = None,
        limit: int = 30,
    ) -> list[dict[str, Any]]:
        """Fetch evaluation run history."""
        if evaluation_type:
            query = """
                SELECT evaluation_id, evaluation_type, job_ids, metrics, created_at
                FROM recap_evaluation_runs
                WHERE evaluation_type = $1
                ORDER BY created_at DESC
                LIMIT $2
            """
            params = [evaluation_type, limit]
        else:
            query = """
                SELECT evaluation_id, evaluation_type, job_ids, metrics, created_at
                FROM recap_evaluation_runs
                ORDER BY created_at DESC
                LIMIT $1
            """
            params = [limit]

        async with self.acquire() as conn:
            rows = await conn.fetch(query, *params)
            return [dict(row) for row in rows]


# Singleton database instance
db = Database()
