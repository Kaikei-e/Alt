"""Unit tests for checkpoint-aware application startup wiring."""

from __future__ import annotations

from contextlib import asynccontextmanager
from unittest.mock import AsyncMock

import pytest

import main as main_module


@pytest.mark.asyncio
async def test_create_app_compiles_graph_with_checkpointer_on_startup(monkeypatch) -> None:
    """When checkpointing is enabled, startup should compile the graph with a checkpointer."""
    sentinel_checkpointer = object()
    sentinel_graph = object()
    compile_calls: list[object | None] = []

    def fake_compile_graph(*, checkpointer: object | None = None) -> object:
        compile_calls.append(checkpointer)
        return sentinel_graph

    @asynccontextmanager
    async def fake_create_checkpointer(db_dsn: str):
        assert db_dsn == main_module._dsn
        yield sentinel_checkpointer

    monkeypatch.setattr(main_module.settings, "checkpoint_enabled", True)
    monkeypatch.setattr(main_module, "_compile_graph", fake_compile_graph)
    monkeypatch.setattr(main_module, "create_checkpointer", fake_create_checkpointer)
    monkeypatch.setattr(main_module._pool, "open", AsyncMock())
    monkeypatch.setattr(main_module._pool, "close", AsyncMock())
    monkeypatch.setattr(main_module._http_client, "aclose", AsyncMock())

    app = main_module.create_app()

    async with app.router.lifespan_context(app):
        assert app.state.connect_service._graph is sentinel_graph

    assert compile_calls == [sentinel_checkpointer]
    main_module._pool.open.assert_awaited_once()
    main_module._pool.close.assert_awaited_once()
    main_module._http_client.aclose.assert_awaited_once()
