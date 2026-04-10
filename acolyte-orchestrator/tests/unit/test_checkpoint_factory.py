"""Unit tests for checkpoint factory — verifies setup() and yield contract."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest

from acolyte.gateway.checkpoint_factory import create_checkpointer


@pytest.mark.asyncio
async def test_create_checkpointer_calls_setup() -> None:
    """Factory must call setup() on the saver to create checkpoint tables."""
    mock_saver = AsyncMock()
    mock_saver.setup = AsyncMock()

    mock_ctx = AsyncMock()
    mock_ctx.__aenter__ = AsyncMock(return_value=mock_saver)
    mock_ctx.__aexit__ = AsyncMock(return_value=False)

    with patch("langgraph.checkpoint.postgres.aio.AsyncPostgresSaver") as mock_cls:
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

    with patch("langgraph.checkpoint.postgres.aio.AsyncPostgresSaver") as mock_cls:
        mock_cls.from_conn_string.return_value = mock_ctx

        async with create_checkpointer("postgresql://test") as saver:
            assert saver is mock_saver
