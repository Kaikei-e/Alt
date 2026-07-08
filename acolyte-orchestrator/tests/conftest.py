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
    from starlette.requests import Request

# Force test mode — prevent real DB connection
os.environ.setdefault("ACOLYTE_DB_DSN", "postgresql://test:test@localhost:5439/test")

import acolyte.gen  # noqa: F401, I001

from acolyte.config.settings import Settings
from acolyte.gateway.memory_job_gw import MemoryJobGateway
from acolyte.gateway.memory_report_gw import MemoryReportGateway
from acolyte.gen.proto.alt.acolyte.v1.acolyte_connect import AcolyteServiceASGIApplication
from acolyte.handler.connect_service import AcolyteConnectService


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
def client() -> TestClient:
    """Create a test client with in-memory stores."""
    app = _create_test_app()
    return TestClient(app)
