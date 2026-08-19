"""Tests for peer-identity middleware."""

from __future__ import annotations

import pytest
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.testclient import TestClient

from acolyte.infra.peer_identity import (
    PEER_IDENTITY_HEADER,
    PeerIdentityMiddleware,
    allowed_peers_from_env,
    resolve_authenticated_peer,
)

# pki-agent shares this container's network namespace and proxies to
# 127.0.0.1:8090, so a loopback transport peer is the sidecar. Anything else
# reached the published plaintext port directly.
SIDECAR = ("127.0.0.1", 44444)
DIRECT = ("172.18.0.9", 44444)


def _echo_peer(request: Request) -> JSONResponse:
    return JSONResponse({"peer": getattr(request.state, "peer_identity", None)})


def _build_app(
    monkeypatch: pytest.MonkeyPatch,
    *,
    allowed: list[str] | None = None,
    strict: bool = False,
    verify_client: str = "on",
) -> Starlette:
    # monkeypatch.setenv restores the prior value (or unsets it again if it
    # was previously unset) automatically at test teardown — no manual
    # os.environ.pop bookkeeping, and no risk of leaking a stomped-over
    # pre-existing PEER_IDENTITY_TRUSTED value into unrelated tests.
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
    # VERIFY_CLIENT=off means the sidecar is NOT authenticating peers,
    # so any X-Alt-Peer-Identity on the wire is attacker-controlled and
    # must be dropped — never propagated into request.state.
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


def test_strict_rejects_missing_peer(monkeypatch: pytest.MonkeyPatch) -> None:
    app = _build_app(monkeypatch, strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo")
        assert resp.status_code == 401


def test_strict_rejects_disallowed_peer(monkeypatch: pytest.MonkeyPatch) -> None:
    app = _build_app(monkeypatch, allowed=["alt-backend"], strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "evil-svc"})
        assert resp.status_code == 403


def test_strict_accepts_allowlisted_peer(monkeypatch: pytest.MonkeyPatch) -> None:
    app = _build_app(monkeypatch, allowed=["alt-backend", "bff"], strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "bff"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": "bff"}


def test_allowed_peers_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", " alt-backend , bff ,  , rag-orchestrator")
    assert allowed_peers_from_env() == ["alt-backend", "bff", "rag-orchestrator"]


def test_allowed_peers_from_env_empty(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("MTLS_ALLOWED_PEERS", raising=False)
    assert allowed_peers_from_env() == []


def test_tls_cn_wins_over_spoofed_header(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "on")
    scope = {
        "type": "http",
        "asgi": {"version": "3.0", "spec_version": "2.3"},
        "http_version": "1.1",
        "method": "GET",
        "scheme": "https",
        "path": "/echo",
        "raw_path": b"/echo",
        "query_string": b"",
        "headers": [(b"x-alt-peer-identity", b"spoofed-root")],
        "client": DIRECT,
        "server": ("127.0.0.1", 9443),
        "root_path": "",
        "extensions": {"tls": {"client_cn": "alt-backend"}},
    }
    assert resolve_authenticated_peer(Request(scope)) == "alt-backend"


def test_tls_cn_is_used_even_when_trust_env_is_off(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "off")
    scope = {
        "type": "http",
        "asgi": {"version": "3.0", "spec_version": "2.3"},
        "http_version": "1.1",
        "method": "GET",
        "scheme": "https",
        "path": "/echo",
        "raw_path": b"/echo",
        "query_string": b"",
        "headers": [(b"x-alt-peer-identity", b"spoofed-root")],
        "client": DIRECT,
        "server": ("127.0.0.1", 9443),
        "root_path": "",
        "extensions": {"tls": {"client_cn": "bff"}},
    }
    assert resolve_authenticated_peer(Request(scope)) == "bff"
