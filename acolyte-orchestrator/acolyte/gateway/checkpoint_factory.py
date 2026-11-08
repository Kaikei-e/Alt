"""Checkpoint factory — creates a pool-backed AsyncPostgresSaver for LangGraph durable execution.

Lifecycle: open connection pool at app startup, close at shutdown.
Tables are auto-created by setup().

GHSA-47pj-3jcm-6whg lives in PostgresStore search/list namespace prefix matching.
This factory only constructs AsyncPostgresSaver (checkpoint thread_id isolation).
Keep langgraph-checkpoint-postgres >=3.1.1 so a later store adoption cannot
revive the leak.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

import structlog
from langgraph.checkpoint.postgres.aio import AsyncPostgresSaver
from psycopg.rows import dict_row
from psycopg_pool import AsyncConnectionPool

if TYPE_CHECKING:
    from psycopg import AsyncConnection
    from psycopg.rows import DictRow

logger = structlog.get_logger(__name__)


@asynccontextmanager
async def create_checkpointer(db_dsn: str) -> AsyncIterator[AsyncPostgresSaver]:
    """Create and initialize a pool-backed Postgres checkpointer for LangGraph.

    Usage:
        async with create_checkpointer(dsn) as checkpointer:
            graph = build_report_graph(..., checkpointer=checkpointer)
    """
    # AsyncPostgresSaver._cursor serializes all DB access under self.lock, so at most 1 pooled connection is ever checked out.
    pool: AsyncConnectionPool[AsyncConnection[DictRow]] = AsyncConnectionPool(
        db_dsn,
        min_size=1,
        max_size=1,
        open=False,
        kwargs={
            "autocommit": True,
            "prepare_threshold": 0,
            "row_factory": dict_row,
        },
        check=AsyncConnectionPool.check_connection,
    )
    try:
        await pool.open(wait=True)
        saver = AsyncPostgresSaver(conn=pool)
        await saver.setup()
        logger.info("Postgres checkpointer initialized")
        yield saver
    finally:
        await pool.close()
