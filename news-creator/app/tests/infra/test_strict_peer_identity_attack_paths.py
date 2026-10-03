"""Attack-path tests for strict peer identity and D02 telemetry contract in news-creator."""

from __future__ import annotations

import logging
import secrets
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.testclient import TestClient

from news_creator.infra.inbound_tls import (
    forget_tls_peer,
    remember_tls_peer,
)
from news_creator.infra.peer_identity import (
    PEER_IDENTITY_HEADER,
    PeerIdentityMiddleware,
    strict_from_env,
)
from news_creator.otel import OTelConfig, init_otel_provider

SIDECAR = ("127.0.0.1", 44444)
DIRECT = ("172.18.0.9", 44444)


def _echo(request):
    return JSONResponse({"peer": getattr(request.state, "peer_identity", None)})


def _health(request):
    return JSONResponse({"status": "healthy"})


def _metrics(request):
    return JSONResponse({"metrics": "ok"})


def _build_test_app(allowed: list[str] | None = None, strict: bool = True) -> Starlette:
    app = Starlette(
        routes=[
            Route("/v1/summarize", _echo, methods=["POST"]),
            Route("/v1/generate", _echo, methods=["POST"]),
            Route("/health", _health, methods=["GET"]),
            Route("/health/deep", _health, methods=["GET"]),
            Route("/metrics", _metrics, methods=["GET"]),
        ]
    )
    app.add_middleware(PeerIdentityMiddleware, allowed=allowed, strict=strict)
    return app


def test_strict_default_is_true(monkeypatch: pytest.MonkeyPatch) -> None:
    """News strict peer identity must default to True when unset."""
    monkeypatch.delenv("PEER_IDENTITY_STRICT", raising=False)
    assert strict_from_env() is True


