"""Characterization tests for inbound_server dual listener wiring."""

from __future__ import annotations

import asyncio
import ssl
from asyncio import CancelledError
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from fastapi import FastAPI

from recap_subworker.app.infra import inbound_server

if TYPE_CHECKING:
    from recap_subworker.app.infra.inbound_tls import InboundCertReloader

PLAINTEXT_PORT = 8002
MTLS_PORT = 9443
KEEP_ALIVE_SECONDS = 5
EXPECTED_SERVER_COUNT = 2


@pytest.mark.asyncio
async def test_serve_after_enrollment_dual_starts_plain_and_mtls_servers(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    served_configs: list[object] = []
    watcher_cancelled = False

    class _FakeServer:
        def __init__(self, config: object) -> None:
            self.config = config
            served_configs.append(config)

        async def serve(self) -> None:
            await asyncio.sleep(0)

    async def _fake_watch(_reloader: InboundCertReloader) -> None:
        nonlocal watcher_cancelled
        try:
            await asyncio.sleep(10)
        except CancelledError:
            watcher_cancelled = True
            raise

    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    monkeypatch.setenv("MTLS_CERT_FILE", str(tmp_path / "cert.pem"))
    monkeypatch.setenv("MTLS_KEY_FILE", str(tmp_path / "key.pem"))
    monkeypatch.setattr(inbound_server, "load_inbound_ssl_context_from_env", lambda: ctx)
    monkeypatch.setattr(inbound_server, "create_app", FastAPI)
    monkeypatch.setattr(inbound_server, "InboundCertReloader", lambda _c, _cert, _key: object())
    monkeypatch.setattr(inbound_server.uvicorn, "Server", _FakeServer)
    monkeypatch.setattr(inbound_server, "watch_inbound_cert_rotation", _fake_watch)
    monkeypatch.setattr(inbound_server, "inbound_mtls_port", lambda: MTLS_PORT)
    monkeypatch.setattr(inbound_server, "wrap_http_protocol", lambda cls: cls)
    monkeypatch.setattr(inbound_server, "ssl_context_factory_for", lambda c: lambda _cfg, _d: c)

    await asyncio.to_thread(
        inbound_server._serve_after_enrollment,
        host="127.0.0.1",
        port=PLAINTEXT_PORT,
        log_level="info",
        keep_alive=KEEP_ALIVE_SECONDS,
    )

    assert len(served_configs) == EXPECTED_SERVER_COUNT
    assert watcher_cancelled is True
