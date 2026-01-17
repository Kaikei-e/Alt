"""
Logging configuration for recap-evaluator.

Provides structured logging with ADR 98/99 business context compliance and
OpenTelemetry integration for distributed tracing.

Business context keys (ADR 98):
- alt.job.id: Recap job tracking
- alt.processing.stage: Processing stage tracking
- alt.ai.pipeline: AI pipeline identification
"""

import logging
import sys
from collections.abc import Callable
from contextvars import ContextVar

import structlog
from structlog.typing import EventDict, WrappedLogger

from recap_evaluator.config import settings
from recap_evaluator.utils.otel import OTelConfig, init_otel_provider

# Global shutdown function for OTel provider
_otel_shutdown: Callable[[], None] = lambda: None

# Context variables for ADR 98 business context (thread-safe for async)
_alt_job_id: ContextVar[str | None] = ContextVar("alt.job.id", default=None)
_alt_processing_stage: ContextVar[str | None] = ContextVar("alt.processing.stage", default=None)


def add_business_context(
    _logger: WrappedLogger,
    _method_name: str,
    event_dict: EventDict,
) -> EventDict:
    """
    Structlog processor that renames keys to ADR 98 format with alt.* prefix.

    Transforms:
    - job_id -> alt.job.id
    - processing_stage -> alt.processing.stage

    Also adds alt.ai.pipeline = 'recap-evaluation' for all logs.
    """
    # Rename existing keys to ADR 98 format
    if "job_id" in event_dict:
        event_dict["alt.job.id"] = event_dict.pop("job_id")
    if "processing_stage" in event_dict:
        event_dict["alt.processing.stage"] = event_dict.pop("processing_stage")

    # Always add AI pipeline identifier
    event_dict["alt.ai.pipeline"] = "recap-evaluation"

    return event_dict


# Context setters
def set_job_id(job_id: str | None) -> None:
    """Set the current job ID in the logging context."""
    _alt_job_id.set(job_id)


def set_processing_stage(stage: str | None) -> None:
    """Set the current processing stage in the logging context."""
    _alt_processing_stage.set(stage)


# Context getters
def get_job_id() -> str | None:
    """Get the current job ID from context."""
    return _alt_job_id.get()


def get_processing_stage() -> str | None:
    """Get the current processing stage from context."""
    return _alt_processing_stage.get()


def clear_context() -> None:
    """Clear all business context values."""
    _alt_job_id.set(None)
    _alt_processing_stage.set(None)


def configure_logging() -> None:
    """
    Configure structured logging for the application.

    Initializes OpenTelemetry provider and LoggingInstrumentor for distributed
    tracing and log correlation.
    """
    global _otel_shutdown

    # Initialize OpenTelemetry provider first
    otel_config = OTelConfig()
    _otel_shutdown = init_otel_provider(otel_config)

    # Instrument stdlib logging with OTel (set_logging_format=False preserves structlog formatting)
    if otel_config.enabled:
        try:
            from opentelemetry.instrumentation.logging import LoggingInstrumentor

            LoggingInstrumentor().instrument(set_logging_format=False)
        except Exception as e:
            sys.stderr.write(f"Failed to initialize OTel LoggingInstrumentor: {e}\n")

    # Determine processors based on format
    if settings.log_format == "json":
        renderer = structlog.processors.JSONRenderer()
    else:
        renderer = structlog.dev.ConsoleRenderer(colors=True)

    # Configure structlog
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            # ADR 98: Add business context with alt.* prefix
            add_business_context,
            structlog.stdlib.ProcessorFormatter.wrap_for_formatter,
        ],
        wrapper_class=structlog.make_filtering_bound_logger(
            getattr(logging, settings.log_level.upper())
        ),
        context_class=dict,
        logger_factory=structlog.stdlib.LoggerFactory(),
        cache_logger_on_first_use=True,
    )

    # Configure stdlib logging to use structlog
    formatter = structlog.stdlib.ProcessorFormatter(
        processors=[
            structlog.stdlib.ProcessorFormatter.remove_processors_meta,
            renderer,
        ],
    )

    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(formatter)

    root_logger = logging.getLogger()
    root_logger.handlers.clear()
    root_logger.addHandler(handler)
    root_logger.setLevel(getattr(logging, settings.log_level.upper()))

    # Reduce noise from third-party libraries
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)
    logging.getLogger("asyncio").setLevel(logging.WARNING)

    # Bind service name to context for all logs
    structlog.contextvars.bind_contextvars(service="recap-evaluator")


def shutdown_logging() -> None:
    """Shutdown OTel provider and cleanup resources."""
    global _otel_shutdown
    if _otel_shutdown:
        _otel_shutdown()
