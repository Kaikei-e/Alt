"""Unit tests for checkpoint factory — verifies pool lifecycle, setup(), and yield contract."""

from __future__ import annotations

import inspect
from contextlib import asynccontextmanager
from importlib.metadata import version
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from psycopg.rows import dict_row
from psycopg_pool import AsyncConnectionPool

from acolyte.gateway.checkpoint_factory import create_checkpointer

if TYPE_CHECKING:
    from collections.abc import AsyncIterator


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


@asynccontextmanager
async def _patched_factory_deps() -> AsyncIterator[tuple[MagicMock, AsyncMock, MagicMock, AsyncMock]]:
    """Patch AsyncConnectionPool and AsyncPostgresSaver on checkpoint_factory."""
    mock_pool = AsyncMock()
    mock_pool.open = AsyncMock()
    mock_pool.close = AsyncMock()
    mock_pool.__aenter__ = AsyncMock(return_value=mock_pool)
    mock_pool.__aexit__ = AsyncMock(return_value=False)

    mock_saver = AsyncMock()
    mock_saver.setup = AsyncMock()

    # Allow legacy from_conn_string path to execute without TypeError during RED
    # so assertions fail cleanly on the new pool contract.
    legacy_saver = AsyncMock()
    legacy_saver.setup = AsyncMock()
    legacy_ctx = AsyncMock()
    legacy_ctx.__aenter__ = AsyncMock(return_value=legacy_saver)
    legacy_ctx.__aexit__ = AsyncMock(return_value=False)

    with (
        patch("acolyte.gateway.checkpoint_factory.AsyncConnectionPool", create=True) as mock_pool_cls,
        patch("acolyte.gateway.checkpoint_factory.AsyncPostgresSaver") as mock_saver_cls,
    ):
        mock_pool_cls.check_connection = AsyncConnectionPool.check_connection
        mock_pool_cls.return_value = mock_pool
        mock_saver_cls.return_value = mock_saver
        mock_saver_cls.from_conn_string.return_value = legacy_ctx
        yield mock_pool_cls, mock_pool, mock_saver_cls, mock_saver


@pytest.mark.asyncio
async def test_create_checkpointer_calls_setup() -> None:
    """Factory must await setup() on the pool-backed AsyncPostgresSaver."""
    async with _patched_factory_deps() as (_mock_pool_cls, _mock_pool, _mock_saver_cls, mock_saver):
        async with create_checkpointer("postgresql://test") as _saver:
            pass

        mock_saver.setup.assert_awaited_once()


@pytest.mark.asyncio
async def test_create_checkpointer_yields_saver() -> None:
    """Factory must yield the AsyncPostgresSaver instance constructed with the pool."""
    async with (
        _patched_factory_deps() as (_mock_pool_cls, _mock_pool, _mock_saver_cls, mock_saver),
        create_checkpointer("postgresql://test") as saver,
    ):
        assert saver is mock_saver


@pytest.mark.asyncio
async def test_create_checkpointer_configures_pool_kwargs_and_check() -> None:
    """Pool must be constructed with autocommit, prepare_threshold=0, dict_row, and check_connection."""
    async with _patched_factory_deps() as (mock_pool_cls, _mock_pool, _mock_saver_cls, _mock_saver):
        async with create_checkpointer("postgresql://test") as _saver:
            pass

        mock_pool_cls.assert_called_once()
        args, call_kwargs = mock_pool_cls.call_args
        dsn_arg = args[0] if args else call_kwargs.get("conninfo")
        assert dsn_arg == "postgresql://test"
        assert call_kwargs.get("kwargs") == {
            "autocommit": True,
            "prepare_threshold": 0,
            "row_factory": dict_row,
        }
        assert call_kwargs.get("check") is AsyncConnectionPool.check_connection


@pytest.mark.asyncio
async def test_create_checkpointer_passes_pool_to_saver() -> None:
    """AsyncPostgresSaver must receive the pool instance directly via conn=pool."""
    async with _patched_factory_deps() as (_mock_pool_cls, mock_pool, mock_saver_cls, _mock_saver):
        async with create_checkpointer("postgresql://test") as _saver:
            pass

        mock_saver_cls.assert_called_once_with(conn=mock_pool)
        mock_saver_cls.from_conn_string.assert_not_called()


@pytest.mark.asyncio
async def test_create_checkpointer_opens_pool_with_wait() -> None:
    """Pool must be opened with wait=True so an unreachable DB fails at startup."""
    async with _patched_factory_deps() as (mock_pool_cls, mock_pool, _mock_saver_cls, _mock_saver):
        async with create_checkpointer("postgresql://test") as _saver:
            mock_pool.open.assert_awaited_once_with(wait=True)

        assert mock_pool_cls.call_args.kwargs.get("open") is False


@pytest.mark.asyncio
async def test_create_checkpointer_closes_pool_on_exit() -> None:
    """Pool must remain open inside the context and be closed when the context exits."""
    async with _patched_factory_deps() as (_mock_pool_cls, mock_pool, _mock_saver_cls, _mock_saver):
        async with create_checkpointer("postgresql://test") as _saver:
            mock_pool.close.assert_not_awaited()

        mock_pool.close.assert_awaited_once()
