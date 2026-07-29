"""Tests for peer-identity middleware (recap-evaluator)."""

from __future__ import annotations

import pytest
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.testclient import TestClient

from recap_evaluator.infra.peer_identity import (
    PEER_IDENTITY_HEADER,
    PeerIdentityMiddleware,
    allowed_peers_from_env,
)

# recap-evaluator has no pki-agent sidecar in compose/pki.yaml, so nothing
# terminates mTLS in front of it today. The loopback case is kept anyway: it
# is the transport that would carry a verified CN once one is added.
SIDECAR = ("127.0.0.1", 44444)
DIRECT = ("172.18.0.9", 44444)


def _echo_peer(request):
    return JSONResponse({"peer": getattr(request.state, "peer_identity", None)})


def _build_app(
    monkeypatch: pytest.MonkeyPatch,
    *,
    allowed: list[str] | None = None,
    strict: bool = False,
    verify_client: str = "on",
) -> Starlette:
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", verify_client)
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware, allowed=allowed, strict=strict)
    return app


def test_header_propagated_when_mtls_on(monkeypatch: pytest.MonkeyPatch) -> None:
    app = _build_app(monkeypatch, verify_client="on")
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": "alt-backend"}


def test_header_stripped_when_mtls_off(monkeypatch: pytest.MonkeyPatch) -> None:
    # VERIFY_CLIENT=off means the sidecar is NOT authenticating peers, so any
    # X-Alt-Peer-Identity on the wire is attacker-controlled.
    app = _build_app(monkeypatch, verify_client="off")
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "root"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_header_stripped_when_trust_unset(monkeypatch: pytest.MonkeyPatch) -> None:
    # Rule 9: "the sidecar verifies client certs" is a claim an operator makes
    # explicitly. Inferring it from an unset variable is how "nobody wired
    # this" became indistinguishable from "deliberately open".
    monkeypatch.delenv("PEER_IDENTITY_TRUSTED", raising=False)
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_header_stripped_when_not_from_sidecar(monkeypatch: pytest.MonkeyPatch) -> None:
    # The published plaintext port bypasses the sidecar entirely, so a header
    # arriving on it was written by the caller. Trusting it launders the
    # attribution of every audit line the request produces.
    app = _build_app(monkeypatch, verify_client="on")
    with TestClient(app, client=DIRECT) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_strict_rejects_peer_from_non_sidecar_transport(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # The dependency the review names: once strict flips to True this is the
    # difference between an authentication bypass and a 401.
    app = _build_app(monkeypatch, allowed=["alt-backend"], strict=True)
    with TestClient(app, client=DIRECT) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 401


def test_allowed_peers_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", " alt-backend , recap-worker ,  ")
    assert allowed_peers_from_env() == ["alt-backend", "recap-worker"]
