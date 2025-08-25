import logging
import sys
import structlog
import json
from typing import Any, Callable, Iterable, Mapping, MutableMapping, Union


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

        # Start with the basics from the LogRecord.
        log_record = {
            "timestamp": self.formatTime(record, self.datefmt),
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


def setup_logging():
    """
    Set up structured logging using structlog, integrated with the standard
    logging library to output JSON.
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
    service_name = os.getenv('SERVICE_NAME', 'tag-generator')
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
    root_logger.setLevel(logging.INFO)

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
    for _lvl_name in ("CRITICAL", "FATAL", "ERROR", "WARN", "WARNING", "INFO", "DEBUG", "NOTSET"):
        if not hasattr(structlog, _lvl_name):
            setattr(structlog, _lvl_name, getattr(logging, _lvl_name))
