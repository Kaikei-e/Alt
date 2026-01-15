"""
ABOUTME: Logging configuration with ADR 98/99 business context compliance.
ABOUTME: Provides structured logging with alt.* prefixed attributes for observability.

This module implements the logging requirements defined in ADR 98 and ADR 99:
- alt.article.id: Article tracking
- alt.job.id: Job tracking (recap jobs)
- alt.ai.pipeline: AI pipeline identification
- alt.processing.stage: Processing stage tracking

Uses contextvars for thread-safe context propagation in async environments.
"""

from __future__ import annotations

import logging
import sys
from contextvars import ContextVar

import structlog
from sqlalchemy import create_engine, text
from structlog.typing import EventDict, WrappedLogger

from .config import get_settings

# Context variables for ADR 98 business context (thread-safe for async)
_alt_job_id: ContextVar[str | None] = ContextVar("alt.job.id", default=None)
_alt_article_id: ContextVar[str | None] = ContextVar("alt.article.id", default=None)
_alt_processing_stage: ContextVar[str | None] = ContextVar(
    "alt.processing.stage", default=None
)


def add_business_context(
    _logger: WrappedLogger,
    _method_name: str,
    event_dict: EventDict,
) -> EventDict:
    """
    Structlog processor that renames keys to ADR 98 format with alt.* prefix.

    Transforms:
    - job_id -> alt.job.id
    - article_id -> alt.article.id

    Also adds alt.ai.pipeline = 'recap-classification' for all logs.
    """
    # Rename existing keys to ADR 98 format
    if "job_id" in event_dict:
        event_dict["alt.job.id"] = event_dict.pop("job_id")
    if "article_id" in event_dict:
        event_dict["alt.article.id"] = event_dict.pop("article_id")
    if "processing_stage" in event_dict:
        event_dict["alt.processing.stage"] = event_dict.pop("processing_stage")

    # Always add AI pipeline identifier
    event_dict["alt.ai.pipeline"] = "recap-classification"

    return event_dict


# Context setters
def set_job_id(job_id: str | None) -> None:
    """
    Set the current job ID in the logging context.

    Args:
        job_id: Job identifier to track in logs (e.g., recap job ID)
    """
    _alt_job_id.set(job_id)


def set_article_id(article_id: str | None) -> None:
    """
    Set the current article ID in the logging context.

    Args:
        article_id: Article identifier to track in logs
    """
    _alt_article_id.set(article_id)


def set_processing_stage(stage: str | None) -> None:
    """
    Set the current processing stage in the logging context.

    Args:
        stage: Processing stage (e.g., 'clustering', 'classification', 'embedding')
    """
    _alt_processing_stage.set(stage)


# Context getters
def get_job_id() -> str | None:
    """Get the current job ID from context."""
    return _alt_job_id.get()


def get_article_id() -> str | None:
    """Get the current article ID from context."""
    return _alt_article_id.get()


def get_processing_stage() -> str | None:
    """Get the current processing stage from context."""
    return _alt_processing_stage.get()


def clear_context() -> None:
    """Clear all business context values."""
    _alt_job_id.set(None)
    _alt_article_id.set(None)
    _alt_processing_stage.set(None)


class DBLogHandler(logging.Handler):
    """Logs error records to the database."""

    def __init__(self):
        super().__init__()
        settings = get_settings()
        # Use sync engine for logging to avoid async context issues in signal handlers/shutdown
        try:
            self.engine = create_engine(settings.db_url_sync)
        except Exception:
            # Fallback if DB config is invalid, don't crash logger init
            self.engine = None
            sys.stderr.write("Failed to initialize DB engine for logging\n")

    def emit(self, record: logging.LogRecord) -> None:
        if not self.engine:
            return

        try:
            # Format message
            msg = self.format(record)

            # Extract structured data if available (structlog puts it in record.msg usually if rendered as json,
            # or record.args/kwargs. Here we rely on standard logging attributes mostly)
            # For structlog integration, we might get a JSON string in msg.

            error_type = record.levelname
            if record.exc_info:
                error_type = f"{record.levelname}: {record.exc_info[0].__name__}"
            elif hasattr(record, "error_type"):
                error_type = str(getattr(record, "error_type"))

            # If msg is a JSON string (from structlog JSONRenderer), we might want to store it as raw_line
            # and try to extract a cleaner message. But for now, simple storage.

            query = text("""
                INSERT INTO log_errors (timestamp, error_type, error_message, raw_line, service)
                VALUES (NOW(), :error_type, :message, :raw_line, 'recap-subworker')
            """)

            with self.engine.connect() as conn:
                conn.execute(
                    query,
                    {
                        "error_type": error_type[:255],
                        "message": msg,  # This might be JSON if structlog is successfully hooking stdlib
                        "raw_line": msg,
                    }
                )
                conn.commit()

        except Exception:
            self.handleError(record)


def configure_logging(level: str) -> None:
    """
    Configure structured logging with ADR 98/99 business context support.

    Args:
        level: Log level string (DEBUG, INFO, WARN, ERROR)
    """
    logging.basicConfig(level=getattr(logging, level.upper(), logging.INFO))

    # Add DB Handler for ERROR+
    try:
        db_handler = DBLogHandler()
        db_handler.setLevel(logging.ERROR)
        logging.getLogger().addHandler(db_handler)
    except Exception as e:
        sys.stderr.write(f"Failed to add DBLogHandler: {e}\n")

    structlog.configure(
        processors=[
            # Merge context variables for async-safe context propagation
            structlog.contextvars.merge_contextvars,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.add_log_level,
            structlog.processors.StackInfoRenderer(),
            structlog.dev.set_exc_info,
            # ADR 98: Add business context with alt.* prefix
            add_business_context,
            structlog.processors.JSONRenderer(),
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=True,
    )

    # Bind service name to context for all logs
    structlog.contextvars.bind_contextvars(service="recap-subworker")
