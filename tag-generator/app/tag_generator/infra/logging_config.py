"""
ABOUTME: Structured logging configuration with ADR 98 compliance.
ABOUTME: Provides JSON output with alt.* prefixed business context attributes.
"""

import json
import logging
import sys
from datetime import UTC, datetime

import structlog
from structlog.typing import EventDict, WrappedLogger

SENSITIVE_KEYS = frozenset({"password", "secret", "token", "dsn", "authorization", "api_key"})


def filter_sensitive_data(
    _logger: WrappedLogger,
    _method_name: str,
    event_dict: EventDict,
) -> EventDict:
    """Redact values whose keys contain sensitive substrings."""
    for key in list(event_dict.keys()):
        if any(s in key.lower() for s in SENSITIVE_KEYS):
            event_dict[key] = "***REDACTED***"
    return event_dict


def add_business_context(
    _logger: WrappedLogger,
    _method_name: str,
    event_dict: EventDict,
) -> EventDict:
    """
    Structlog processor that renames keys to ADR 98 format with alt.* prefix.

    Transforms:
    - article_id -> alt.article.id
    - feed_id -> alt.feed.id

    Also adds alt.ai.pipeline = 'tag-extraction' for all logs.
    """
    # Rename existing keys to ADR 98 format
    if "article_id" in event_dict:
        event_dict["alt.article.id"] = event_dict.pop("article_id")
    if "feed_id" in event_dict:
        event_dict["alt.feed.id"] = event_dict.pop("feed_id")

    # Always add AI pipeline identifier
    event_dict["alt.ai.pipeline"] = "tag-extraction"

    return event_dict


class JsonFormatter(logging.Formatter):
    """
    A custom formatter to render log records as JSON.
    It extracts all non-standard attributes from the LogRecord
    and includes them in the final JSON output.
    """

    def format(self, record):
        # These are the standard attributes of a LogRecord that we handle explicitly
        # or want to ignore.
        standard_attrs = {
            "args",
            "asctime",
            "created",
            "exc_info",
            "exc_text",
            "filename",
            "funcName",
            "levelname",
            "levelno",
            "lineno",
            "module",
            "msecs",
            "message",
            "msg",
            "name",
            "pathname",
            "process",
            "processName",
            "relativeCreated",
            "stack_info",
            "thread",
            "threadName",
        }

        # Get timestamp - prefer structlog's timestamp if available, otherwise format it
        timestamp = None
        if hasattr(record, "timestamp"):
            # Use getattr with type ignore since timestamp is a dynamic attribute added by structlog
            timestamp_value = getattr(record, "timestamp", None)  # type: ignore[attr-defined]  # noqa: B009
            if isinstance(timestamp_value, str):
                timestamp = timestamp_value
        elif self.datefmt == "iso":
            # Generate ISO format timestamp
            timestamp = datetime.fromtimestamp(record.created, tz=UTC).isoformat()
        else:
            timestamp = self.formatTime(record, self.datefmt)

        # Start with the basics from the LogRecord.
        log_record = {
            "timestamp": timestamp,
            "level": record.levelname.lower(),
            "logger": record.name,
        }

        # The main message is in 'event' from structlog, which gets moved to msg.
        # record.getMessage() will format it.
        log_record["msg"] = record.getMessage()

        # Add all other non-standard attributes from the record to the log.
        # This is how we get the context and kwargs from structlog.
        for key, value in record.__dict__.items():
            if key not in standard_attrs and key not in log_record:
                log_record[key] = value

        # structlog passes the original event name in the 'event' key.
        # If it exists, we'll use it as the primary message.
        if "event" in log_record:
            log_record["msg"] = log_record.pop("event")

        # Use default=str to ensure non-serialisable objects (e.g., Exception instances)
        # are rendered as their string representation instead of raising TypeError.
        return json.dumps(log_record, sort_keys=True, default=str)


def setup_logging(enable_otel: bool = True):
    """
    Set up structured logging using structlog, integrated with the standard
    logging library to output JSON. Optionally enables OpenTelemetry log export.

    Args:
        enable_otel: Whether to enable OTel log export. Default True.
    """
    import os

    # Define a type for structlog processors
    # Processor = Callable[[Any, str, Any] , Any] # Simplified type for Pyright

    # Processors that prepare the log record for the standard logger.
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.stdlib.add_log_level,
            structlog.stdlib.add_logger_name,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.stdlib.PositionalArgumentsFormatter(),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.processors.UnicodeDecoder(),
            # Security: Redact sensitive values
            filter_sensitive_data,
            # ADR 98: Add business context with alt.* prefix
            add_business_context,
            # This processor is the key for integration. It takes the event dict
            # and prepares it as keyword arguments for the standard logger.
            structlog.stdlib.render_to_log_kwargs,
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=True,
    )

    # Bind the service name to the context, so it's included in all logs.
    # Use environment variable to allow test override
    service_name = os.getenv("SERVICE_NAME", "tag-generator")
    structlog.contextvars.bind_contextvars(service=service_name)

    # Configure the standard logging handler.
    handler = logging.StreamHandler(sys.stdout)
    # Use our custom JSON formatter.
    handler.setFormatter(JsonFormatter(datefmt="iso"))

    root_logger = logging.getLogger()

    # Clear any existing handlers to avoid duplicate output.
    if root_logger.hasHandlers():
        root_logger.handlers.clear()

    root_logger.addHandler(handler)

    # Add OTel logging handler if enabled
    if enable_otel:
        otel_enabled = os.getenv("OTEL_ENABLED", "true").lower() == "true"
        if otel_enabled:
            try:
                from tag_generator.infra.otel import get_otel_logging_handler

                otel_handler = get_otel_logging_handler()
                if otel_handler is not None:
                    root_logger.addHandler(otel_handler)
            except ImportError:
                pass  # OTel dependencies not installed

    # Allow log level override via environment variable (default: INFO)
    log_level_str = os.getenv("TAG_LOG_LEVEL", "INFO").upper()
    log_level = getattr(logging, log_level_str, logging.INFO)
    root_logger.setLevel(log_level)

    # Ensure every LogRecord has an 'event' attribute for tests that rely on it.
    class _EnsureEvent(logging.Filter):
        def filter(self, record: logging.LogRecord) -> bool:  # noqa: D401
            if not hasattr(record, "event"):
                # Default to the already formatted message
                record.event = record.getMessage()
            return True

    root_logger.addFilter(_EnsureEvent())

    # Expose standard logging level constants on the structlog module so code/tests
    # can use `structlog.INFO`, `structlog.ERROR`, etc. This mirrors what the
    # stdlib `logging` module provides and maintains backward-compatibility with
    # typical logging APIs.
    for _lvl_name in (
        "CRITICAL",
        "FATAL",
        "ERROR",
        "WARN",
        "WARNING",
        "INFO",
        "DEBUG",
        "NOTSET",
    ):
        if not hasattr(structlog, _lvl_name):
            setattr(structlog, _lvl_name, getattr(logging, _lvl_name))
