"""Lifespan-based DI tests for Phase 1.

Verifies that ``create_app`` wires ``ServiceContainer`` into ``app.state``
via an ``@asynccontextmanager`` lifespan, and that shutdown invokes
``aclose`` on the owned database resources.
"""

from __future__ import annotations

from unittest.mock import patch

import pytest
from fastapi.testclient import TestClient


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

    async def _fake_shutdown(self) -> None:  # noqa: ANN001
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
