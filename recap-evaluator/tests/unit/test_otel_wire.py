"""D02 wire tests - recap-evaluator OTLP provider.

Tests run the real ``init_otel_provider()`` production entrypoint in an
isolated child process per test to ensure:
- 1 provider per process (clean cleanup of handlers and thread pools)
- Genuine SDK Once flags are never corrupted across tests
- No vacuous passes from dropped spans/logs

Environment contract verified:
- Both /v1/traces and /v1/logs receive Authorization: Bearer <token>
- Content-Type is application/x-protobuf; body decodes to real span/log records
- 307 redirect: positive origin receives BOTH routes, destination receives ZERO requests
- Separate trace/log compression env preserves correct body/header decode (no cross-contamination)
- Missing / invalid token file with telemetry enabled → RuntimeError (fail-closed, secret never leaked)
- Disabled telemetry → no file needed, no-op shutdown returned
"""

import gzip
import os
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

import pytest
from opentelemetry.proto.collector.logs.v1.logs_service_pb2 import ExportLogsServiceRequest
from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import ExportTraceServiceRequest

_FAKE_TOKEN = "ValidRaskIngestToken1234567890abcdefgh"


# ---------------------------------------------------------------------------
# HTTP handlers & server helpers
# ---------------------------------------------------------------------------


class _CollectingHandler(BaseHTTPRequestHandler):
    """Captures every POST: path, Authorization, Content-Type, Content-Encoding, body."""

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        self.server.requests.append(
            {
                "path": self.path,
                "auth": self.headers.get("Authorization"),
                "content_type": self.headers.get("Content-Type"),
                "content_encoding": self.headers.get("Content-Encoding"),
                "body": body,
            }
        )
        self.send_response(200)
        self.send_header("Content-Type", "application/x-protobuf")
        self.end_headers()
        self.wfile.write(b"")

    def log_message(self, fmt, *args):
        pass


class _RedirectHandler(BaseHTTPRequestHandler):
    """Returns 307 for every POST; directs to destination server base."""

    def do_POST(self):
        self.server.requests.append(self.path)
        dest_base = getattr(self.server, "destination_base", "http://127.0.0.1:1")
        if self.path == "/v1/traces":
            dest_path = "/v1/traces"
        elif self.path == "/v1/logs":
            dest_path = "/v1/logs"
        else:
            dest_path = "/"
        self.send_response(307)
        self.send_header("Location", f"{dest_base}{dest_path}")
        self.end_headers()

    def log_message(self, fmt, *args):
        pass


class _DestinationHandler(BaseHTTPRequestHandler):
    """Destination server: should receive ZERO requests when redirect guard is active."""

    def do_POST(self):
        self.server.hit_count += 1
        self.send_response(200)
        self.end_headers()

    def log_message(self, fmt, *args):
        pass


def _start_server(handler_cls, **extra_attrs):
    srv = HTTPServer(("127.0.0.1", 0), handler_cls)
    srv.requests = []
    for k, v in extra_attrs.items():
        setattr(srv, k, v)
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    return srv, thread


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def rask_token_file(tmp_path):
    f = tmp_path / "rask_ingest_token"
    f.write_text(_FAKE_TOKEN)
    return f


@pytest.fixture
def otlp_server():
    srv, thread = _start_server(_CollectingHandler)
    yield srv
    srv.shutdown()
    thread.join(timeout=5)


@pytest.fixture
def redirect_server():
    srv, thread = _start_server(_RedirectHandler)
    yield srv
    srv.shutdown()
    thread.join(timeout=5)


@pytest.fixture
def destination_server():
    srv, thread = _start_server(_DestinationHandler, hit_count=0)
    yield srv
    srv.shutdown()
    thread.join(timeout=5)


# ---------------------------------------------------------------------------
# Child process runner & protobuf decode helpers
# ---------------------------------------------------------------------------


