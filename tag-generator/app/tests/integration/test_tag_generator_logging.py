import logging
from unittest.mock import MagicMock, patch

import pytest
import structlog

from main import TagGeneratorConfig, TagGeneratorService


@pytest.fixture(autouse=True)
def setup_test_logging(monkeypatch):
    """Override conftest.py's logging configuration to use stdlib logger for caplog."""
    # Set service name for tests via environment variable
    monkeypatch.setenv("SERVICE_NAME", "tag-generator-test")

    # Configure structlog to use standard logging (allows caplog to capture)
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
            structlog.stdlib.render_to_log_kwargs,  # Integrates with standard logging
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=False,
    )

    # Bind service name
    structlog.contextvars.bind_contextvars(service="tag-generator-test")

    # Configure root logger for caplog
    root_logger = logging.getLogger()
    root_logger.setLevel(logging.DEBUG)  # Set to DEBUG to capture all levels
    root_logger.handlers = []  # Remove all handlers so caplog can add its own

    # Re-initialize module-level loggers to use new configuration
    import tag_generator.service

    tag_generator.service.logger = structlog.get_logger(tag_generator.service.__name__)


@pytest.fixture
def mock_tag_generator_service():
    config = TagGeneratorConfig(
        processing_interval=1,
        error_retry_interval=1,
        batch_limit=1,
        max_connection_retries=1,
        connection_retry_delay=0.1,
    )
    service = TagGeneratorService(config)
    # Mock database connection and related methods
    service._get_database_connection = MagicMock()
    service._get_database_connection.return_value.__enter__.return_value = MagicMock()  # Mock the context manager
    service._get_database_dsn = MagicMock(return_value="mock_dsn")
    service.article_fetcher = MagicMock()
    service.tag_extractor = MagicMock()
    service.tag_inserter = MagicMock()
    return service


def test_tag_generator_service_initialization_logs(mock_tag_generator_service, caplog):
    """Test that TagGeneratorService logs initialization messages properly."""
    with caplog.at_level(logging.INFO):
        caplog.clear()

        # Re-initialize service to capture init logs
        config = TagGeneratorConfig(
            processing_interval=1,
            error_retry_interval=1,
            batch_limit=1,
            max_connection_retries=1,
            connection_retry_delay=0.1,
        )
        TagGeneratorService(config)

        # Check that initialization logs are present
        log_messages = [record.getMessage() for record in caplog.records]

        # Look for initialization message
        init_found = any("Tag Generator Service initialized" in msg for msg in log_messages)
        config_found = any("Configuration:" in msg for msg in log_messages)

        assert init_found, f"Should log service initialization. Log messages: {log_messages}"
        assert config_found, f"Should log configuration. Log messages: {log_messages}"

        # Verify that service context appears in log records (check extra attributes)
        for record in caplog.records:
            if "Tag Generator Service initialized" in record.getMessage():
                # Check that service name appears in extra attributes or message
                service_in_message = "tag-generator-test" in record.getMessage()
                service_in_extra = hasattr(record, "service") and record.service == "tag-generator-test"
                assert service_in_message or service_in_extra, (
                    f"Service name should appear in log. Message: {record.getMessage()}, "
                    f"Extra: {getattr(record, '__dict__', {})}"
                )


def test_tag_generator_service_error_logging(mock_tag_generator_service, caplog):
    """Test that TagGeneratorService logs errors properly."""
    with caplog.at_level(logging.ERROR):
        caplog.clear()

        # Test error logging by calling a method that logs errors
        # Use the actual _create_direct_connection method but with broken DSN
        with patch.object(
            mock_tag_generator_service,
            "_get_database_dsn",
            return_value="invalid://dsn",
        ):
            try:
                mock_tag_generator_service._create_direct_connection()
            except Exception:
                pass  # Expected to fail - testing error logging behavior

        # Check if any error logs were captured
        error_records = [record for record in caplog.records if record.levelno >= logging.ERROR]

        assert len(error_records) > 0, (
            f"Should capture error logs. All records: {[r.getMessage() for r in caplog.records]}"
        )

        error_log = error_records[0]
        error_message = error_log.getMessage().lower()

        # Verify error content
        assert (
            "connection failed" in error_message
            or "failed to connect" in error_message
            or "database connection failed" in error_message
        ), f"Error message should mention connection failure: {error_message}"

        # Verify structured logging context appears in log record (check extra attributes)
        service_in_message = "tag-generator-test" in error_log.getMessage()
        service_in_extra = hasattr(error_log, "service") and error_log.service == "tag-generator-test"
        assert service_in_message or service_in_extra, (
            f"Service name should appear in error log. Message: {error_log.getMessage()}, "
            f"Extra: {getattr(error_log, '__dict__', {})}"
        )
