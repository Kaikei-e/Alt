"""Logging configuration utilities."""

from __future__ import annotations

import logging
import sys
from typing import Any

import structlog
from sqlalchemy import create_engine, text

from .config import get_settings


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
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.add_log_level,
            structlog.processors.StackInfoRenderer(),
            structlog.dev.set_exc_info,
            structlog.processors.JSONRenderer(),
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=True,
    )
