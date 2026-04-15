"""Tests for peer-identity middleware."""

from __future__ import annotations

import os

import pytest
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.testclient import TestClient

from tag_generator.infra.peer_identity import (
    PEER_IDENTITY_HEADER,
    PeerIdentityMiddleware,
    allowed_peers_from_env,
)


def _echo_peer(request):
    return JSONResponse({"peer": getattr(request.state, "peer_identity", None)})


def _build_app(*, allowed=None, strict=False, verify_client="on"):
    os.environ["PEER_IDENTITY_TRUSTED"] = verify_client
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware, allowed=allowed, strict=strict)
    return app


def test_header_propagated_when_mtls_on():
    app = _build_app(verify_client="on")
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": "alt-backend"}


def test_header_stripped_when_mtls_off():
    app = _build_app(verify_client="off")
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "root"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_strict_rejects_missing_peer():
    app = _build_app(strict=True)
    with TestClient(app) as client:
        resp = client.get("/echo")
        assert resp.status_code == 401


def test_strict_rejects_disallowed_peer():
    app = _build_app(allowed=["recap-worker"], strict=True)
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "evil-svc"})
        assert resp.status_code == 403


def test_strict_accepts_allowlisted_peer():
    app = _build_app(allowed=["recap-worker", "mq-hub"], strict=True)
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "mq-hub"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": "mq-hub"}


def test_allowed_peers_from_env(monkeypatch):
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", " recap-worker , mq-hub , alt-backend")
    assert allowed_peers_from_env() == ["recap-worker", "mq-hub", "alt-backend"]


@pytest.fixture(autouse=True)
def _reset_env():
    yield
    os.environ.pop("PEER_IDENTITY_TRUSTED", None)
