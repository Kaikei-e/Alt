"""Main entry point for tag generation service."""

import structlog

from tag_generator.config import TagGeneratorConfig
from tag_generator.logging_config import setup_logging
from tag_generator.otel import init_otel_provider
from tag_generator.service import TagGeneratorService

# Initialize OpenTelemetry first (before logging setup)
otel_shutdown = init_otel_provider()

# Configure logging with OTel integration
setup_logging(enable_otel=True)
logger = structlog.get_logger(__name__)


def main() -> int:
    """Main entry point for the tag generation service."""
    logger.info("Hello from tag-generator!")

    try:
        # Create and configure service
        config = TagGeneratorConfig()
        service = TagGeneratorService(config)

        # Run service
        service.run_service()

    except Exception as e:
        logger.error("Failed to start service", error=e)
        return 1
    finally:
        # Shutdown OTel providers
        otel_shutdown()

    return 0


if __name__ == "__main__":
    exit(main())
