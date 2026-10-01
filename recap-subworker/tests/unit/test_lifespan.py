"""Lifespan-based DI tests for Phase 1.

Verifies that ``create_app`` wires ``ServiceContainer`` into ``app.state``
via an ``@asynccontextmanager`` lifespan, and that shutdown invokes
``aclose`` on the owned database resources.
"""

from __future__ import annotations

from unittest.mock import patch

import pytest
from fastapi.testclient import TestClient

pytestmark = pytest.mark.usefixtures("stub_startup_sweep")


@pytest.fixture
def app_factory():
    from recap_subworker.app.main import create_app

    return create_app


def test_lifespan_binds_container_to_app_state(app_factory) -> None:
    app = app_factory()
    with TestClient(app):
        container = getattr(app.state, "container", None)
        assert container is not None, "app.state.container must be set by lifespan"
        # Settings exposed on the container
        assert container.settings is not None


def test_lifespan_shutdown_disposes_database_engine(app_factory) -> None:
    """ServiceContainer.shutdown() must be invoked on lifespan shutdown."""
    from recap_subworker.app.container import ServiceContainer

    calls: list[str] = []

    async def _fake_shutdown(self) -> None:
        calls.append("shutdown")

    with patch.object(ServiceContainer, "shutdown", _fake_shutdown):
        app = app_factory()
        with TestClient(app):
            pass  # enter + exit lifespan
        assert calls == ["shutdown"], "ServiceContainer.shutdown must run on lifespan exit"


def test_separate_apps_have_independent_containers(app_factory) -> None:
    """Two TestClients backed by separate ``create_app()`` instances must
    not share the same ServiceContainer (no module-level leakage).
    """
    app_a = app_factory()
    app_b = app_factory()

    with TestClient(app_a), TestClient(app_b):
        container_a = app_a.state.container
        container_b = app_b.state.container
        assert container_a is not container_b


def test_lifespan_binds_deep_health_runner(app_factory) -> None:
    app = app_factory()
    with TestClient(app):
        runner = getattr(app.state, "deep_health_runner", None)
        assert runner is not None, "app.state.deep_health_runner must be set by lifespan"


def test_lifespan_binds_admin_auth_to_app_state(app_factory) -> None:
    app = app_factory()
    with TestClient(app):
        admin_auth = getattr(app.state, "admin_auth", None)
        assert admin_auth is not None, "app.state.admin_auth must be set by lifespan"


def test_lifespan_fails_fast_when_admin_auth_enabled_without_token(
    app_factory, monkeypatch
) -> None:
    """CLAUDE.md rule 9: a forgotten ADMIN_TOKEN_FILE must abort startup,
    never silently serve /admin/* and /v1/runs unauthenticated."""
    monkeypatch.delenv("ADMIN_AUTH", raising=False)
    monkeypatch.delenv("ADMIN_TOKEN_FILE", raising=False)
    app = app_factory()
    with pytest.raises(RuntimeError, match="ADMIN_TOKEN_FILE"), TestClient(app):
        pass


def test_separate_apps_have_independent_deep_health_runners(app_factory) -> None:
    app_a = app_factory()
    app_b = app_factory()
    with TestClient(app_a), TestClient(app_b):
        assert app_a.state.deep_health_runner is not app_b.state.deep_health_runner


def test_lifespan_invokes_orphan_sweep_on_startup(app_factory) -> None:
    """Lifespan must sweep orphaned runs on startup before serving requests."""
    from unittest.mock import AsyncMock, patch

    mock_sweep = AsyncMock(return_value=0)
    with patch("recap_subworker.app.main.sweep_orphaned_runs", mock_sweep):
        app = app_factory()
        with TestClient(app):
            pass
        mock_sweep.assert_awaited_once()


def test_lifespan_raises_when_sweep_fails(app_factory) -> None:
    """A sweep failure at startup must stay loud: propagate out of lifespan."""
    from unittest.mock import AsyncMock, patch

    mock_sweep = AsyncMock(side_effect=RuntimeError("db connection failed"))
    with patch("recap_subworker.app.main.sweep_orphaned_runs", mock_sweep):
        app = app_factory()
        with pytest.raises(RuntimeError, match="db connection failed"):
            with TestClient(app):
                pass


def test_lifespan_watchdog_stop_failure_does_not_skip_container_shutdown(app_factory) -> None:
    """stop() must not let a task exception skip container.shutdown()."""
    from unittest.mock import AsyncMock, patch
    from recap_subworker.app.container import ServiceContainer
    from recap_subworker.infra.loop_watchdog import LoopWatchdog

    shutdown_called = False

    async def _fake_shutdown(self) -> None:
        nonlocal shutdown_called
        shutdown_called = True

    async def _failing_stop(self) -> None:
        raise RuntimeError("watchdog task failed")

    with (
        patch.object(LoopWatchdog, "start", lambda self: None),
        patch.object(LoopWatchdog, "stop", _failing_stop),
        patch.object(ServiceContainer, "shutdown", _fake_shutdown),
        patch("recap_subworker.app.main.sweep_orphaned_runs", AsyncMock(return_value=0)),
    ):
        app = app_factory()
        with pytest.raises(RuntimeError, match="watchdog task failed"):
            with TestClient(app):
                pass
        assert shutdown_called, "container.shutdown() must run even if watchdog.stop() raises"


def test_lifespan_starts_and_stops_loop_watchdog(app_factory) -> None:
    """Lifespan must start the loop watchdog on boot and stop it on shutdown."""
    from recap_subworker.infra.loop_watchdog import LoopWatchdog

    events: list[str] = []

    def _fake_start(self) -> None:
        events.append("start")

    async def _fake_stop(self) -> None:
        events.append("stop")

    with (
        patch.object(LoopWatchdog, "start", _fake_start),
        patch.object(LoopWatchdog, "stop", _fake_stop),
    ):
        app = app_factory()
        with TestClient(app):
            assert "start" in events, "LoopWatchdog.start must run on lifespan startup"
        assert "stop" in events, "LoopWatchdog.stop must run on lifespan shutdown"


def test_lifespan_does_not_log_watchdog_enabled_if_first_arm_fails(app_factory) -> None:
    """If first arm raises, LoopWatchdog.start() raises loudly and loop_watchdog_enabled is not logged."""
    from structlog.testing import capture_logs
    from recap_subworker.infra.loop_watchdog import LoopWatchdog

    def _failing_start(self) -> None:
        raise RuntimeError("arm failure")

    with capture_logs() as cap_logs:
        with patch.object(LoopWatchdog, "start", _failing_start):
            app = app_factory()
            with pytest.raises(RuntimeError, match="arm failure"):
                with TestClient(app):
                    pass
        events = [log.get("event") for log in cap_logs]
        assert "loop_watchdog_enabled" not in events


def test_lifespan_sweep_runs_before_container_bound_to_app_state(app_factory) -> None:
    """The startup sweep must run before app.state.container is assigned."""
    container_visible_during_sweep: bool | None = None

    async def _fake_sweep(session_factory) -> int:
        nonlocal container_visible_during_sweep
        container_visible_during_sweep = getattr(app.state, "container", None) is not None
        return 0

    app = app_factory()
    with patch("recap_subworker.app.main.sweep_orphaned_runs", _fake_sweep):
        with TestClient(app):
            pass

    assert container_visible_during_sweep is False, (
        "app.state.container must not be set while sweep_orphaned_runs is running"
    )
