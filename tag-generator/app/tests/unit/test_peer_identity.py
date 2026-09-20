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
    resolve_authenticated_peer,
    strict_from_env,
)

# pki-agent shares this container's network namespace and proxies to
# 127.0.0.1:9400, so a loopback transport peer is the sidecar. Anything else
# reached the published plaintext port directly.
SIDECAR = ("127.0.0.1", 44444)
DIRECT = ("172.18.0.9", 44444)


def _echo_peer(request):
    return JSONResponse({"peer": getattr(request.state, "peer_identity", None)})


class _TLSInjectionMiddleware:
    """Local test wrapper injecting the ASGI TLS extension into scope.

    Matches inbound_tls.py:271-279 where uvicorn sets
    scope['extensions']['tls']['client_cn'] = cn.
    """

    def __init__(self, app: object, client_cn: str = "") -> None:
        self.app = app
        self.client_cn = client_cn

    async def __call__(self, scope: dict[str, object], receive: object, send: object) -> None:
        if scope.get("type") == "http":
            extensions = scope.setdefault("extensions", {})
            if isinstance(extensions, dict):
                extensions["tls"] = {"client_cn": self.client_cn}
        await self.app(scope, receive, send)  # type: ignore[misc]


def _build_app(
    *,
    allowed: list[str] | None = None,
    strict: bool = False,
    verify_client: str = "on",
    tls_cn: str | None = None,
    routes: list[Route] | None = None,
) -> Starlette:
    os.environ["PEER_IDENTITY_TRUSTED"] = verify_client
    app_routes = routes or [
        Route("/echo", _echo_peer),
        Route("/health", _echo_peer),
        Route("/metrics", _echo_peer),
    ]
    app = Starlette(routes=app_routes)
    app.add_middleware(PeerIdentityMiddleware, allowed=allowed, strict=strict)
    if tls_cn is not None:
        app.add_middleware(_TLSInjectionMiddleware, client_cn=tls_cn)
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
    os.environ.pop("PEER_IDENTITY_TRUSTED", None)
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_header_stripped_when_not_from_sidecar():
    app = _build_app(verify_client="on")
    with TestClient(app, client=DIRECT) as client:
        resp = client.get("/echo", headers={PEER_IDENTITY_HEADER: "alt-backend"})
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_strict_rejects_peer_from_non_sidecar_transport():
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


def test_tls_cn_wins_over_spoofed_header():
    from starlette.requests import Request

    os.environ["PEER_IDENTITY_TRUSTED"] = "on"
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
        "extensions": {"tls": {"client_cn": "recap-worker"}},
    }
    assert resolve_authenticated_peer(Request(scope)) == "recap-worker"


def test_tls_cn_is_used_even_when_trust_env_is_off():
    from starlette.requests import Request

    os.environ["PEER_IDENTITY_TRUSTED"] = "off"
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
        "extensions": {"tls": {"client_cn": "mq-hub"}},
    }
    assert resolve_authenticated_peer(Request(scope)) == "mq-hub"


# ==============================================================================
# SEC-07 Regression Tests: In-process mTLS allowlist enforcement & bypass guards
# ==============================================================================


def test_strict_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """Verify strict_from_env defaults to False and parses truthy/falsy values."""
    monkeypatch.delenv("PEER_IDENTITY_STRICT", raising=False)
    assert strict_from_env() is False

    for truthy in ("true", "1", "on", "yes", "TRUE", "On", "YES"):
        monkeypatch.setenv("PEER_IDENTITY_STRICT", truthy)
        assert strict_from_env() is True

    for falsy in ("false", "0", "off", "no", "", "random", "DISABLED"):
        monkeypatch.setenv("PEER_IDENTITY_STRICT", falsy)
        assert strict_from_env() is False


def test_auth_service_wires_strict_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """Verify auth_service configures PeerIdentityMiddleware strictness from env."""
    import importlib

    import auth_service

    original_app = auth_service.app
    try:
        with monkeypatch.context() as patch:
            patch.setenv("PEER_IDENTITY_STRICT", "true")
            importlib.reload(auth_service)
            middlewares = [
                m for m in auth_service.app.user_middleware if getattr(m, "cls", None) is PeerIdentityMiddleware
            ]
            assert len(middlewares) == 1
            assert middlewares[0].kwargs.get("strict") is True
    finally:
        auth_service.app = original_app


def test_tls_peer_allowlisted_passes_even_when_not_strict() -> None:
    """Verified client cert with allowed CN is admitted under strict=False."""
    app = _build_app(allowed=["recap-worker"], strict=False, tls_cn="recap-worker")
    with TestClient(app, client=DIRECT) as http:
        resp = http.get("/echo")
        assert resp.status_code == 200
        assert resp.json() == {"peer": "recap-worker"}


