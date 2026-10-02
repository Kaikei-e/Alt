"""
ABOUTME: OpenTelemetry provider for recap-subworker service.
ABOUTME: Handles trace and log export to OTLP endpoint with ADR 98 compliance.
ABOUTME: D02 – Bearer auth via RASK_INGEST_TOKEN_FILE; no-redirect safe session.
"""

import os
import re
from collections.abc import Callable
from pathlib import Path

import requests as _requests
from opentelemetry import trace
from opentelemetry._logs import set_logger_provider
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk._logs import LoggerProvider
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

# RFC 7235 token68: base64url + optional trailing '=' padding chars.
_TOKEN68_RE = re.compile(r"^[A-Za-z0-9\-._~+/]+=*\Z")

_DEFAULT_TIMEOUT_S = 7.0  # finite cap; SDK supplies its own value when calling post()


def _cap_timeout(timeout: object) -> object:
    """Cap finite connection and read timeouts at 7.0 seconds.

    Preserves shorter requested values. Avoids unbounded fallback.
    """
    if timeout is None:
        return _DEFAULT_TIMEOUT_S
    if isinstance(timeout, (int, float)):
        return min(float(timeout), _DEFAULT_TIMEOUT_S)
    if isinstance(timeout, (tuple, list)):
        return tuple(
            min(float(t), _DEFAULT_TIMEOUT_S) if isinstance(t, (int, float)) else _DEFAULT_TIMEOUT_S
            for t in timeout
        )
    return _DEFAULT_TIMEOUT_S


class _NoRedirectSession(_requests.Session):
    """requests.Session subclass that never follows redirects.

    Per D02: OTLP exporters must not silently POST credentials to a redirect
    destination. ``allow_redirects`` is forced ``False`` on every request so a
    307/308 from the collector causes an immediate non-success response rather
    than re-sending the Authorization header to an arbitrary third party.

    System CA bundles, mTLS cert env vars, and proxy settings inherited from
    the base class continue to work without additional configuration.
    Timeouts are capped at 7 seconds to prevent unbounded hangs.
    """

    def request(self, method, url, **kwargs):  # type: ignore[override]
        kwargs["allow_redirects"] = False  # always – never follow redirects
        t = kwargs.get("timeout")
        if t is None and "timeout" not in kwargs:
            kwargs["timeout"] = _DEFAULT_TIMEOUT_S
        else:
            kwargs["timeout"] = _cap_timeout(t)
        return super().request(method, url, **kwargs)


def load_rask_ingest_token() -> str:
    """Load nonempty startup token from RASK_INGEST_TOKEN_FILE (D02 contract).

    The token must be a non-empty RFC 7235 token68 value
    ([A-Za-z0-9._~+/-]+ followed by optional ``=`` padding).  The credential
    is never written to logs or error messages.

    Raises:
        RuntimeError: when the env var is unset/unreadable, the file is
            missing, unreadable, contains non-UTF-8 bytes, or the value fails
            the token68 pattern check.
    """
    token_file = os.getenv("RASK_INGEST_TOKEN_FILE", "/run/secrets/rask_ingest_token")
    try:
        raw = Path(token_file).read_text(encoding="utf-8")
    except OSError as exc:
        raise RuntimeError(
            f"RASK_INGEST_TOKEN_FILE ({token_file}) could not be read: {exc}"
        ) from exc
    except UnicodeDecodeError:
        raise RuntimeError(
            f"RASK_INGEST_TOKEN_FILE ({token_file}) contains invalid UTF-8"
        ) from None

    token = raw.rstrip("\r\n")
    if not token or not _TOKEN68_RE.match(token):
        raise RuntimeError(f"RASK_INGEST_TOKEN_FILE ({token_file}) contains an invalid token")
    return token


class OTelConfig:
    """OpenTelemetry configuration from environment variables."""

    def __init__(self):
        self.service_name = os.getenv("OTEL_SERVICE_NAME", "recap-subworker")
        self.service_version = os.getenv("SERVICE_VERSION", "0.1.0")
        self.environment = os.getenv("DEPLOYMENT_ENV", "development")
        self.otlp_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
        self.enabled = os.getenv("OTEL_ENABLED", "true").lower() == "true"


def init_otel_provider(config: OTelConfig | None = None) -> Callable[[], None]:
    """Initialize OpenTelemetry providers for tracing and logging.

    When ``config.enabled`` is ``False`` returns a no-op shutdown callable
    without touching the filesystem (no token file required).  When enabled,
    ``RASK_INGEST_TOKEN_FILE`` must exist and contain a valid token68 value;
    startup fails with ``RuntimeError`` otherwise (fail-closed).

    Args:
        config: Optional OTel configuration. If None, reads from environment.

    Returns:
        A shutdown function to clean up both the tracer and logger providers.
    """
    if config is None:
        config = OTelConfig()

    if not config.enabled:
        return lambda: None

    # D02: load Bearer token from file – raises RuntimeError on any problem.
    rask_token = load_rask_ingest_token()
    otlp_headers = {"Authorization": f"Bearer {rask_token}"}

    resource = Resource.create(
        {
            "service.name": config.service_name,
            "service.version": config.service_version,
            "deployment.environment": config.environment,
        }
    )

    # Distinct no-redirect sessions per exporter to prevent header contamination (e.g. Content-Encoding)
    trace_session = _NoRedirectSession()

    # Tracer provider
    tracer_provider = TracerProvider(resource=resource)
    trace_exporter = OTLPSpanExporter(
        endpoint=f"{config.otlp_endpoint}/v1/traces",
        headers=otlp_headers,
        session=trace_session,
    )
    tracer_provider.add_span_processor(BatchSpanProcessor(trace_exporter))
    trace.set_tracer_provider(tracer_provider)

    # Logger provider
    log_session = _NoRedirectSession()
    logger_provider = LoggerProvider(resource=resource)
    log_exporter = OTLPLogExporter(
        endpoint=f"{config.otlp_endpoint}/v1/logs",
        headers=otlp_headers,
        session=log_session,
    )
    logger_provider.add_log_record_processor(BatchLogRecordProcessor(log_exporter))
    set_logger_provider(logger_provider)

    def shutdown() -> None:
        """Flush and shut down both OTel providers gracefully."""
        tracer_provider.force_flush()
        logger_provider.force_flush()
        tracer_provider.shutdown()
        logger_provider.shutdown()
        trace_session.close()
        log_session.close()

    return shutdown
