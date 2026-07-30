"""Redis Streams consumer for tag-generator service."""

import asyncio
import json
import os
import time
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any, cast

import redis.asyncio as redis
import structlog
from pydantic import Field, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

logger = structlog.get_logger(__name__)

# Redis rejects writes for reasons that clear on their own: the instance is at
# maxmemory (OOM), still loading its dataset (LOADING), has lost its primary
# (MASTERDOWN), or is a read-only replica (READONLY). Creating the consumer
# group was the only step on the startup path with no retry around it, so a
# transient rejection killed the consumer thread and latched /health at 503.
_TRANSIENT_RESPONSE_ERRORS = ("OOM", "LOADING", "MASTERDOWN", "READONLY")

# Enough to ride out a brief blip. A longer outage is the supervisor's job:
# giving up here lets it restart us with backoff instead of retrying forever
# inside a startup step that nothing is watching.
_GROUP_CREATE_MAX_ATTEMPTS = 5
_GROUP_CREATE_BACKOFF_BASE_SECONDS = 0.5
_GROUP_CREATE_BACKOFF_CAP_SECONDS = 5.0


def is_group_already_exists(error: redis.ResponseError) -> bool:
    """Whether `XGROUP CREATE` failed only because the group is already there."""
    return "BUSYGROUP" in str(error)


def is_transient_response_error(error: redis.ResponseError) -> bool:
    """Whether a Redis error is worth retrying rather than treating as fatal.

    `BUSYGROUP` is deliberately excluded: it means the write succeeded earlier,
    which callers handle as success rather than as something to retry.
    """
    message = str(error).upper()
    if "BUSYGROUP" in message:
        return False
    return any(token in message for token in _TRANSIENT_RESPONSE_ERRORS)


def supervise(
    name: str,
    run: Callable[[], None],
    *,
    on_health: Callable[[str, bool], None],
    sleep: Callable[[float], None] = time.sleep,
    max_attempts: int | None = None,
    backoff_base_seconds: float = 1.0,
    backoff_cap_seconds: float = 60.0,
) -> None:
    """Run `run` until it returns cleanly, restarting it with backoff if it raises.

    A clean return means the consumer was asked to stop, so supervision ends. An
    exception means it died, so it is restarted — indefinitely by default,
    because the condition that kills it (Redis unwritable, connection lost) can
    outlast any fixed budget, and a consumer that gives up is indistinguishable
    from one that was never wired.

    `on_health` is called with the current state on every transition so the
    readiness probe can report the outage *while* it is happening, and clear
    once the consumer is back. `max_attempts` and `sleep` exist so tests do not
    have to wait out real backoff.
    """
    attempt = 0
    while max_attempts is None or attempt < max_attempts:
        attempt += 1
        on_health(name, True)
        try:
            run()
        except Exception as error:  # noqa: BLE001 - a consumer must survive anything its handler raises
            on_health(name, False)
            # No backoff before giving up — that would only delay the log that
            # tells an operator supervision has stopped.
            if max_attempts is not None and attempt >= max_attempts:
                break
            delay = min(backoff_base_seconds * 2 ** (attempt - 1), backoff_cap_seconds)
            logger.error(
                "consumer_supervisor_restarting",
                consumer=name,
                attempt=attempt,
                delay_seconds=delay,
                error=str(error),
            )
            sleep(delay)
        else:
            logger.info("consumer_supervisor_stopped", consumer=name, attempt=attempt)
            return

    # `message` is a reserved LogRecord attribute: passing it as a bound value
    # raises once structlog is routed through stdlib logging.
    logger.error(
        "consumer_supervisor_exhausted",
        consumer=name,
        attempts=attempt,
        detail="consumer will not be restarted again; readiness stays failed",
    )


