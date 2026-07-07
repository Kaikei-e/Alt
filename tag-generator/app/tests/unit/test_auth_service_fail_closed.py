"""Tests that authentication fails closed when alt_auth.client is unavailable.

.claude/rules/di-wiring.md (Python section): an ImportError/ModuleNotFoundError
fallback that silently swaps in a no-op / anonymous implementation is forbidden
for authentication -- a missing alt_auth.client must fail closed, never inject
a fabricated anonymous UserContext (CLAUDE.md rule 8).
"""

import asyncio

import pytest
from fastapi import HTTPException
from fastapi.testclient import TestClient

import auth_service


@pytest.fixture()
def client():
    return TestClient(auth_service.app)


class TestRequireAuthFallbackFailsClosed:
    """Direct unit test of the fallback `require_auth` decorator."""

    def test_async_handler_raises_instead_of_running(self):
        handler_called = False

        @auth_service.require_auth(None)
        async def _protected(user_context=None):
            nonlocal handler_called
            handler_called = True
            return user_context

        with pytest.raises(HTTPException) as exc_info:
            asyncio.run(_protected())

        assert exc_info.value.status_code == 503
        assert not handler_called

    def test_sync_handler_raises_instead_of_running(self):
        handler_called = False

        @auth_service.require_auth(None)
        def _protected(user_context=None):
            nonlocal handler_called
            handler_called = True
            return user_context

        with pytest.raises(HTTPException) as exc_info:
            _protected()

        assert exc_info.value.status_code == 503
        assert not handler_called


class TestProtectedEndpointsFailClosed:
    """Endpoint-level regression: no anonymous/public fallback identity.

    `require_auth`'s wrapper is exposed via `functools.wraps`, so FastAPI's
    dependant-building follows `__wrapped__` and binds parameters against the
    *undecorated* handler signature `(request: TagGenerationRequest,
    user_context: UserContext)`. With two body-shaped parameters, FastAPI
    embeds each under its own key, so a request body must nest
    `{"request": {...}, "user_context": {...}}` to reach the decorator at
    all (a pre-existing request-shape quirk, unrelated to this fix, and out
    of scope here). We use that reachable shape so these tests actually
    exercise the fail-closed decorator rather than short-circuiting on
    FastAPI's own 422 body validation.
    """

    def test_generate_tags_endpoint_fails_closed(self, client):
        resp = client.post(
            "/api/v1/generate-tags",
            json={
                "request": {"article_id": "a-1", "title": "t", "content": "c"},
                "user_context": {},
            },
        )

        assert resp.status_code == 503
        # Must never have reached business logic that returns a fabricated identity.
        assert "user_id" not in resp.json()

    def test_generate_tags_endpoint_rejects_even_a_caller_supplied_identity(self, client):
        """Before this fix, `kwargs.setdefault("user_context", ...)` never
        fired here because FastAPI had already bound `user_context` from the
        request body -- so a caller could hand-craft any UserContext
        (including someone else's tenant_id) and have it silently trusted.
        The fail-closed decorator now rejects the request before the handler
        ever sees the caller-supplied identity."""
        resp = client.post(
            "/api/v1/generate-tags",
            json={
                "request": {"article_id": "a-1", "title": "t", "content": "c"},
                "user_context": {"user_id": "spoofed-admin", "tenant_id": "victim-tenant"},
            },
        )

        assert resp.status_code == 503
        assert "spoofed-admin" not in resp.text

    def test_user_preferences_endpoint_fails_closed(self, client):
        resp = client.request("GET", "/api/v1/user-preferences", json={"user_context": {}})

        assert resp.status_code == 503
        assert "user_id" not in resp.json()
