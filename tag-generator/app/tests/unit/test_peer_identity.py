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

# pki-agent shares this container's network namespace and proxies to
# 127.0.0.1:9400, so a loopback transport peer is the sidecar. Anything else
# reached the published plaintext port directly.
SIDECAR = ("127.0.0.1", 44444)
DIRECT = ("172.18.0.9", 44444)


def _echo_peer(request):
    return JSONResponse({"peer": getattr(request.state, "peer_identity", None)})


def _build_app(*, allowed=None, strict=False, verify_client="on"):
    os.environ["PEER_IDENTITY_TRUSTED"] = verify_client
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware, allowed=allowed, strict=strict)
    return app


def test_header_propagated_when_mtls_on():
    app = _build_app(verify_client="on")
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": "alt-backend"}


def test_header_stripped_when_mtls_off():
    app = _build_app(verify_client="off")
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "root"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_header_stripped_when_trust_unset():
    # Rule 9: "the sidecar verifies client certs" is a claim an operator makes
    # explicitly. Inferring it from an unset variable is how "nobody wired
    # this" became indistinguishable from "deliberately open".
    os.environ.pop("PEER_IDENTITY_TRUSTED", None)
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_header_stripped_when_not_from_sidecar():
    # The published plaintext port bypasses the sidecar entirely, so a header
    # arriving on it was written by the caller. Trusting it launders the
    # attribution of every audit line the request produces.
    app = _build_app(verify_client="on")
    with TestClient(app, client=DIRECT) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_strict_rejects_peer_from_non_sidecar_transport():
    # The dependency the review names: once strict flips to True this is the
    # difference between an authentication bypass and a 401.
    app = _build_app(allowed=["recap-worker"], strict=True)
    with TestClient(app, client=DIRECT) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "recap-worker"})
        assert resp.status_code == 401


def test_strict_rejects_missing_peer():
    app = _build_app(strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo")
        assert resp.status_code == 401


def test_strict_rejects_disallowed_peer():
    app = _build_app(allowed=["recap-worker"], strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "evil-svc"})
        assert resp.status_code == 403


def test_strict_accepts_allowlisted_peer():
    app = _build_app(allowed=["recap-worker", "mq-hub"], strict=True)
    with TestClient(app, client=SIDECAR) as client:
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