class ConsumerConfig(BaseSettings):
    """Configuration for Redis Streams consumer (pydantic-settings)."""

    model_config = SettingsConfigDict(extra="ignore", populate_by_name=True)

    redis_url: str = Field(default="redis://localhost:6379", validation_alias="REDIS_STREAMS_URL")
    group_name: str = Field(default="tag-generator-group", validation_alias="CONSUMER_GROUP")
    consumer_name: str = Field(
        default_factory=lambda: f"tag-generator-{os.getpid()}",
        validation_alias="CONSUMER_NAME",
    )
    stream_key: str = Field(default="alt:events:articles", validation_alias="STREAM_KEY")
    batch_size: int = Field(default=10, validation_alias="CONSUMER_BATCH_SIZE")
    block_timeout_ms: int = Field(default=5000, validation_alias="CONSUMER_BLOCK_TIMEOUT_MS")
    claim_idle_time_ms: int = Field(default=30000, validation_alias="CONSUMER_CLAIM_IDLE_TIME_MS")
    enabled: bool = Field(default=False, validation_alias="CONSUMER_ENABLED")
    max_delivery_count: int = Field(default=5, validation_alias="CONSUMER_MAX_DELIVERY_COUNT")
    reclaim_interval_seconds: float = Field(default=30.0, validation_alias="CONSUMER_RECLAIM_INTERVAL_SECONDS")

    @field_validator("enabled", mode="before")
    @classmethod
    def _parse_enabled(cls, v: Any) -> bool:
        if isinstance(v, bool):
            return v
        if v is None:
            return False
        return str(v).lower() in {"1", "true", "yes", "on"}

    @classmethod
    def from_env(cls) -> ConsumerConfig:
        """Create config from environment variables."""
        return cls()

    @classmethod
    def tags_stream_from_env(cls) -> ConsumerConfig:
        """Create config for the dedicated tags stream (on-the-fly tag generation).

        Uses low batch_size and short block_timeout for low-latency response.
        """
        return cls(
            redis_url=os.getenv("REDIS_STREAMS_URL", "redis://localhost:6379"),
            group_name=os.getenv("TAGS_CONSUMER_GROUP", "tag-generator-tags-group"),
            consumer_name=os.getenv("TAGS_CONSUMER_NAME", f"tag-generator-tags-{os.getpid()}"),
            stream_key="alt:events:tags",
            batch_size=1,
            block_timeout_ms=1000,
            claim_idle_time_ms=os.getenv("CONSUMER_CLAIM_IDLE_TIME_MS", "30000"),
            enabled=os.getenv("CONSUMER_ENABLED", "false"),
            max_delivery_count=os.getenv("CONSUMER_MAX_DELIVERY_COUNT", "5"),
            reclaim_interval_seconds=os.getenv("CONSUMER_RECLAIM_INTERVAL_SECONDS", "30"),
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

        try:
            async with asyncio.TaskGroup() as tg:
                tg.create_task(self._consume_loop())
                tg.create_task(self._reclaim_loop())
        finally:
            # Close the client on the same event loop that created it. stop()
            # only flips self._shutdown from whatever thread calls it; the
            # actual teardown always happens here, inside this consumer's own
            # asyncio.run() loop.
            await self.client.close()

    def stop(self) -> None:
        """Signal the consumer to stop.

        Thread-safe: this only flips a flag that the consumer's own loops
        (running in their own asyncio.run() loop, possibly on another thread)
        observe and react to. It must never touch self.client directly --
        closing an asyncio Redis client from a different event loop than the
        one that created it is undefined behaviour.
        """
        self._shutdown = True

    @property
    def is_stopped(self) -> bool:
        """Whether stop() has been called (or the consumer was never enabled)."""
        return self._shutdown or not self.config.enabled

    async def _ensure_consumer_group(self) -> None:
        """Create the consumer group, waiting out a temporarily unwritable Redis.

        A permanent error still fails on the first attempt — retrying a real bug
        only turns a loud failure into a slow one.
        """
        if self.client is None:
            raise RuntimeError("Redis client is not initialized")

        for attempt in range(1, _GROUP_CREATE_MAX_ATTEMPTS + 1):
            try:
                await self.client.xgroup_create(
                    self.config.stream_key,
                    self.config.group_name,
                    id="0",
                    mkstream=True,
                )
                return
            except redis.ResponseError as e:
                if is_group_already_exists(e):
                    return
                if not is_transient_response_error(e) or attempt == _GROUP_CREATE_MAX_ATTEMPTS:
                    raise
                delay = min(
                    _GROUP_CREATE_BACKOFF_BASE_SECONDS * 2 ** (attempt - 1),
                    _GROUP_CREATE_BACKOFF_CAP_SECONDS,
                )
                logger.warning(
                    "consumer_group_create_retry",
                    stream=self.config.stream_key,
                    group=self.config.group_name,
                    attempt=attempt,
                    max_attempts=_GROUP_CREATE_MAX_ATTEMPTS,
                    delay_seconds=delay,
                    error=str(e),
                )
                await asyncio.sleep(delay)

    async def _consume_loop(self) -> None:
        """Main consume loop."""
        while not self._shutdown:
            try:
                await self._read_and_process()
            except Exception as e:
                logger.error("consume_error", error=str(e))
                await asyncio.sleep(1)  # Back off on error

    async def _reclaim_loop(self) -> None:
        """Periodically reclaim pending entries idle longer than claim_idle_time_ms.

        Without this, messages left in the PEL by a crashed consumer (read via
        XREADGROUP but never XACK'd) are never redelivered to anyone.
        """
        while not self._shutdown:
            try:
                await self._reclaim_idle_messages()
            except Exception as e:
                logger.error("reclaim_error", error=str(e))
            await asyncio.sleep(self.config.reclaim_interval_seconds)

    async def _reclaim_idle_messages(self) -> None:
        """Walk the PEL via XAUTOCLAIM, reprocessing or dead-lettering entries."""
        if self.client is None:
            raise RuntimeError("Redis client is not initialized")
        cursor = "0-0"
        while not self._shutdown:
            next_cursor, messages, _deleted = await self.client.xautoclaim(
                name=self.config.stream_key,
                groupname=self.config.group_name,
                consumername=self.config.consumer_name,
                min_idle_time=self.config.claim_idle_time_ms,
                start_id=cursor,
                count=self.config.batch_size,
            )
            if messages:
                await self._process_claimed_messages(messages)
            if next_cursor == "0-0":
                break
            cursor = next_cursor

    async def _process_claimed_messages(self, messages: list[Any]) -> None:
        """Reprocess reclaimed messages, dead-lettering ones over the retry limit."""
        if self.client is None:
            raise RuntimeError("Redis client is not initialized")
        for message_id, data in messages:
            delivery_count = await self._get_delivery_count(message_id)
            if delivery_count > self.config.max_delivery_count:
                await self._move_to_dlq(message_id, data, delivery_count)
                continue

            event = self._parse_event(message_id, data)
            try:
                await self.handler.handle_event(event)
                await self.client.xack(self.config.stream_key, self.config.group_name, message_id)
            except Exception as e:
                logger.error(
                    "reclaimed_event_processing_failed",
                    message_id=message_id,
                    event_type=event.event_type,
                    delivery_count=delivery_count,
                    error=str(e),
                )
                # Don't ACK -- stays in the PEL for the next reclaim pass.

    async def _get_delivery_count(self, message_id: str) -> int:
        """Look up how many times this message has been delivered."""
        if self.client is None:
            raise RuntimeError("Redis client is not initialized")
        entries = await self.client.xpending_range(
            self.config.stream_key,
            self.config.group_name,
            min=message_id,
            max=message_id,
            count=1,
        )
        if entries:
            return int(entries[0]["times_delivered"])
        return 0

    async def _move_to_dlq(self, message_id: str, data: dict[str, Any], delivery_count: int) -> None:
        """Dead-letter a message that exceeded max_delivery_count and ACK it
        so it stops occupying the PEL."""
        if self.client is None:
            raise RuntimeError("Redis client is not initialized")
        dlq_stream = f"{self.config.stream_key}:dlq"
        dlq_fields = {**data, "original_message_id": message_id, "delivery_count": str(delivery_count)}
        await self.client.xadd(dlq_stream, cast(dict[Any, Any], dlq_fields))
        await self.client.xack(self.config.stream_key, self.config.group_name, message_id)
        logger.error(
            "message_moved_to_dlq",
            message_id=message_id,
            delivery_count=delivery_count,
            dlq_stream=dlq_stream,
        )

    async def _read_and_process(self) -> None:
        """Read events from stream and process them."""
        if self.client is None:
            raise RuntimeError("Redis client is not initialized")

        # New messages only (">"). Redelivery of unacked pending entries is
        # handled by _reclaim_loop via XAUTOCLAIM, not here.
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
            # pyrefly: ignore [not-iterable]
            for message_id, data in messages:
                # pyrefly: ignore [bad-argument-type]
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
            except ValueError, AttributeError:
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
            fields: dict[str, str] = {
                "event_id": event_data.get("event_id", ""),
                "event_type": event_data.get("event_type", "TagGenerationCompleted"),
                "source": "tag-generator",
                "created_at": datetime.now(UTC).isoformat(),
                "payload": json.dumps(event_data.get("payload", {})),
            }
            if "metadata" in event_data:
                fields["metadata"] = json.dumps(event_data["metadata"])

            # Publish to stream with maxlen to avoid unbounded growth while
            # keeping enough backlog that concurrent in-flight replies aren't
            # trimmed before a consumer reads them (maxlen=1 caused replies to
            # be evicted by the very next publish under concurrent requests).
            # Cast to satisfy Pyrefly's generic type inference for redis xadd
            message_id = await self.client.xadd(stream_key, cast(dict[Any, Any], fields), maxlen=1000, approximate=True)
            logger.info(
                "reply_published",
                stream_key=stream_key,
                message_id=message_id,
            )
            # pyrefly: ignore [bad-return]
            return message_id
        except Exception as e:
            logger.error(
                "reply_publish_failed",
                stream_key=stream_key,
                error=str(e),
            )
            return None
