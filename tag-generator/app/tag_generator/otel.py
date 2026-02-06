"""Backward-compatible re-export from infra.otel."""

from tag_generator.infra.otel import OTelConfig, get_otel_logging_handler, init_otel_provider

__all__ = ["OTelConfig", "get_otel_logging_handler", "init_otel_provider"]
