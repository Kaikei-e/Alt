"""Dependency wiring tests for Phase 1.

``get_container`` must read from ``request.app.state.container`` and
fail explicitly if the lifespan never populated it.
"""

from __future__ import annotations

from types import SimpleNamespace

import pytest
from fastapi import FastAPI


def test_get_container_requires_lifespan_initialization() -> None:
    from recap_subworker.app.deps import get_container

    app = FastAPI()
    # app.state.container is intentionally NOT set
    request = SimpleNamespace(app=app)

    with pytest.raises(RuntimeError, match="ServiceContainer"):
        get_container(request)  # type: ignore[arg-type]


def test_get_container_returns_app_state_container() -> None:
    from recap_subworker.app.container import ServiceContainer
    from recap_subworker.app.deps import get_container
    from recap_subworker.infra.config import get_settings

    app = FastAPI()
    container = ServiceContainer(get_settings())
    app.state.container = container

    request = SimpleNamespace(app=app)
    assert get_container(request) is container  # type: ignore[arg-type]
