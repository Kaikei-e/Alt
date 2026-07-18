"""Shared test fixtures for acolyte-orchestrator."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

import pytest
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Mount, Route
from starlette.testclient import TestClient

if TYPE_CHECKING:
    from collections.abc import Iterator

    from starlette.requests import Request

_TEST_DB_DSN = "postgresql://test:test@localhost:5439/test"

# Force (not setdefault) before any Settings()/main imports during collection,
# so a CI-provided ACOLYTE_DB_DSN cannot leak into unit tests that import main.
os.environ["ACOLYTE_DB_DSN"] = _TEST_DB_DSN

import acolyte.gen  # noqa: E402, F401
from acolyte.config.settings import Settings  # noqa: E402
from acolyte.gateway.memory_job_gw import MemoryJobGateway  # noqa: E402
from acolyte.gateway.memory_report_gw import MemoryReportGateway  # noqa: E402
from acolyte.gen.proto.alt.acolyte.v1.acolyte_connect import AcolyteServiceASGIApplication  # noqa: E402
from acolyte.handler.connect_service import AcolyteConnectService  # noqa: E402


@pytest.fixture(autouse=True)
def _force_test_db_dsn(monkeypatch: pytest.MonkeyPatch) -> None:
    """Re-assert the test DSN for every test via monkeypatch (restored after)."""
    monkeypatch.setenv("ACOLYTE_DB_DSN", _TEST_DB_DSN)


def _create_test_app() -> Starlette:
    """Create app with in-memory stores for testing (no DB needed)."""
    settings = Settings()
    report_repo = MemoryReportGateway()
    job_queue = MemoryJobGateway()
    connect_service = AcolyteConnectService(settings, report_repo, job_queue)
    asgi_app = AcolyteServiceASGIApplication(connect_service)

    async def health_endpoint(request: Request) -> JSONResponse:
        return JSONResponse({"status": "ok", "service": "acolyte-orchestrator"})

    return Starlette(
        routes=[
            Route("/health", health_endpoint),
            Mount(asgi_app.path, app=asgi_app),
        ],
    )


@pytest.fixture
def client() -> Iterator[TestClient]:
    """Create a test client with in-memory stores."""
    app = _create_test_app()
    with TestClient(app) as test_client:
        yield test_client
