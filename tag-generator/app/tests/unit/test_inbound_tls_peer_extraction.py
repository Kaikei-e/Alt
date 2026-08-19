"""Security tests: spoofed X-Alt-Peer-Identity must not win over TLS CN.

The in-process mTLS listener strips inbound identity headers and injects
the verified client-cert CN into ASGI scope. An untrusted client cert is
rejected at handshake and never reaches the app.
"""

from __future__ import annotations

import json
import ssl
import subprocess
from pathlib import Path

import httpx
import pytest
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.types import Receive, Scope, Send

from tag_generator.infra.inbound_tls import (
    build_inbound_ssl_context,
    serve_inbound_tls,
)
from tag_generator.infra.peer_identity import PEER_IDENTITY_HEADER, PeerIdentityMiddleware


def _write_ca(dir_path: Path, name: str) -> tuple[Path, Path]:
    cert_path = dir_path / f"{name}-ca-cert.pem"
    key_path = dir_path / f"{name}-ca-key.pem"
    subprocess.run(  # noqa: S603, S607
        [
            "openssl",  # noqa: S607 — test-only openssl
            "req",
            "-x509",
            "-newkey",
            "rsa:2048",
            "-keyout",
            str(key_path),
            "-out",
            str(cert_path),
            "-days",
            "2",
            "-nodes",
            "-subj",
            f"/CN={name}-ca",
        ],
        check=True,
        capture_output=True,
    )
    return cert_path, key_path


def _write_signed_leaf(dir_path: Path, cn: str, ca_cert: Path, ca_key: Path) -> tuple[Path, Path]:
    key_path = dir_path / f"{cn}-key.pem"
    csr_path = dir_path / f"{cn}.csr"
    cert_path = dir_path / f"{cn}-cert.pem"
    subprocess.run(  # noqa: S603, S607
        [
            "openssl",  # noqa: S607 — test-only openssl
            "req",
            "-new",
            "-newkey",
            "rsa:2048",
            "-keyout",
            str(key_path),
            "-out",
            str(csr_path),
            "-nodes",
            "-subj",
            f"/CN={cn}",
            "-addext",
            "subjectAltName=DNS:localhost,IP:127.0.0.1",
        ],
        check=True,
        capture_output=True,
    )
    subprocess.run(  # noqa: S603, S607
        [
            "openssl",  # noqa: S607 — test-only openssl
            "x509",
            "-req",
            "-in",
            str(csr_path),
            "-CA",
            str(ca_cert),
            "-CAkey",
            str(ca_key),
            "-CAcreateserial",
            "-out",
            str(cert_path),
            "-days",
            "2",
            "-copy_extensions",
            "copy",
        ],
        check=True,
        capture_output=True,
    )
    return cert_path, key_path


def _echo_peer(request):
    header = request.headers.get(PEER_IDENTITY_HEADER)
    return JSONResponse(
        {
            "peer": getattr(request.state, "peer_identity", None),
            "header": header,
        }
    )


def _app(*, allowed=None, strict=True):
    app = Starlette(routes=[Route("/echo", _echo_peer)])
    app.add_middleware(PeerIdentityMiddleware, allowed=allowed, strict=strict)
    return app


def _client_ctx(ca: Path, cert: Path, key: Path) -> ssl.SSLContext:
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_REQUIRED
    ctx.load_verify_locations(cafile=str(ca))
    ctx.load_cert_chain(str(cert), str(key))
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    return ctx


@pytest.mark.asyncio
async def test_spoofed_peer_header_is_ignored_for_tls_client(tmp_path: Path) -> None:
    ca_cert, ca_key = _write_ca(tmp_path, "alt")
    server_cert, server_key = _write_signed_leaf(tmp_path, "tag-generator", ca_cert, ca_key)
    client_cert, client_key = _write_signed_leaf(tmp_path, "recap-worker", ca_cert, ca_key)
    ctx = build_inbound_ssl_context(str(server_cert), str(server_key), str(ca_cert))
    app = _app(allowed=["recap-worker"], strict=True)
    server = await serve_inbound_tls(app, ctx, host="127.0.0.1", port=0, allowed_peers=("recap-worker",))
    try:
        port = server.sockets[0].getsockname()[1]
        client_ctx = _client_ctx(ca_cert, client_cert, client_key)
        async with httpx.AsyncClient(verify=client_ctx) as client:
            resp = await client.get(
                f"https://127.0.0.1:{port}/echo",
                headers={PEER_IDENTITY_HEADER: "spoofed-root"},
            )
        assert resp.status_code == 200
        body = resp.json()
        assert body["peer"] == "recap-worker"
        assert body["header"] == "recap-worker"
    finally:
        server.close()
        await server.wait_closed()


