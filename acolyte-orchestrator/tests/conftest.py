"""Shared test fixtures for acolyte-orchestrator."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING
from unittest.mock import MagicMock
from uuid import UUID

import pytest
from connectrpc.method import IdempotencyLevel, MethodInfo
from connectrpc.request import Headers, RequestContext
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Mount, Route
from starlette.testclient import TestClient

if TYPE_CHECKING:
    from collections.abc import Iterator

    from starlette.requests import Request

_TEST_DB_DSN = "postgresql://test:test@localhost:5439/test"
_TEST_DEV_USER_ID = "00000000-0000-0000-0000-000000000001"

# Force (not setdefault) before any Settings()/main imports during collection,
# so a CI-provided ACOLYTE_DB_DSN cannot leak into unit tests that import main.
os.environ["ACOLYTE_DB_DSN"] = _TEST_DB_DSN
os.environ["BACKEND_TOKEN_VERIFICATION"] = "disabled"
os.environ["USER_IDENTITY_DEV_USER_ID"] = _TEST_DEV_USER_ID

# Acting user for tests that need an owner UUID but do not exercise ownership
# isolation themselves; matches the dev identity the interceptor injects.
TEST_USER_ID = UUID(_TEST_DEV_USER_ID)

import acolyte.gen  # noqa: E402, F401
from acolyte.config.settings import Settings  # noqa: E402
from acolyte.gateway.memory_job_gw import MemoryJobGateway  # noqa: E402
from acolyte.gateway.memory_report_gw import MemoryReportGateway  # noqa: E402
from acolyte.gen.proto.alt.acolyte.v1.acolyte_connect import AcolyteServiceASGIApplication  # noqa: E402
from acolyte.handler.connect_service import AcolyteConnectService  # noqa: E402
from acolyte.infra.user_identity import UserIdentityInterceptor, attach_acting_user_id  # noqa: E402


def make_request_ctx(method_name: str, user_id: UUID | None = TEST_USER_ID) -> RequestContext:
    """Build a RequestContext carrying an acting user, as UserIdentityInterceptor would.

    ``user_id=None`` produces the unauthenticated context handlers must reject.
    """
    method_info = MethodInfo(
        name=method_name,
        service_name="alt.acolyte.v1.AcolyteService",
        input=MagicMock(),
        output=MagicMock(),
        idempotency_level=IdempotencyLevel.UNKNOWN,
    )
    ctx: RequestContext = RequestContext(
        method=method_info,
        http_method="POST",
        request_headers=Headers(),
    )
    if user_id is not None:
        attach_acting_user_id(ctx, user_id)
    return ctx


@pytest.fixture(autouse=True)
def _force_test_db_dsn(monkeypatch: pytest.MonkeyPatch) -> None:
    """Re-assert the test DSN and dev user for every test via monkeypatch (restored after)."""
    monkeypatch.setenv("ACOLYTE_DB_DSN", _TEST_DB_DSN)
    monkeypatch.setenv("USER_IDENTITY_DEV_USER_ID", _TEST_DEV_USER_ID)


def _create_test_app() -> Starlette:
    """Create app with in-memory stores for testing (no DB needed)."""
    settings = Settings()
    report_repo = MemoryReportGateway()
    job_queue = MemoryJobGateway()
    connect_service = AcolyteConnectService(settings, report_repo, job_queue)
    interceptor = UserIdentityInterceptor(dev_user_id=settings.resolve_dev_user_id())
    asgi_app = AcolyteServiceASGIApplication(connect_service, interceptors=[interceptor])

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
