import pytest
import structlog
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

def test_tag_generator_service_initialization_logs_structlog_format(mock_tag_generator_service, caplog):
    with caplog.at_level(structlog.INFO):
        # Re-initialize service to capture init logs
        config = TagGeneratorConfig(
            processing_interval=1,
            error_retry_interval=1,
            batch_limit=1,
            max_connection_retries=1,
            connection_retry_delay=0.1
        )
        TagGeneratorService(config)

        assert len(caplog.records) >= 2
        init_log = caplog.records[0]
        config_log = caplog.records[1]

        # Assert that logs are structured (checking for key fields)
        assert hasattr(init_log, 'event')
        assert hasattr(init_log, 'level')
        assert hasattr(init_log, 'service')
        assert init_log.service == "tag-generator-test"
        assert init_log.event == "Tag Generator Service initialized"
        assert init_log.level == "info"

        assert hasattr(config_log, 'event')
        assert hasattr(config_log, 'level')
        assert hasattr(config_log, 'service')
        assert config_log.service == "tag-generator-test"
        assert config_log.event == f"Configuration: {config}"
        assert config_log.level == "info"

def test_tag_generator_service_error_logging_structlog_format(mock_tag_generator_service, caplog):
    with caplog.at_level(structlog.ERROR):
        # Simulate an error during database connection
        mock_tag_generator_service._create_direct_connection.side_effect = Exception("Test DB Error")
        mock_tag_generator_service._get_database_dsn.return_value = "mock_dsn" # Ensure DSN is available

        with pytest.raises(Exception, match="Test DB Error"):
            mock_tag_generator_service._create_direct_connection()

        assert len(caplog.records) >= 1
        error_log = caplog.records[0]

        assert hasattr(error_log, 'event')
        assert hasattr(error_log, 'level')
        assert hasattr(error_log, 'service')
        assert error_log.service == "tag-generator-test"
        assert error_log.event.startswith("Database connection failed")
        assert error_log.level == "error"
        assert hasattr(error_log, 'exc_info') # Check for exception info