@pytest.mark.asyncio
async def test_untrusted_client_cert_never_reaches_app(tmp_path: Path) -> None:
    ca_cert, ca_key = _write_ca(tmp_path, "alt")
    other_ca, other_key = _write_ca(tmp_path, "other")
    server_cert, server_key = _write_signed_leaf(tmp_path, "tag-generator", ca_cert, ca_key)
    evil_cert, evil_key = _write_signed_leaf(tmp_path, "recap-worker", other_ca, other_key)
    ctx = build_inbound_ssl_context(str(server_cert), str(server_key), str(ca_cert))
    hits: list[str] = []

    async def marker_app(scope: Scope, receive: Receive, send: Send) -> None:
        if scope.get("type") == "http":
            hits.append("reached")
            await send({"type": "http.response.start", "status": 200, "headers": []})
            await send({"type": "http.response.body", "body": b"ok"})

    server = await serve_inbound_tls(marker_app, ctx, host="127.0.0.1", port=0)
    try:
        port = server.sockets[0].getsockname()[1]
        client_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        client_ctx.check_hostname = False
        client_ctx.verify_mode = ssl.CERT_NONE
        client_ctx.load_cert_chain(str(evil_cert), str(evil_key))
        with pytest.raises((ssl.SSLError, httpx.ConnectError, httpx.ReadError, httpx.RemoteProtocolError)):
            async with httpx.AsyncClient(verify=client_ctx) as client:
                await client.get(f"https://127.0.0.1:{port}/echo")
        assert hits == []
    finally:
        server.close()
        await server.wait_closed()


@pytest.mark.asyncio
async def test_allowlist_rejects_foreign_cn_at_tls_boundary(tmp_path: Path) -> None:
    ca_cert, ca_key = _write_ca(tmp_path, "alt")
    server_cert, server_key = _write_signed_leaf(tmp_path, "tag-generator", ca_cert, ca_key)
    client_cert, client_key = _write_signed_leaf(tmp_path, "evil-svc", ca_cert, ca_key)
    ctx = build_inbound_ssl_context(str(server_cert), str(server_key), str(ca_cert))
    app = _app(allowed=["recap-worker"], strict=True)
    server = await serve_inbound_tls(app, ctx, host="127.0.0.1", port=0, allowed_peers=("recap-worker",))
    try:
        port = server.sockets[0].getsockname()[1]
        client_ctx = _client_ctx(ca_cert, client_cert, client_key)
        with pytest.raises((httpx.ConnectError, httpx.ReadError, httpx.RemoteProtocolError, ssl.SSLError)):
            async with httpx.AsyncClient(verify=client_ctx) as client:
                await client.get(f"https://127.0.0.1:{port}/echo")
    finally:
        server.close()
        await server.wait_closed()


@pytest.mark.asyncio
async def test_asgi_scope_tls_cn_survives_json_roundtrip(tmp_path: Path) -> None:
    ca_cert, ca_key = _write_ca(tmp_path, "alt")
    server_cert, server_key = _write_signed_leaf(tmp_path, "tag-generator", ca_cert, ca_key)
    client_cert, client_key = _write_signed_leaf(tmp_path, "mq-hub", ca_cert, ca_key)
    ctx = build_inbound_ssl_context(str(server_cert), str(server_key), str(ca_cert))

    async def raw_app(scope: Scope, receive: Receive, send: Send) -> None:
        if scope.get("type") != "http":
            return
        tls = (scope.get("extensions") or {}).get("tls") or {}
        body = json.dumps({"client_cn": tls.get("client_cn")}).encode()
        await send(
            {
                "type": "http.response.start",
                "status": 200,
                "headers": [(b"content-type", b"application/json")],
            }
        )
        await send({"type": "http.response.body", "body": body})

    server = await serve_inbound_tls(raw_app, ctx, host="127.0.0.1", port=0)
    try:
        port = server.sockets[0].getsockname()[1]
        client_ctx = _client_ctx(ca_cert, client_cert, client_key)
        async with httpx.AsyncClient(verify=client_ctx) as client:
            resp = await client.get(
                f"https://127.0.0.1:{port}/echo",
                headers={PEER_IDENTITY_HEADER: "spoofed"},
            )
        assert resp.json() == {"client_cn": "mq-hub"}
    finally:
        server.close()
        await server.wait_closed()
