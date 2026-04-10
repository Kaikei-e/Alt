"""Checkpoint factory — creates AsyncPostgresSaver for LangGraph durable execution.

Lifecycle: create at app startup, close at shutdown.
Tables are auto-created by setup().
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import structlog

logger = structlog.get_logger(__name__)


@asynccontextmanager
async def create_checkpointer(db_dsn: str) -> AsyncIterator:
    """Create and initialize a Postgres checkpointer for LangGraph.

    Usage:
        async with create_checkpointer(dsn) as checkpointer:
            graph = build_report_graph(..., checkpointer=checkpointer)
    """
    from langgraph.checkpoint.postgres.aio import AsyncPostgresSaver

    async with AsyncPostgresSaver.from_conn_string(db_dsn) as checkpointer:
        await checkpointer.setup()
        logger.info("Postgres checkpointer initialized")
        yield checkpointer
