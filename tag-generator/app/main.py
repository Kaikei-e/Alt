"""Main entry point for tag generation service."""

import asyncio
import threading

import structlog

from tag_generator.config import TagGeneratorConfig
from tag_generator.logging_config import setup_logging
from tag_generator.otel import init_otel_provider
from tag_generator.service import TagGeneratorService
from tag_generator.stream_consumer import ConsumerConfig, StreamConsumer
from tag_generator.stream_event_handler import TagGeneratorEventHandler

# Initialize OpenTelemetry first (before logging setup)
otel_shutdown = init_otel_provider()

# Configure logging with OTel integration
setup_logging(enable_otel=True)
logger = structlog.get_logger(__name__)


def run_consumer(consumer: StreamConsumer) -> None:
    """Run the consumer in a background thread."""
    try:
        asyncio.run(consumer.start())
    except Exception as e:
        logger.error("Consumer thread error", error=str(e))


def main() -> int:
    """Main entry point for the tag generation service."""
    logger.info("Hello from tag-generator!")

    consumer: StreamConsumer | None = None

    try:
        # Create and configure service
        config = TagGeneratorConfig()
        service = TagGeneratorService(config)

        # Initialize Redis Streams consumer (if enabled)
        consumer_config = ConsumerConfig.from_env()
        if consumer_config.enabled:
            logger.info(
                "initializing_redis_streams_consumer",
                stream=consumer_config.stream_key,
                group=consumer_config.group_name,
                consumer=consumer_config.consumer_name,
            )
            event_handler = TagGeneratorEventHandler(service)
            consumer = StreamConsumer(consumer_config, event_handler)

            # Set stream_consumer reference for reply publishing (ADR-168)
            event_handler.stream_consumer = consumer

            # Run consumer in background thread (since service.run_service is blocking)
            consumer_thread = threading.Thread(
                target=run_consumer,
                args=(consumer,),
                daemon=True,
                name="redis-streams-consumer",
            )
            consumer_thread.start()
            logger.info("redis_streams_consumer_started")
        else:
            logger.info("redis_streams_consumer_disabled")

        # Run service (blocking)
        service.run_service()

    except Exception as e:
        logger.error("Failed to start service", error=e)
        return 1
    finally:
        # Stop consumer if running
        if consumer is not None:
            asyncio.run(consumer.stop())

        # Shutdown OTel providers
        otel_shutdown()

    return 0


if __name__ == "__main__":
    exit(main())