def test_anonymous_inference_rejected_on_plaintext(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Anonymous request to inference endpoint must be rejected with 401."""
    monkeypatch.delenv("PEER_IDENTITY_TRUSTED", raising=False)
    app = _build_test_app(allowed=["recap-worker", "alt-backend"], strict=True)
    with TestClient(app, client=DIRECT) as client:
        resp = client.post("/v1/summarize")
        assert resp.status_code == 401
        assert "unauthenticated peer" in resp.text


def test_spoofed_peer_header_rejected_on_direct_plaintext(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Spoofed peer header directly on plaintext must be rejected with 401."""
    monkeypatch.setenv(
        "PEER_IDENTITY_TRUSTED", "on"
    )  # Even if trusted is on, transport is DIRECT
    app = _build_test_app(allowed=["recap-worker", "alt-backend"], strict=True)
    with TestClient(app, client=DIRECT) as client:
        resp = client.post(
            "/v1/summarize", headers={PEER_IDENTITY_HEADER: "alt-backend"}
        )
        assert resp.status_code == 401
        assert "unauthenticated peer" in resp.text


def test_verified_sidecar_peer_passes(monkeypatch: pytest.MonkeyPatch) -> None:
    """Legitimate sidecar-forwarded peer with trusted flag passes to usecase."""
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "on")
    app = _build_test_app(allowed=["alt-backend"], strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.post(
            "/v1/summarize", headers={PEER_IDENTITY_HEADER: "alt-backend"}
        )
        assert resp.status_code == 200
        assert resp.json() == {"peer": "alt-backend"}


def test_verified_tls_peer_passes() -> None:
    """TLS client cert peer in allowlist passes without extra token."""
    client_addr = ("10.0.8.20", 44444)
    remember_tls_peer(client_addr, "recap-worker")
    try:
        app = _build_test_app(allowed=["recap-worker"], strict=True)
        with TestClient(app, client=client_addr) as http:
            resp = http.post("/v1/summarize")
            assert resp.status_code == 200
            assert resp.json() == {"peer": "recap-worker"}
    finally:
        forget_tls_peer(client_addr)


def test_tls_peer_not_in_allowlist_rejected() -> None:
    """TLS client cert peer not in allowlist is rejected with 403."""
    client_addr = ("10.0.8.21", 44444)
    remember_tls_peer(client_addr, "unknown-service")
    try:
        app = _build_test_app(allowed=["recap-worker"], strict=True)
        with TestClient(app, client=client_addr) as http:
            resp = http.post("/v1/summarize")
            assert resp.status_code == 403
            assert "peer not allowlisted" in resp.text
    finally:
        forget_tls_peer(client_addr)


def test_health_and_metrics_exempt_on_plaintext(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Exact health/metrics paths are accessible on plaintext without credentials."""
    monkeypatch.delenv("PEER_IDENTITY_TRUSTED", raising=False)
    app = _build_test_app(allowed=["recap-worker"], strict=True)
    with TestClient(app, client=DIRECT) as client:
        assert client.get("/health").status_code == 200
        assert client.get("/health/deep").status_code == 200
        assert client.get("/metrics").status_code == 200


def test_health_and_metrics_exempt_on_tls() -> None:
    """Exact health/metrics paths are accessible via TLS even with unallowlisted CN."""
    client_addr = ("10.0.8.22", 44444)
    remember_tls_peer(client_addr, "untrusted-prober")
    try:
        app = _build_test_app(allowed=["recap-worker"], strict=True)
        with TestClient(app, client=client_addr) as http:
            assert http.get("/health").status_code == 200
            assert http.get("/health/deep").status_code == 200
            assert http.get("/metrics").status_code == 200
    finally:
        forget_tls_peer(client_addr)


# ---------------------------------------------------------------------------
# D02 Telemetry Exporter Tests
# ---------------------------------------------------------------------------


def test_otel_enabled_without_token_file_fails_fast(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    """When OTEL is enabled, missing RASK_INGEST_TOKEN_FILE must raise RuntimeError."""
    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(tmp_path / "nonexistent_token"))
    cfg = OTelConfig()
    with pytest.raises(RuntimeError, match="RASK_INGEST_TOKEN_FILE"):
        init_otel_provider(cfg)


def test_otel_enabled_with_empty_token_file_fails_fast(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    """When OTEL is enabled, empty RASK_INGEST_TOKEN_FILE must raise RuntimeError."""
    token_file = tmp_path / "empty_token"
    token_file.write_text("   \n")
    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(token_file))
    cfg = OTelConfig()
    with pytest.raises(RuntimeError, match="RASK_INGEST_TOKEN_FILE"):
        init_otel_provider(cfg)


def test_otel_enabled_with_valid_token_sets_bearer_header(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    """When OTEL is enabled with valid token, Authorization: Bearer header is added to exporters."""
    token_val = secrets.token_urlsafe(32)
    token_file = tmp_path / "rask_token"
    token_file.write_text(token_val)
    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(token_file))

    cfg = OTelConfig()
    with (
        patch("news_creator.otel.OTLPSpanExporter") as mock_span_exp,
        patch("news_creator.otel.OTLPLogExporter") as mock_log_exp,
        patch("news_creator.otel.BatchSpanProcessor"),
        patch("news_creator.otel.BatchLogRecordProcessor"),
        patch("news_creator.otel.PrometheusMetricReader"),
        patch("news_creator.otel.LoggingInstrumentor"),
        patch("news_creator.otel.LoggingHandler") as mock_logging_handler,
    ):
        mock_handler_inst = MagicMock()
        mock_handler_inst.level = logging.NOTSET
        mock_logging_handler.return_value = mock_handler_inst
        shutdown = init_otel_provider(cfg)
        try:
            # Check headers passed to OTLPSpanExporter
            _, span_kwargs = mock_span_exp.call_args
            assert span_kwargs.get("headers") == {
                "Authorization": f"Bearer {token_val}"
            }

            # Check headers passed to OTLPLogExporter
            _, log_kwargs = mock_log_exp.call_args
            assert log_kwargs.get("headers") == {"Authorization": f"Bearer {token_val}"}
        finally:
            shutdown()


def test_otel_disabled_does_not_require_token_file(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    """When OTEL is disabled, RASK_INGEST_TOKEN_FILE is not loaded and shutdown is a no-op."""
    monkeypatch.setenv("OTEL_ENABLED", "false")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(tmp_path / "nonexistent_token"))
    cfg = OTelConfig()
    shutdown = init_otel_provider(cfg)
    assert callable(shutdown)
    shutdown()


def test_health_prefix_attack_rejected_on_plaintext(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Non-exact health paths (like /health-admin or /healthbypass) must not be exempt and be rejected."""
    monkeypatch.delenv("PEER_IDENTITY_TRUSTED", raising=False)
    app = _build_test_app(allowed=["recap-worker"], strict=True)
    with TestClient(app, client=DIRECT) as client:
        resp = client.get("/health-bypass")
        assert resp.status_code == 401


def test_strict_blank_allowlist_rejects_valid_cn_on_real_tls() -> None:
    """When allowlist is blank/empty, even a valid TLS CN must be rejected with 403."""
    client_addr = ("10.0.8.25", 44444)
    remember_tls_peer(client_addr, "alt-backend")
    try:
        app = _build_test_app(allowed=[], strict=True)
        with TestClient(app, client=client_addr) as http:
            resp = http.post("/v1/summarize")
            assert resp.status_code == 403
            assert "peer not allowlisted" in resp.text
    finally:
        forget_tls_peer(client_addr)


def test_strict_matching_allowlist_accepts_valid_cn_on_real_tls() -> None:
    """When allowlist contains the valid TLS CN, real TLS request succeeds with 200."""
    client_addr = ("10.0.8.26", 44444)
    remember_tls_peer(client_addr, "alt-backend")
    try:
        app = _build_test_app(allowed=["alt-backend"], strict=True)
        with TestClient(app, client=client_addr) as http:
            resp = http.post("/v1/summarize")
            assert resp.status_code == 200
            assert resp.json() == {"peer": "alt-backend"}
    finally:
        forget_tls_peer(client_addr)


def test_strict_blank_allowlist_rejects_valid_cn_on_trusted_proxy(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """When allowlist is blank/empty, even a valid CN via trusted proxy sidecar must be rejected with 403."""
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "on")
    app = _build_test_app(allowed=[], strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.post(
            "/v1/summarize", headers={PEER_IDENTITY_HEADER: "alt-backend"}
        )
        assert resp.status_code == 403
        assert "peer not allowlisted" in resp.text


def test_strict_matching_allowlist_accepts_valid_cn_on_trusted_proxy(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """When allowlist contains the valid CN, trusted proxy request succeeds with 200."""
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "on")
    app = _build_test_app(allowed=["alt-backend"], strict=True)
    with TestClient(app, client=SIDECAR) as client:
        resp = client.post(
            "/v1/summarize", headers={PEER_IDENTITY_HEADER: "alt-backend"}
        )
        assert resp.status_code == 200
        assert resp.json() == {"peer": "alt-backend"}
