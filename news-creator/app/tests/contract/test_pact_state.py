"""Unit tests for the provider-state registry helper."""

from __future__ import annotations

import logging
from typing import Any

import pytest

from ._pact_state import StateRegistry, dispatch


@pytest.mark.asyncio
async def test_dispatch_invokes_registered_action():
    seen: list[dict[str, Any]] = []

    async def _action(params: dict[str, Any]) -> None:
        seen.append(params)

    registry: StateRegistry = {"a known state": _action}
    await dispatch(
        registry, {"state": "a known state", "params": {"k": 1}, "action": "setup"}
    )
    assert seen == [{"k": 1}]


@pytest.mark.asyncio
async def test_dispatch_warns_on_unknown_state(caplog: pytest.LogCaptureFixture):
    registry: StateRegistry = {}
    with caplog.at_level(logging.WARNING):
        await dispatch(registry, {"state": "never registered", "action": "setup"})
    assert any("Unknown provider state" in rec.message for rec in caplog.records)


@pytest.mark.asyncio
async def test_dispatch_skips_teardown():
    called = False

    async def _action(_params: dict[str, Any]) -> None:
        nonlocal called
        called = True

    registry: StateRegistry = {"x": _action}
    await dispatch(registry, {"state": "x", "action": "teardown"})
    assert not called
