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


def test_producer_and_relay_share_one_enabled_decision() -> None:
    expected = None if main_module._relay_config is None else main_module._relay_config.user_id
    assert main_module._job_queue._notification_user_id == expected
    assert (main_module._relay is None) == (main_module._relay_config is None)


def test_no_metrics_route_is_mounted() -> None:
    """The API listener must not gain a metrics surface.

    Its access control is "who can open a socket", so a route added here is a
    new unauthenticated surface — and run counts and model names are exactly
    what a scraped /metrics would publish. This service is observed through the
    rask log pipeline instead (OTEL_EXPORTER_OTLP_ENDPOINT in
    compose/acolyte.yaml).

    e2e/playwright/acolyte-orchestrator/tests/topology.spec.ts asserts the same
    thing against a running container. This is the cheap copy that fails in a
    unit run rather than after a stack boots.
    """
    app = main_module.create_app()

    assert "/metrics" not in {getattr(route, "path", None) for route in app.routes}


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


@pytest.mark.asyncio
async def test_lifespan_starts_enrollment_before_inbound_tls(monkeypatch: pytest.MonkeyPatch) -> None:
    order: list[str] = []

    class _FakePki:
        async def aclose(self) -> None:
            order.append("pki_stop")

    async def _fake_enroll(service_name: str) -> _FakePki:
        assert service_name == "acolyte-orchestrator"
        order.append("pki_start")
        return _FakePki()

    class _FakeRuntime:
        async def aclose(self) -> None:
            order.append("tls_stop")

    async def _fake_tls(_app: object, _bind: object) -> _FakeRuntime:
        order.append("tls_start")
        return _FakeRuntime()

    monkeypatch.setattr(main_module, "start_enrollment", _fake_enroll)
    monkeypatch.setattr(main_module, "start_inbound_tls_listener", _fake_tls)
    monkeypatch.setattr(main_module.settings, "checkpoint_enabled", False)
    monkeypatch.setattr(main_module._pool, "open", AsyncMock())
    monkeypatch.setattr(main_module._pool, "close", AsyncMock())
    monkeypatch.setattr(main_module._http_client, "aclose", AsyncMock())
    monkeypatch.setattr(main_module._job_queue, "list_running_runs", AsyncMock(return_value=[]))
    monkeypatch.setattr(main_module, "_relay", None)
    monkeypatch.setattr(main_module, "_relay_config", None)

    app = main_module.create_app()
    async with app.router.lifespan_context(app):
        assert order == ["pki_start", "tls_start"]

    assert order == ["pki_start", "tls_start", "tls_stop", "pki_stop"]


@pytest.mark.asyncio
async def test_lifespan_wires_inbound_tls_listener(monkeypatch: pytest.MonkeyPatch) -> None:
    started = asyncio.Event()
    closed = asyncio.Event()

    class _FakeRuntime:
        async def aclose(self) -> None:
            closed.set()

    async def _fake_start(app: object, bind: object) -> _FakeRuntime:
        started.set()
        return _FakeRuntime()

    async def _fake_enroll(_service_name: str) -> None:
        return None

    monkeypatch.setattr(main_module, "start_enrollment", _fake_enroll)
    monkeypatch.setattr(main_module, "start_inbound_tls_listener", _fake_start)
    monkeypatch.setattr(main_module.settings, "checkpoint_enabled", False)
    monkeypatch.setattr(main_module._pool, "open", AsyncMock())
    monkeypatch.setattr(main_module._pool, "close", AsyncMock())
    monkeypatch.setattr(main_module._http_client, "aclose", AsyncMock())
    monkeypatch.setattr(main_module._job_queue, "list_running_runs", AsyncMock(return_value=[]))
    monkeypatch.setattr(main_module, "_relay", None)
    monkeypatch.setattr(main_module, "_relay_config", None)

    app = main_module.create_app()
    async with app.router.lifespan_context(app):
        await asyncio.wait_for(started.wait(), timeout=1.0)

    await asyncio.wait_for(closed.wait(), timeout=1.0)
