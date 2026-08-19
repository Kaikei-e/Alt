"""Unit tests for checkpoint factory — verifies setup() and yield contract."""

from __future__ import annotations

import inspect
from importlib.metadata import version
from unittest.mock import AsyncMock, patch

import pytest

from acolyte.gateway.checkpoint_factory import create_checkpointer


def test_checkpoint_postgres_is_patched_for_ghsa_47pj() -> None:
    """GHSA-47pj-3jcm-6whg / PYSEC-2026-3635: namespace prefix must not cross segments.

    Acolyte wires AsyncPostgresSaver, not PostgresStore search/list. Still require
    the patched 3.1.1 line so a later store adoption cannot revive the leak.
    """
    parts = tuple(int(p) for p in version("langgraph-checkpoint-postgres").split(".")[:3])
    assert parts >= (3, 1, 1)
    src = inspect.getsource(create_checkpointer)
    assert "PostgresStore" not in src
    assert "AsyncPostgresSaver" in src


@pytest.mark.asyncio
async def test_create_checkpointer_calls_setup() -> None:
    """Factory must call setup() on the saver to create checkpoint tables."""
    mock_saver = AsyncMock()
    mock_saver.setup = AsyncMock()

    mock_ctx = AsyncMock()
    mock_ctx.__aenter__ = AsyncMock(return_value=mock_saver)
    mock_ctx.__aexit__ = AsyncMock(return_value=False)

    with patch("acolyte.gateway.checkpoint_factory.AsyncPostgresSaver") as mock_cls:
        mock_cls.from_conn_string.return_value = mock_ctx

        async with create_checkpointer("postgresql://test") as _saver:
            pass

        mock_saver.setup.assert_awaited_once()


@pytest.mark.asyncio
async def test_create_checkpointer_yields_saver() -> None:
    """Factory must yield the saver instance inside the context manager."""
    mock_saver = AsyncMock()
    mock_saver.setup = AsyncMock()

    mock_ctx = AsyncMock()
    mock_ctx.__aenter__ = AsyncMock(return_value=mock_saver)
    mock_ctx.__aexit__ = AsyncMock(return_value=False)

    with patch("acolyte.gateway.checkpoint_factory.AsyncPostgresSaver") as mock_cls:
        mock_cls.from_conn_string.return_value = mock_ctx

        async with create_checkpointer("postgresql://test") as saver:
            assert saver is mock_saver
