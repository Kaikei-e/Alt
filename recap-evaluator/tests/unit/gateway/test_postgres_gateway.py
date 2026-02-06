"""Tests for PostgresGateway."""

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock
from uuid import UUID, uuid4

import pytest

from recap_evaluator.gateway.postgres_gateway import PostgresGateway


@pytest.fixture
def mock_pool():
    """Mock asyncpg pool — acquire() returns sync context manager wrapping async conn."""
    pool = MagicMock()
    conn = AsyncMock()
    cm = AsyncMock()
    cm.__aenter__.return_value = conn
    cm.__aexit__.return_value = False
    pool.acquire.return_value = cm
    return pool, conn


class TestPostgresGateway:
    def test_init_stores_pool(self):
        pool = AsyncMock()
        gw = PostgresGateway(pool)
        assert gw._pool is pool

    async def test_fetch_recent_jobs_uses_parameterized_query(self, mock_pool):
        pool, conn = mock_pool
        conn.fetch.return_value = []
        gw = PostgresGateway(pool)

        result = await gw.fetch_recent_jobs(days=7, status="completed")

        conn.fetch.assert_called_once()
        call_args = conn.fetch.call_args
        # Verify parameterized query: days and status as separate params
        assert call_args[0][1] == 7  # days parameter
        assert call_args[0][2] == "completed"  # status parameter
        # Verify no string interpolation in query
        assert "%s" not in call_args[0][0]
        assert "make_interval" in call_args[0][0]
        assert result == []

    async def test_fetch_job_articles_limits_fulltext(self, mock_pool):
        pool, conn = mock_pool
        conn.fetch.return_value = []
        gw = PostgresGateway(pool)

        await gw.fetch_job_articles(uuid4())

        query = conn.fetch.call_args[0][0]
        assert "LEFT(fulltext_html, 500)" in query

    async def test_fetch_stage_logs_batch_groups_by_job_id(self, mock_pool):
        pool, conn = mock_pool
        job_id_1 = uuid4()
        job_id_2 = uuid4()
        conn.fetch.return_value = [
            {"job_id": job_id_1, "stage": "preprocess", "status": "completed",
             "started_at": None, "finished_at": None, "message": None},
            {"job_id": job_id_1, "stage": "classify", "status": "completed",
             "started_at": None, "finished_at": None, "message": None},
            {"job_id": job_id_2, "stage": "preprocess", "status": "completed",
             "started_at": None, "finished_at": None, "message": None},
        ]
        gw = PostgresGateway(pool)

        result = await gw.fetch_stage_logs_batch([job_id_1, job_id_2])

        assert len(result[job_id_1]) == 2
        assert len(result[job_id_2]) == 1
        query = conn.fetch.call_args[0][0]
        assert "ANY($1)" in query

    async def test_fetch_preprocess_metrics_batch(self, mock_pool):
        pool, conn = mock_pool
        job_id = uuid4()
        conn.fetch.return_value = [
            {"job_id": job_id, "total_articles_fetched": 100,
             "articles_processed": 95, "articles_dropped_empty": 5,
             "total_characters": 500000, "avg_chars_per_article": 5263,
             "languages_detected": {}},
        ]
        gw = PostgresGateway(pool)

        result = await gw.fetch_preprocess_metrics_batch([job_id])

        assert job_id in result
        assert result[job_id]["total_articles_fetched"] == 100

    async def test_fetch_evaluation_by_id_returns_none_when_not_found(self, mock_pool):
        pool, conn = mock_pool
        conn.fetchrow.return_value = None
        gw = PostgresGateway(pool)

        result = await gw.fetch_evaluation_by_id(uuid4())

        assert result is None

    async def test_fetch_evaluation_by_id_returns_dict(self, mock_pool):
        pool, conn = mock_pool
        eval_id = uuid4()
        conn.fetchrow.return_value = {
            "evaluation_id": eval_id,
            "evaluation_type": "full",
            "job_ids": [],
            "metrics": {},
            "created_at": datetime(2025, 1, 1, tzinfo=timezone.utc),
        }
        gw = PostgresGateway(pool)

        result = await gw.fetch_evaluation_by_id(eval_id)

        assert result is not None
        assert result["evaluation_id"] == eval_id

    async def test_save_evaluation_run(self, mock_pool):
        pool, conn = mock_pool
        gw = PostgresGateway(pool)
        eval_id = uuid4()

        await gw.save_evaluation_run(
            evaluation_id=eval_id,
            evaluation_type="full",
            job_ids=[uuid4()],
            metrics={"test": True},
            created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
        )

        conn.execute.assert_called_once()
