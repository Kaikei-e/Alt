"""Tests for peer-identity middleware."""

from __future__ import annotations

import os
from collections.abc import Generator
from typing import TYPE_CHECKING

import pytest
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.testclient import TestClient

from acolyte.infra.peer_identity import (
    PEER_IDENTITY_HEADER,
    PeerIdentityMiddleware,
    allowed_peers_from_env,
)

if TYPE_CHECKING:
    from starlette.requests import Request


def _echo_peer(request: Request) -> JSONResponse:
    return JSONResponse({"peer": getattr(request.state, "peer_identity", None)})


def _build_app(*, allowed: list[str] | None = None, strict: bool = False, verify_client: str = "on") -> Starlette:
    os.environ["PEER_IDENTITY_TRUSTED"] = verify_client
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware, allowed=allowed, strict=strict)
    return app


def test_header_propagated_when_mtls_on() -> None:
    app = _build_app(verify_client="on")
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": "alt-backend"}


def test_header_stripped_when_mtls_off() -> None:
    # VERIFY_CLIENT=off means the sidecar is NOT authenticating peers,
    # so any X-Alt-Peer-Identity on the wire is attacker-controlled and
    # must be dropped — never propagated into request.state.
    app = _build_app(verify_client="off")
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "root"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_strict_rejects_missing_peer() -> None:
    app = _build_app(strict=True)
    with TestClient(app) as client:
        resp = client.get("/echo")
        assert resp.status_code == 401


def test_strict_rejects_disallowed_peer() -> None:
    app = _build_app(allowed=["alt-backend"], strict=True)
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "evil-svc"})
        assert resp.status_code == 403


def test_strict_accepts_allowlisted_peer() -> None:
    app = _build_app(allowed=["alt-backend", "bff"], strict=True)
    with TestClient(app) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "bff"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": "bff"}


def test_allowed_peers_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", " alt-backend , bff ,  , rag-orchestrator")
    assert allowed_peers_from_env() == ["alt-backend", "bff", "rag-orchestrator"]


def test_allowed_peers_from_env_empty(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("MTLS_ALLOWED_PEERS", raising=False)
    assert allowed_peers_from_env() == []


@pytest.fixture(autouse=True)
def _reset_env() -> Generator[None]:
    # Prevent cross-test leak of PEER_IDENTITY_TRUSTED.
    yield
    os.environ.pop("PEER_IDENTITY_TRUSTED", None)
