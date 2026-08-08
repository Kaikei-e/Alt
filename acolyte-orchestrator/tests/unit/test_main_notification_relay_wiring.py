"""Composition-root wiring for the notification outbox.

The failure this guards against is the quiet one: a relay that is constructed
but never started, or a producer wired to a different notion of "enabled" than
the relay, both of which look exactly like "no notifications happened yet".
"""

from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock

import pytest

import main as main_module
from acolyte.infra.metrics import RelayMetrics


def test_producer_and_relay_share_one_enabled_decision() -> None:
    expected = None if main_module._relay_config is None else main_module._relay_config.user_id
    assert main_module._job_queue._notification_user_id == expected
    assert (main_module._relay is None) == (main_module._relay_config is None)


def test_metrics_route_is_mounted() -> None:
    app = main_module.create_app()
    assert "/metrics" in {getattr(route, "path", None) for route in app.routes}


@pytest.mark.asyncio
async def test_metrics_endpoint_renders_both_relay_gauges(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(main_module, "_relay_metrics", RelayMetrics())

    response = await main_module.metrics_endpoint(None)  # type: ignore[arg-type]

    body = bytes(response.body).decode()
    assert "notification_outbox_oldest_pending_age_seconds 0.0" in body
    assert "notification_outbox_last_tick_timestamp_seconds 0.0" in body


@pytest.mark.asyncio
async def test_metrics_endpoint_exposes_nothing_when_the_relay_is_off(monkeypatch: pytest.MonkeyPatch) -> None:
    # An absent series is an unambiguous "this relay is not running"; a zero
    # would read as a healthy relay with an empty backlog.
    monkeypatch.setattr(main_module, "_relay_metrics", None)

    response = await main_module.metrics_endpoint(None)  # type: ignore[arg-type]

    assert bytes(response.body) == b""


@pytest.mark.asyncio
async def test_lifespan_runs_the_relay_loop_and_stops_it(monkeypatch: pytest.MonkeyPatch) -> None:
    started = asyncio.Event()
    cancelled = asyncio.Event()

    class _FakeRelay:
        async def run_forever(self, interval_seconds: float) -> None:
            assert interval_seconds == 1.5
            started.set()
            try:
                await asyncio.Event().wait()
            except asyncio.CancelledError:
                cancelled.set()
                raise

    class _FakeConfig:
        interval_seconds = 1.5

    monkeypatch.setattr(main_module, "_relay", _FakeRelay())
    monkeypatch.setattr(main_module, "_relay_config", _FakeConfig())
    monkeypatch.setattr(main_module.settings, "checkpoint_enabled", False)
    monkeypatch.setattr(main_module._pool, "open", AsyncMock())
    monkeypatch.setattr(main_module._pool, "close", AsyncMock())
    monkeypatch.setattr(main_module._http_client, "aclose", AsyncMock())
    monkeypatch.setattr(main_module._job_queue, "list_running_runs", AsyncMock(return_value=[]))

    app = main_module.create_app()
    async with app.router.lifespan_context(app):
        await asyncio.wait_for(started.wait(), timeout=1.0)

    await asyncio.wait_for(cancelled.wait(), timeout=1.0)


@pytest.mark.asyncio
async def test_lifespan_starts_no_loop_when_the_relay_is_off(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(main_module, "_relay", None)
    monkeypatch.setattr(main_module, "_relay_config", None)
    monkeypatch.setattr(main_module.settings, "checkpoint_enabled", False)
    monkeypatch.setattr(main_module._pool, "open", AsyncMock())
    monkeypatch.setattr(main_module._pool, "close", AsyncMock())
    monkeypatch.setattr(main_module._http_client, "aclose", AsyncMock())
    monkeypatch.setattr(main_module._job_queue, "list_running_runs", AsyncMock(return_value=[]))

    app = main_module.create_app()
    async with app.router.lifespan_context(app):
        relay_tasks = [t for t in asyncio.all_tasks() if t.get_name() == "notification-outbox-relay"]
        assert relay_tasks == []
