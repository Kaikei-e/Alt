"""Redis Streams consumer for tag-generator service."""

import asyncio
import json
import os
from dataclasses import dataclass
from datetime import datetime
from typing import Any

import redis.asyncio as redis
import structlog

logger = structlog.get_logger(__name__)


@dataclass
class ConsumerConfig:
    """Configuration for Redis Streams consumer."""

    redis_url: str = "redis://localhost:6379"
    group_name: str = "tag-generator-group"
    consumer_name: str = "tag-generator-1"
    stream_key: str = "alt:events:articles"
    batch_size: int = 10
    block_timeout_ms: int = 5000
    claim_idle_time_ms: int = 30000
    enabled: bool = False

    @classmethod
    def from_env(cls) -> "ConsumerConfig":
        """Create config from environment variables."""
        return cls(
            redis_url=os.getenv("REDIS_STREAMS_URL", "redis://localhost:6379"),
            group_name=os.getenv("CONSUMER_GROUP", "tag-generator-group"),
            consumer_name=os.getenv("CONSUMER_NAME", f"tag-generator-{os.getpid()}"),
            stream_key=os.getenv("STREAM_KEY", "alt:events:articles"),
            batch_size=int(os.getenv("CONSUMER_BATCH_SIZE", "10")),
            block_timeout_ms=int(os.getenv("CONSUMER_BLOCK_TIMEOUT_MS", "5000")),
            claim_idle_time_ms=int(os.getenv("CONSUMER_CLAIM_IDLE_TIME_MS", "30000")),
            enabled=os.getenv("CONSUMER_ENABLED", "false").lower() == "true",
        )


@dataclass
class Event:
    """Domain event from Redis Stream."""

    message_id: str
    event_id: str
    event_type: str
    source: str
    created_at: datetime | None
    payload: dict[str, Any]
    metadata: dict[str, str]


class EventHandler:
    """Interface for event handlers."""

    async def handle_event(self, event: Event) -> None:
        """Handle a single event."""
        raise NotImplementedError


class StreamConsumer:
    """Redis Streams consumer for tag-generator."""

    def __init__(
        self,
        config: ConsumerConfig,
        handler: EventHandler,
    ) -> None:
        self.config = config
        self.handler = handler
        self.client: redis.Redis | None = None
        self._shutdown = False

    async def start(self) -> None:
        """Start the consumer."""
        if not self.config.enabled:
            logger.info("consumer_disabled", message="Consumer is disabled, not starting")
            return

        # decode_responses=True to get string keys instead of bytes
        self.client = redis.from_url(self.config.redis_url, decode_responses=True)

        # Ensure consumer group exists
        await self._ensure_consumer_group()

        logger.info(
            "consumer_started",
            stream=self.config.stream_key,
            group=self.config.group_name,
            consumer=self.config.consumer_name,
        )

        await self._consume_loop()

    async def stop(self) -> None:
        """Stop the consumer gracefully."""
        self._shutdown = True
        if self.client:
            await self.client.close()

    async def _ensure_consumer_group(self) -> None:
        """Create consumer group if it doesn't exist."""
        assert self.client is not None
        try:
            await self.client.xgroup_create(
                self.config.stream_key,
                self.config.group_name,
                id="0",
                mkstream=True,
            )
        except redis.ResponseError as e:
            if "BUSYGROUP" not in str(e):
                raise

    async def _consume_loop(self) -> None:
        """Main consume loop."""
        while not self._shutdown:
            try:
                await self._read_and_process()
            except Exception as e:
                logger.error("consume_error", error=str(e))
                await asyncio.sleep(1)  # Back off on error

    async def _read_and_process(self) -> None:
        """Read events from stream and process them."""
        assert self.client is not None

        # Read new messages using XREADGROUP
        # Note: Redis 8.4 CLAIM option can be used here for idle pending messages
        streams = await self.client.xreadgroup(
            groupname=self.config.group_name,
            consumername=self.config.consumer_name,
            streams={self.config.stream_key: ">"},
            count=self.config.batch_size,
            block=self.config.block_timeout_ms,
        )

        if not streams:
            return

        for _stream_name, messages in streams:
            for message_id, data in messages:
                event = self._parse_event(message_id, data)

                try:
                    await self.handler.handle_event(event)

                    # Acknowledge successful processing
                    await self.client.xack(
                        self.config.stream_key,
                        self.config.group_name,
                        message_id,
                    )
                except Exception as e:
                    logger.error(
                        "event_processing_failed",
                        message_id=message_id,
                        event_type=event.event_type,
                        error=str(e),
                    )
                    # Don't ACK failed messages, they'll be retried

    def _parse_event(self, message_id: str, data: dict[str, Any]) -> Event:
        """Parse Redis Stream message to Event."""
        created_at = None
        if "created_at" in data:
            try:
                created_at = datetime.fromisoformat(data["created_at"].replace("Z", "+00:00"))
            except (ValueError, AttributeError):
                pass

        payload = {}
        if "payload" in data:
            try:
                payload = json.loads(data["payload"])
            except json.JSONDecodeError:
                pass

        metadata = {}
        if "metadata" in data:
            try:
                metadata = json.loads(data["metadata"])
            except json.JSONDecodeError:
                pass

        return Event(
            message_id=message_id,
            event_id=data.get("event_id", ""),
            event_type=data.get("event_type", ""),
            source=data.get("source", ""),
            created_at=created_at,
            payload=payload,
            metadata=metadata,
        )

    async def publish_reply(self, stream_key: str, event_data: dict[str, Any]) -> str | None:
        """Publish a reply event to a stream.

        Args:
            stream_key: The Redis stream key to publish to
            event_data: Event data to publish (will be JSON-encoded for payload)

        Returns:
            Message ID if successful, None otherwise
        """
        if self.client is None:
            logger.warning("Cannot publish reply: client not initialized")
            return None

        try:
            # Build the message fields
            fields = {
                "event_id": event_data.get("event_id", ""),
                "event_type": event_data.get("event_type", "TagGenerationCompleted"),
                "source": "tag-generator",
                "created_at": datetime.now().isoformat(),
                "payload": json.dumps(event_data.get("payload", {})),
            }
            if "metadata" in event_data:
                fields["metadata"] = json.dumps(event_data["metadata"])

            # Publish to stream with maxlen to avoid unbounded growth
            message_id = await self.client.xadd(stream_key, fields, maxlen=1)
            logger.info(
                "reply_published",
                stream_key=stream_key,
                message_id=message_id,
            )
            return message_id
        except Exception as e:
            logger.error(
                "reply_publish_failed",
                stream_key=stream_key,
                error=str(e),
            )
            return None
