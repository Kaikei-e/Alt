"""Dedicated loopback ops listener for PKI /health and /metrics."""

from __future__ import annotations

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from prometheus_client import CONTENT_TYPE_LATEST, CollectorRegistry, generate_latest

DEFAULT_OPS_LISTEN = "127.0.0.1:9110"
OPS_LISTEN_ENV = "OPS_LISTEN"


def ops_listen_addr(environ: dict[str, str] | None = None) -> str:
    env = os.environ if environ is None else environ
    raw = str(env.get(OPS_LISTEN_ENV, "")).strip()
    if raw:
        return raw
    return DEFAULT_OPS_LISTEN


class _OpsHTTPServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(
        self,
        addr: tuple[str, int],
        service_name: str,
        registry: CollectorRegistry | None,
    ) -> None:
        super().__init__(addr, _OpsHandler)
        self.service_name = service_name
        self.registry = registry


class _OpsHandler(BaseHTTPRequestHandler):
    def log_message(self, format: str, *args: object) -> None:
        _ = (format, args)

    def do_GET(self) -> None:
        path = self.path.split("?", 1)[0]
        server = self.server
        assert isinstance(server, _OpsHTTPServer)
        if path == "/health":
            body = json.dumps({"status": "healthy", "service": server.service_name}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if path == "/metrics":
            if server.registry is None:
                msg = b"metrics exporter is not wired: PKI enrollment is disabled or has no private registry\n"
                self.send_response(503)
                self.send_header("Content-Type", "text/plain; charset=utf-8")
                self.send_header("Content-Length", str(len(msg)))
                self.end_headers()
                self.wfile.write(msg)
                return
            body = generate_latest(server.registry)
            self.send_response(200)
            self.send_header("Content-Type", CONTENT_TYPE_LATEST)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_error(404)


class OpsHandle:
    """Running ops listener, stopped with the enrollment handle."""

    def __init__(self, server: _OpsHTTPServer, thread: threading.Thread) -> None:
        self._server = server
        self._thread = thread
        host, port = server.server_address[:2]
        self.addr = f"{host}:{port}"

    def aclose_sync(self) -> None:
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(timeout=5)

    async def aclose(self) -> None:
        self.aclose_sync()


def start_ops(service_name: str, registry: CollectorRegistry | None, *, listen: str | None = None) -> OpsHandle:
    addr = listen if listen is not None else ops_listen_addr()
    if addr.startswith(":"):
        host, port_s = "0.0.0.0", addr[1:]  # noqa: S104 — OPS_LISTEN=:9110 is the compose scrape bind
    elif ":" in addr:
        host, port_s = addr.rsplit(":", 1)
    else:
        host, port_s = "127.0.0.1", addr
    port = int(port_s) if port_s else 9110
    server = _OpsHTTPServer((host, port), service_name, registry)
    thread = threading.Thread(target=server.serve_forever, daemon=True, name="pki-ops")
    thread.start()
    return OpsHandle(server, thread)
