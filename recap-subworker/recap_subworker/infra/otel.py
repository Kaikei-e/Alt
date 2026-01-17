"""
ABOUTME: OpenTelemetry provider for recap-subworker service.
ABOUTME: Handles trace and log export to OTLP endpoint with ADR 98 compliance.
"""

import os
from collections.abc import Callable

from opentelemetry import trace
from opentelemetry._logs import set_logger_provider
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk._logs import LoggerProvider
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor


class OTelConfig:
    """OpenTelemetry configuration from environment variables."""

    def __init__(self):
        self.service_name = os.getenv("OTEL_SERVICE_NAME", "recap-subworker")
        self.service_version = os.getenv("SERVICE_VERSION", "0.1.0")
        self.environment = os.getenv("DEPLOYMENT_ENV", "development")
        self.otlp_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
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

    def shutdown():
        """Shutdown OTel providers gracefully."""
        tracer_provider.shutdown()
        logger_provider.shutdown()

    return shutdown
