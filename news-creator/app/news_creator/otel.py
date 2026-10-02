"""OpenTelemetry provider for news-creator service."""

import logging
import os
import re
from pathlib import Path
from typing import Callable

import requests as _requests
from opentelemetry import metrics, trace
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.exporter.prometheus import PrometheusMetricReader
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.logging import LoggingInstrumentor
from opentelemetry.sdk._logs import LoggerProvider, LoggingHandler
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.view import (
    ExplicitBucketHistogramAggregation,
    View,
)
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry._logs import set_logger_provider

_DISPATCH_DURATION_BUCKETS = [0.5, 1, 2, 5, 10, 20, 30, 60, 120, 180, 300]

logger = logging.getLogger(__name__)

_RFC6750_B64TOKEN = re.compile(r"^[A-Za-z0-9\-._~+/]+=*$")

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
    """requests.Session subclass that never follows HTTP redirects.

    Per D02: OTLP exporters must not forward Authorization headers to a
    redirect destination. ``allow_redirects`` is forced ``False`` on every
    outgoing request so a 307/308 from the collector is treated as a failure
    rather than silently re-posting credentials to an arbitrary origin.

    System CA bundles, mTLS cert env vars, and proxy settings inherited from
    the base class continue to work without additional configuration.
    Timeouts are capped at 7 seconds to prevent unbounded hangs.
    """

    def request(self, method, url, **kwargs):  # type: ignore[override]
        kwargs["allow_redirects"] = False
        t = kwargs.get("timeout")
        if t is None and "timeout" not in kwargs:
            kwargs["timeout"] = _DEFAULT_TIMEOUT_S
        else:
            kwargs["timeout"] = _cap_timeout(t)
        return super().request(method, url, **kwargs)


def load_rask_ingest_token() -> str:
    """Load nonempty startup token from RASK_INGEST_TOKEN_FILE (D02 contract).

    Never logs the credential. Raises RuntimeError on missing or empty file.
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
            f"RASK_INGEST_TOKEN_FILE ({token_file}) contains invalid characters"
        ) from None

    token = raw.strip()
    if not token or not _RFC6750_B64TOKEN.match(token):
        raise RuntimeError(f"RASK_INGEST_TOKEN_FILE ({token_file}) contains invalid characters")
    return token


class OTelConfig:
    """OpenTelemetry configuration from environment variables."""

    def __init__(self):
        self.service_name = os.getenv("OTEL_SERVICE_NAME", "news-creator")
        self.service_version = os.getenv("SERVICE_VERSION", "2.0.0")
        self.environment = os.getenv("DEPLOYMENT_ENV", "development")
        self.otlp_endpoint = os.getenv(
            "OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"
        )
        self.enabled = os.getenv("OTEL_ENABLED", "true").lower() == "true"


def init_otel_provider(config: OTelConfig | None = None) -> Callable[[], None]:
    """
    Initialize OpenTelemetry providers for tracing and logging.

    Args:
        config: Optional OTel configuration. If None, reads from environment.

    Returns:
        A shutdown function to clean up providers.
    """
    if config is None:
        config = OTelConfig()

    if not config.enabled:
        logger.info("OpenTelemetry disabled")
        return lambda: None

    # Load nonempty startup token only when telemetry enabled (D02 contract)
    rask_token = load_rask_ingest_token()
    otlp_headers = {"Authorization": f"Bearer {rask_token}"}

    # Create resource with service information
    resource = Resource.create(
        {
            "service.name": config.service_name,
            "service.version": config.service_version,
            "deployment.environment": config.environment,
        }
    )

    # Distinct no-redirect sessions per exporter to prevent header contamination (e.g. Content-Encoding)
    trace_session = _NoRedirectSession()

    # Initialize Tracer Provider
    tracer_provider = TracerProvider(resource=resource)
    trace_exporter = OTLPSpanExporter(
        endpoint=f"{config.otlp_endpoint}/v1/traces",
        headers=otlp_headers,
        session=trace_session,
    )
    tracer_provider.add_span_processor(BatchSpanProcessor(trace_exporter))
    trace.set_tracer_provider(tracer_provider)

    # Initialize Meter Provider with Prometheus scrape endpoint.
    # The PrometheusMetricReader registers instruments with prometheus_client's
    # global REGISTRY, which is served at /metrics via make_asgi_app in main.
    metric_reader = PrometheusMetricReader()
    dispatch_duration_view = View(
        instrument_name="newscreator.distributed_be.request.duration",
        aggregation=ExplicitBucketHistogramAggregation(_DISPATCH_DURATION_BUCKETS),
    )
    meter_provider = MeterProvider(
        resource=resource,
        metric_readers=[metric_reader],
        views=[dispatch_duration_view],
    )
    metrics.set_meter_provider(meter_provider)

    # Initialize Logger Provider
    log_session = _NoRedirectSession()
    logger_provider = LoggerProvider(resource=resource)
    log_exporter = OTLPLogExporter(
        endpoint=f"{config.otlp_endpoint}/v1/logs",
        headers=otlp_headers,
        session=log_session,
    )
    logger_provider.add_log_record_processor(BatchLogRecordProcessor(log_exporter))
    set_logger_provider(logger_provider)

    # Instrument logging to add trace context

    LoggingInstrumentor().instrument(
        set_logging_format=False
    )  # pyrefly: ignore[missing-attribute]

    # Add OTel handler to root logger
    otel_handler = LoggingHandler(logger_provider=logger_provider)
    logging.getLogger().addHandler(otel_handler)

    logger.info(
        "OpenTelemetry initialized",
        extra={
            "service_name": config.service_name,
            "otlp_endpoint": config.otlp_endpoint,
        },
    )

    def shutdown():
        """Shutdown OTel providers gracefully."""
        logging.getLogger().removeHandler(otel_handler)
        tracer_provider.shutdown()
        meter_provider.shutdown()
        logger_provider.shutdown()
        trace_session.close()
        log_session.close()

    return shutdown


def get_otel_logging_handler() -> LoggingHandler | None:
    """
    Get an OTel logging handler for integration with standard logging.

    This function should be called AFTER init_otel_provider() and AFTER
    clearing any existing handlers on the root logger.

    Returns:
        LoggingHandler if OTel is enabled, None otherwise.
    """
    config = OTelConfig()
    if not config.enabled:
        return None

    from opentelemetry._logs import get_logger_provider

    logger_provider = get_logger_provider()
    return LoggingHandler(logger_provider=logger_provider)


def instrument_fastapi(app):
    """
    Instrument a FastAPI application with OpenTelemetry.

    Args:
        app: FastAPI application instance.
    """
    config = OTelConfig()
    if config.enabled:
        FastAPIInstrumentor.instrument_app(app)
        logger.info("FastAPI instrumented with OpenTelemetry")
