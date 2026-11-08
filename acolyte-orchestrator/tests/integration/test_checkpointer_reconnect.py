"""Integration test: checkpointer survives backend termination via pooled reconnect."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING
from uuid import uuid4

import psycopg
import pytest
from langgraph.checkpoint.base import empty_checkpoint

from acolyte.gateway.checkpoint_factory import create_checkpointer

if TYPE_CHECKING:
    from langchain_core.runnables import RunnableConfig

_DSN = os.getenv("ACOLYTE_TEST_DB_DSN", "")

pytestmark = pytest.mark.skipif(
    not _DSN,
    reason="ACOLYTE_TEST_DB_DSN not set (needs a disposable Postgres instance)",
)


@pytest.mark.asyncio
async def test_checkpointer_reconnects_after_backend_terminated() -> None:
    """After pg_terminate_backend kills the pooled connection, subsequent reads succeed."""
    thread_id = f"acolyte-reconnect:{uuid4()}"
    write_config: RunnableConfig = {"configurable": {"thread_id": thread_id, "checkpoint_ns": ""}}
    read_config: RunnableConfig = {"configurable": {"thread_id": thread_id}}

    checkpoint = empty_checkpoint()
    checkpoint["channel_values"] = {"step": "before-terminate"}

    async with create_checkpointer(_DSN) as saver:
        await saver.aput(write_config, checkpoint, {"source": "input", "step": 0}, {})

        initial = await saver.aget(read_config)
        assert initial is not None
        assert initial["id"] == checkpoint["id"]
        assert initial["channel_values"] == {"step": "before-terminate"}

        async with await psycopg.AsyncConnection.connect(_DSN, autocommit=True) as admin_conn:
            await admin_conn.execute(
                "SELECT pg_terminate_backend(pid) "
                "FROM pg_stat_activity "
                "WHERE datname = current_database() AND pid <> pg_backend_pid()"
            )

        recovered = await saver.aget(read_config)
        assert recovered is not None
        assert recovered["id"] == checkpoint["id"]
        assert recovered["channel_values"] == {"step": "before-terminate"}
