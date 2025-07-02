import pytest
import structlog
import logging
from unittest.mock import MagicMock, patch
from tag_generator.logging_config import setup_logging
from main import TagGeneratorService, TagGeneratorConfig

@pytest.fixture(autouse=True)
def setup_test_logging():
    # Ensure logging is set up for tests
    setup_logging()
    # Reset structlog processors for testing
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.stdlib.add_log_level,
            structlog.stdlib.add_logger_name,
            structlog.stdlib.PositionalArgumentsFormatter(),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.processors.UnicodeDecoder(),
            structlog.dev.ConsoleRenderer() # Use console renderer for easier debugging in tests
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=True,
    )
    # Bind service name for consistency
    structlog.contextvars.bind_contextvars(service="tag-generator-test")

@pytest.fixture
def mock_tag_generator_service():
    config = TagGeneratorConfig(
        processing_interval=1,
        error_retry_interval=1,
        batch_limit=1,
        max_connection_retries=1,
        connection_retry_delay=0.1
    )
    service = TagGeneratorService(config)
    # Mock database connection and related methods
    service._get_database_connection = MagicMock()
    service._get_database_connection.return_value.__enter__.return_value = MagicMock() # Mock the context manager
    service._get_database_dsn = MagicMock(return_value="mock_dsn")
    service.article_fetcher = MagicMock()
    service.tag_extractor = MagicMock()
    service.tag_inserter = MagicMock()
    return service

def test_tag_generator_service_initialization_logs(mock_tag_generator_service, caplog):
    """Test that TagGeneratorService logs initialization messages properly."""
    with caplog.at_level(logging.INFO):
        # Re-initialize service to capture init logs
        config = TagGeneratorConfig(
            processing_interval=1,
            error_retry_interval=1,
            batch_limit=1,
            max_connection_retries=1,
            connection_retry_delay=0.1
        )
        TagGeneratorService(config)

        # Check that initialization logs are present
        log_messages = [record.getMessage() for record in caplog.records]

        # Look for initialization message
        init_found = any("Tag Generator Service initialized" in msg for msg in log_messages)
        config_found = any("Configuration:" in msg for msg in log_messages)

        assert init_found, "Should log service initialization"
        assert config_found, "Should log configuration"

        # Verify that service context appears in log messages
        for record in caplog.records:
            if "Tag Generator Service initialized" in record.getMessage():
                # Check that service name appears in the formatted message
                assert "tag-generator-test" in record.getMessage(), \
                    "Service name should appear in log message"

def test_tag_generator_service_error_logging(mock_tag_generator_service, caplog):
    """Test that TagGeneratorService logs errors properly."""
    with caplog.at_level(logging.ERROR):
        # Test error logging by calling a method that logs errors
        # Use the actual _create_direct_connection method but with broken DSN
        with patch.object(mock_tag_generator_service, '_get_database_dsn', return_value="invalid://dsn"):
            try:
                mock_tag_generator_service._create_direct_connection()
            except Exception:
                pass  # Expected to fail

        # Check if any error logs were captured
        error_records = [record for record in caplog.records if record.levelno >= logging.ERROR]

        assert len(error_records) > 0, "Should capture error logs"

        error_log = error_records[0]
        error_message = error_log.getMessage().lower()

        # Verify error content
        assert "connection failed" in error_message or "failed to connect" in error_message, \
            f"Error message should mention connection failure: {error_message}"

        # Verify structured logging context appears in message
        assert "tag-generator-test" in error_log.getMessage(), \
            "Service name should appear in error log message"
