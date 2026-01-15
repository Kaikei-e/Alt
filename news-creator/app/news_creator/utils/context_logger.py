"""
ABOUTME: Business context logger for ADR 98 compliance.
ABOUTME: Provides context-aware structured logging with alt.* prefixed attributes.

This module implements the logging requirements defined in ADR 98:
- alt.article.id: Article tracking
- alt.ai.pipeline: AI pipeline identification
- alt.processing.stage: Processing stage tracking

Uses contextvars for thread-safe context propagation in async environments.
"""

import logging
from contextvars import ContextVar
from typing import Any

# Context variables for business context (thread-safe for async)
_alt_article_id: ContextVar[str | None] = ContextVar("alt.article.id", default=None)
_alt_job_id: ContextVar[str | None] = ContextVar("alt.job.id", default=None)
_alt_ai_pipeline: ContextVar[str | None] = ContextVar("alt.ai.pipeline", default=None)
_alt_processing_stage: ContextVar[str | None] = ContextVar(
    "alt.processing.stage", default=None
)


class BusinessContextFilter(logging.Filter):
    """
    Logging filter that adds ADR 98 business context to log records.

    Adds the following attributes to each log record:
    - alt.article.id: Current article being processed
    - alt.ai.pipeline: AI pipeline name (e.g., 'summarization', 'tag-extraction')
    - alt.processing.stage: Current processing stage
    """

    def filter(self, record: logging.LogRecord) -> bool:
        """Add business context attributes to the log record."""
        # Add context values (None values will be excluded in JSON output)
        setattr(record, "alt.article.id", _alt_article_id.get())
        setattr(record, "alt.job.id", _alt_job_id.get())
        setattr(record, "alt.ai.pipeline", _alt_ai_pipeline.get())
        setattr(record, "alt.processing.stage", _alt_processing_stage.get())
        return True


class BusinessContextJSONFormatter(logging.Formatter):
    """
    JSON formatter that includes ADR 98 business context attributes.

    Output format follows OpenTelemetry semantic conventions with alt.* prefix.
    """

    def format(self, record: logging.LogRecord) -> str:
        """Format log record as JSON with business context."""
        import json

        # Base log fields
        log_dict: dict[str, Any] = {
            "timestamp": self.formatTime(record, self.datefmt),
            "level": record.levelname,
            "logger": record.name,
            "msg": record.getMessage(),
        }

        # Add business context if present
        article_id = getattr(record, "alt.article.id", None)
        if article_id:
            log_dict["alt.article.id"] = article_id

        job_id = getattr(record, "alt.job.id", None)
        if job_id:
            log_dict["alt.job.id"] = job_id

        ai_pipeline = getattr(record, "alt.ai.pipeline", None)
        if ai_pipeline:
            log_dict["alt.ai.pipeline"] = ai_pipeline

        processing_stage = getattr(record, "alt.processing.stage", None)
        if processing_stage:
            log_dict["alt.processing.stage"] = processing_stage

        # Add extra fields from record (e.g., from logger.info(..., extra={...}))
        if hasattr(record, "__dict__"):
            for key, value in record.__dict__.items():
                # Skip standard LogRecord attributes and already processed fields
                if key not in (
                    "name",
                    "msg",
                    "args",
                    "created",
                    "filename",
                    "funcName",
                    "levelname",
                    "levelno",
                    "lineno",
                    "module",
                    "msecs",
                    "pathname",
                    "process",
                    "processName",
                    "relativeCreated",
                    "stack_info",
                    "exc_info",
                    "exc_text",
                    "thread",
                    "threadName",
                    "message",
                    "alt.article.id",
                    "alt.job.id",
                    "alt.ai.pipeline",
                    "alt.processing.stage",
                    "taskName",
                ):
                    # Include extra fields
                    if not key.startswith("_"):
                        log_dict[key] = value

        return json.dumps(log_dict, ensure_ascii=False, default=str)


def set_article_id(article_id: str | None) -> None:
    """
    Set the current article ID in the logging context.

    Args:
        article_id: Article identifier to track in logs
    """
    _alt_article_id.set(article_id)


def set_job_id(job_id: str | None) -> None:
    """
    Set the current job ID in the logging context.

    Args:
        job_id: Job identifier to track in logs (e.g., recap job ID)
    """
    _alt_job_id.set(job_id)


def set_ai_pipeline(pipeline: str | None) -> None:
    """
    Set the AI pipeline name in the logging context.

    Args:
        pipeline: Pipeline name (e.g., 'summarization', 'recap-summary', 'query-expansion')
    """
    _alt_ai_pipeline.set(pipeline)


def set_processing_stage(stage: str | None) -> None:
    """
    Set the current processing stage in the logging context.

    Args:
        stage: Processing stage (e.g., 'validation', 'cleaning', 'generation', 'postprocessing')
    """
    _alt_processing_stage.set(stage)


def clear_context() -> None:
    """Clear all business context values."""
    _alt_article_id.set(None)
    _alt_job_id.set(None)
    _alt_ai_pipeline.set(None)
    _alt_processing_stage.set(None)


def get_article_id() -> str | None:
    """Get the current article ID from context."""
    return _alt_article_id.get()


def get_job_id() -> str | None:
    """Get the current job ID from context."""
    return _alt_job_id.get()


def get_ai_pipeline() -> str | None:
    """Get the current AI pipeline from context."""
    return _alt_ai_pipeline.get()


def get_processing_stage() -> str | None:
    """Get the current processing stage from context."""
    return _alt_processing_stage.get()