def test_tls_peer_not_allowlisted_rejected_even_when_not_strict() -> None:
    """SEC-07: Verified client cert with disallowed CN must be rejected (403), even when strict=False."""
    app = _build_app(allowed=["recap-worker"], strict=False, tls_cn="impostor")
    with TestClient(app, client=DIRECT) as http:
        resp = http.get("/echo")
        assert resp.status_code == 403
        assert resp.text == "peer not allowlisted"


@pytest.mark.parametrize("allowed_peers", [["recap-worker"], None])
def test_tls_peer_empty_cn_rejected_even_when_not_strict(allowed_peers: list[str] | None) -> None:
    """SEC-07: Leaf certificate missing CN must be rejected (403) whether allowlist is set or empty."""
    app = _build_app(allowed=allowed_peers, strict=False, tls_cn="")
    with TestClient(app, client=DIRECT) as http:
        resp = http.get("/echo")
        assert resp.status_code == 403
        assert resp.text == "peer not allowlisted"


def test_tls_peer_empty_cn_cannot_bypass_via_forged_loopback_header() -> None:
    """SEC-07: TLS leaf missing CN connecting from loopback must FAIL (403) despite forged header."""
    app = _build_app(allowed=["recap-worker"], strict=False, verify_client="on", tls_cn="")
    with TestClient(app, client=SIDECAR) as http:
        resp = http.get("/echo", headers={PEER_IDENTITY_HEADER: "recap-worker"})
        assert resp.status_code == 403
        assert resp.text == "peer not allowlisted"


def test_tls_disallowed_peer_cannot_bypass_via_forged_loopback_header() -> None:
    """SEC-07: Disallowed TLS CN connecting from loopback cannot bypass allowlist with forged header."""
    app = _build_app(allowed=["recap-worker"], strict=False, verify_client="on", tls_cn="impostor")
    with TestClient(app, client=SIDECAR) as http:
        resp = http.get("/echo", headers={PEER_IDENTITY_HEADER: "recap-worker"})
        assert resp.status_code == 403
        assert resp.text == "peer not allowlisted"


@pytest.mark.parametrize("path", ["/health", "/metrics"])
@pytest.mark.parametrize("cn", ["", "impostor"])
def test_probes_on_tls_port_exempt_from_allowlist(path: str, cn: str) -> None:
    """Documented exact probes (/health, /metrics) are exempt from TLS allowlist checks."""
    app = _build_app(allowed=["recap-worker"], strict=False, tls_cn=cn)
    with TestClient(app, client=DIRECT) as http:
        resp = http.get(path)
        assert resp.status_code == 200


def test_plaintext_origin_without_cert_unaffected_by_tls_allowlist() -> None:
    """Plaintext requests without cert remain governed by strict=False (legacy behavior)."""
    app = _build_app(allowed=["recap-worker"], strict=False, verify_client="off")
    with TestClient(app, client=DIRECT) as http:
        resp = http.get("/echo")
        assert resp.status_code == 200
        assert resp.json() == {"peer": None}


def test_plaintext_origin_rejected_when_strict_true() -> None:
    """Plaintext requests without cert are rejected with 401 when strict=True."""
    app = _build_app(allowed=["recap-worker"], strict=True, verify_client="off")
    with TestClient(app, client=DIRECT) as http:
        resp = http.get("/echo")
        assert resp.status_code == 401
        assert resp.text == "unauthenticated peer"


@pytest.mark.asyncio
async def test_raw_asgi_tls_missing_cn_rejected_even_with_forged_loopback_header() -> None:
    """Direct ASGI scope test: empty TLS client_cn + loopback forged header must return 403."""
    os.environ["PEER_IDENTITY_TRUSTED"] = "on"
    called = False

    async def call_next(r):
        nonlocal called
        called = True
        return JSONResponse({"peer": getattr(r.state, "peer_identity", None)})

    from starlette.requests import Request

    middleware = PeerIdentityMiddleware(None, allowed=["recap-worker"], strict=False)

    scope = {
        "type": "http",
        "asgi": {"version": "3.0", "spec_version": "2.3"},
        "http_version": "1.1",
        "method": "GET",
        "scheme": "https",
        "path": "/echo",
        "raw_path": b"/echo",
        "query_string": b"",
        "headers": [(b"x-alt-peer-identity", b"recap-worker")],
        "client": ("127.0.0.1", 44444),
        "server": ("127.0.0.1", 9443),
        "root_path": "",
        "extensions": {"tls": {"client_cn": ""}},
    }

    resp = await middleware.dispatch(Request(scope), call_next)
    assert resp.status_code == 403
    assert not called


@pytest.fixture(autouse=True)
def _reset_env():
    yield
    os.environ.pop("PEER_IDENTITY_TRUSTED", None)
    os.environ.pop("PEER_IDENTITY_STRICT", None)