def _run_client_in_subprocess(
    *,
    token_file: str,
    endpoint: str,
    extra_env: dict[str, str] | None = None,
    span_name: str = "evaluator-wire-span",
    log_body: str = "evaluator-wire-log",
) -> None:
    code = f"""
import os, sys
import opentelemetry.trace as trace_api
import opentelemetry._logs as logs_api
from opentelemetry._logs import LogRecord, SeverityNumber
from recap_evaluator.utils.otel import OTelConfig, init_otel_provider

config = OTelConfig()
shutdown = init_otel_provider(config)
try:
    tracer = trace_api.get_tracer("wire-client")
    with tracer.start_as_current_span({span_name!r}):
        pass

    otel_logger = logs_api.get_logger_provider().get_logger("wire-client")
    otel_logger.emit(
        LogRecord(
            body={log_body!r},
            severity_number=SeverityNumber.INFO,
            severity_text="INFO",
        )
    )
finally:
    shutdown()
"""
    env = os.environ.copy()
    src_dir = Path("src").resolve()
    env["PYTHONPATH"] = f"{src_dir}:{os.pathsep.join(sys.path)}"
    env["OTEL_ENABLED"] = "true"
    env["RASK_INGEST_TOKEN_FILE"] = str(token_file)
    env["OTEL_EXPORTER_OTLP_ENDPOINT"] = endpoint
    env["OTEL_SERVICE_NAME"] = "recap-evaluator-wire"
    env.pop("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", None)
    env.pop("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", None)
    if extra_env:
        env.update(extra_env)

    res = subprocess.run(
        [sys.executable, "-c", code],
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode != 0:
        raise RuntimeError(
            f"Child process failed (code {res.returncode}):\nSTDOUT: {res.stdout}\nSTDERR: {res.stderr}"
        )


def _decode_spans(body: bytes, content_encoding: str | None) -> list[str]:
    raw = gzip.decompress(body) if content_encoding == "gzip" else body
    req = ExportTraceServiceRequest()
    req.ParseFromString(raw)
    return [span.name for rs in req.resource_spans for ss in rs.scope_spans for span in ss.spans]


def _decode_logs(body: bytes, content_encoding: str | None) -> list[str]:
    raw = gzip.decompress(body) if content_encoding == "gzip" else body
    req = ExportLogsServiceRequest()
    req.ParseFromString(raw)
    return [
        lr_record.body.string_value
        for rl in req.resource_logs
        for sl in rl.scope_logs
        for lr_record in sl.log_records
    ]


# ---------------------------------------------------------------------------
# Tests (Target 5 tests)
# ---------------------------------------------------------------------------


def test_init_otel_provider_sends_bearer_to_traces_and_logs(rask_token_file, otlp_server):
    """init_otel_provider wires Bearer token and emits decoded span + log records."""
    endpoint = f"http://127.0.0.1:{otlp_server.server_port}"
    _run_client_in_subprocess(
        token_file=str(rask_token_file),
        endpoint=endpoint,
        span_name="test-span-evaluator-wire",
        log_body="test-log-evaluator-wire",
    )

    by_path = {r["path"]: r for r in otlp_server.requests}
    assert "/v1/traces" in by_path, f"No traces received; got {list(by_path.keys())}"
    assert "/v1/logs" in by_path, f"No logs received; got {list(by_path.keys())}"

    tr = by_path["/v1/traces"]
    lr = by_path["/v1/logs"]

    # Strict Bearer token check
    assert tr["auth"] == f"Bearer {_FAKE_TOKEN}"
    assert lr["auth"] == f"Bearer {_FAKE_TOKEN}"

    # Protobuf content type
    assert "application/x-protobuf" in tr["content_type"]
    assert "application/x-protobuf" in lr["content_type"]

    # Decode actual inner protobuf records
    spans = _decode_spans(tr["body"], tr["content_encoding"])
    logs = _decode_logs(lr["body"], lr["content_encoding"])
    assert "test-span-evaluator-wire" in spans, f"Expected span not found; got {spans}"
    assert "test-log-evaluator-wire" in logs, f"Expected log not found; got {logs}"


def test_redirect_307_hits_destination_zero_times(
    rask_token_file, redirect_server, destination_server
):
    """307 redirect: positive origin receives BOTH routes before distinct redirect dest 0."""
    dest_base = f"http://127.0.0.1:{destination_server.server_port}"
    redirect_server.destination_base = dest_base
    endpoint = f"http://127.0.0.1:{redirect_server.server_port}"

    _run_client_in_subprocess(
        token_file=str(rask_token_file),
        endpoint=endpoint,
        span_name="redir-span-evaluator",
        log_body="redir-log-evaluator",
    )

    # Positive origin receives BOTH routes
    assert "/v1/traces" in redirect_server.requests, "Origin did not receive /v1/traces"
    assert "/v1/logs" in redirect_server.requests, "Origin did not receive /v1/logs"

    # Distinct destination receives ZERO hits
    assert destination_server.hit_count == 0, (
        f"Redirect was followed! Destination received {destination_server.hit_count} hits"
    )


def test_separate_trace_log_compression_no_cross_contamination(rask_token_file, otlp_server):
    """Separate trace/log compression env preserves correct body/header decode (no contamination)."""
    endpoint = f"http://127.0.0.1:{otlp_server.server_port}"

    # Case 1: Trace gzip, Log none
    _run_client_in_subprocess(
        token_file=str(rask_token_file),
        endpoint=endpoint,
        extra_env={
            "OTEL_EXPORTER_OTLP_TRACES_COMPRESSION": "gzip",
            "OTEL_EXPORTER_OTLP_LOGS_COMPRESSION": "none",
        },
        span_name="span-gzip-evaluator",
        log_body="log-plain-evaluator",
    )

    by_path = {r["path"]: r for r in otlp_server.requests}
    tr = by_path["/v1/traces"]
    lr = by_path["/v1/logs"]

    assert tr["content_encoding"] == "gzip", (
        f"Expected trace gzip encoding; got {tr['content_encoding']}"
    )
    assert lr["content_encoding"] is None or lr["content_encoding"] == "none", (
        f"Log was contaminated with gzip encoding! Got {lr['content_encoding']}"
    )

    spans = _decode_spans(tr["body"], tr["content_encoding"])
    logs = _decode_logs(lr["body"], lr["content_encoding"])
    assert "span-gzip-evaluator" in spans
    assert "log-plain-evaluator" in logs

    # Case 2: Vice versa - Trace none, Log gzip
    otlp_server.requests.clear()
    _run_client_in_subprocess(
        token_file=str(rask_token_file),
        endpoint=endpoint,
        extra_env={
            "OTEL_EXPORTER_OTLP_TRACES_COMPRESSION": "none",
            "OTEL_EXPORTER_OTLP_LOGS_COMPRESSION": "gzip",
        },
        span_name="span-plain-evaluator",
        log_body="log-gzip-evaluator",
    )

    by_path2 = {r["path"]: r for r in otlp_server.requests}
    tr2 = by_path2["/v1/traces"]
    lr2 = by_path2["/v1/logs"]

    assert tr2["content_encoding"] is None or tr2["content_encoding"] == "none", (
        f"Trace was contaminated with gzip encoding! Got {tr2['content_encoding']}"
    )
    assert lr2["content_encoding"] == "gzip", (
        f"Expected log gzip encoding; got {lr2['content_encoding']}"
    )

    spans2 = _decode_spans(tr2["body"], tr2["content_encoding"])
    logs2 = _decode_logs(lr2["body"], lr2["content_encoding"])
    assert "span-plain-evaluator" in spans2
    assert "log-gzip-evaluator" in logs2


def test_missing_or_invalid_token_file_raises_when_enabled(tmp_path, monkeypatch):
    """Missing or invalid token file with OTEL_ENABLED=true raises RuntimeError fail-closed."""
    from recap_evaluator.utils.otel import OTelConfig, init_otel_provider

    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.setenv("OTEL_SERVICE_NAME", "recap-evaluator-err-test")

    # Missing file
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(tmp_path / "nonexistent"))
    with pytest.raises(RuntimeError, match="RASK_INGEST_TOKEN_FILE"):
        init_otel_provider(OTelConfig())

    # Invalid token content (spaces, secret not echoed)
    bad_token = tmp_path / "bad_token"
    bad_val = "bad token with spaces"
    bad_token.write_text(bad_val)
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(bad_token))
    with pytest.raises(RuntimeError) as exc_info:
        init_otel_provider(OTelConfig())
    assert bad_val not in str(exc_info.value), "Secret leaked in exception message"


def test_disabled_returns_noop_without_token_file(monkeypatch):
    """OTEL_ENABLED=false does not read file and returns no-op shutdown."""
    from recap_evaluator.utils.otel import OTelConfig, init_otel_provider

    monkeypatch.setenv("OTEL_ENABLED", "false")
    monkeypatch.delenv("RASK_INGEST_TOKEN_FILE", raising=False)

    shutdown = init_otel_provider(OTelConfig())
    assert callable(shutdown)
    shutdown()
