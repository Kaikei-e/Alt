"""Structured logging configuration."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import structlog

from acolyte.infra.otel import make_otel_log_processor

if TYPE_CHECKING:
    from opentelemetry.sdk._logs import LoggerProvider


def configure_logging(
    *,
    log_level: str = "info",
    logger_provider: LoggerProvider | None = None,
) -> None:
    """Configure structlog with JSON output and optional OpenTelemetry log bridge."""
    level = getattr(logging, log_level.upper(), logging.INFO)

    processors: list[structlog.typing.Processor] = [
        structlog.contextvars.merge_contextvars,
        structlog.processors.add_log_level,
        structlog.processors.StackInfoRenderer(),
        structlog.dev.set_exc_info,
        structlog.processors.TimeStamper(fmt="iso"),
    ]
    if logger_provider is not None:
        processors.append(make_otel_log_processor(logger_provider))
    processors.append(structlog.processors.JSONRenderer())

    # Mutate the active processor list in-place when present so any bound
    # loggers already cached with cache_logger_on_first_use=True (and
    # structlog.testing.capture_logs) keep sharing the same list reference.
    existing_processors = structlog.get_config().get("processors")
    if isinstance(existing_processors, list):
        existing_processors.clear()
        existing_processors.extend(processors)
        processors = existing_processors

    structlog.configure(
        processors=processors,
        wrapper_class=structlog.make_filtering_bound_logger(level),
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )
