"""OpenTelemetry provider for news-creator service."""

import logging
import os
from typing import Callable

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.logging import LoggingInstrumentor
from opentelemetry.sdk._logs import LoggerProvider, LoggingHandler
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry._logs import set_logger_provider

logger = logging.getLogger(__name__)


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

    # Create resource with service information
    resource = Resource.create(
        {
            "service.name": config.service_name,
            "service.version": config.service_version,
            "deployment.environment": config.environment,
        }
    )

    # Initialize Tracer Provider
    tracer_provider = TracerProvider(resource=resource)
    trace_exporter = OTLPSpanExporter(
        endpoint=f"{config.otlp_endpoint}/v1/traces",
    )
    tracer_provider.add_span_processor(BatchSpanProcessor(trace_exporter))
    trace.set_tracer_provider(tracer_provider)

    # Initialize Logger Provider
    logger_provider = LoggerProvider(resource=resource)
    log_exporter = OTLPLogExporter(
        endpoint=f"{config.otlp_endpoint}/v1/logs",
    )
    logger_provider.add_log_record_processor(BatchLogRecordProcessor(log_exporter))
    set_logger_provider(logger_provider)

    # Instrument logging to add trace context
    LoggingInstrumentor().instrument(set_logging_format=False)

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
        tracer_provider.shutdown()
        logger_provider.shutdown()

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
