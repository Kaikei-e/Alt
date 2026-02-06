"""Usecase: Handle Redis Stream events for real-time tag generation."""

from __future__ import annotations

import asyncio
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from typing import TYPE_CHECKING

import structlog

from tag_generator.domain.models import TagExtractionResult

if TYPE_CHECKING:
    from tag_generator.port.event_publisher import EventPublisherPort
    from tag_generator.usecase.extract_tags import ExtractTagsUsecase

logger = structlog.get_logger(__name__)


class HandleStreamEventUsecase:
    """Process tag generation events from Redis Streams."""

    def __init__(
        self,
        extract_usecase: ExtractTagsUsecase,
        event_publisher: EventPublisherPort | None = None,
        executor: ThreadPoolExecutor | None = None,
    ) -> None:
        self._extract = extract_usecase
        self._publisher = event_publisher
        self._executor = executor or ThreadPoolExecutor(max_workers=4)

    async def handle_tag_generation_request(
        self,
        article_id: str,
        title: str,
        content: str,
        feed_id: str,
        reply_to: str | None = None,
        correlation_id: str | None = None,
    ) -> TagExtractionResult:
        """Generate tags for an article, optionally publishing a reply.

        ML inference is offloaded to a thread pool to avoid blocking the event loop.
        """
        start_time = time.perf_counter()

        loop = asyncio.get_running_loop()
        result = await loop.run_in_executor(
            self._executor,
            self._extract.execute,
            article_id,
            title,
            content,
        )

        inference_ms = (time.perf_counter() - start_time) * 1000

        if reply_to and self._publisher:
            tags_payload = [
                {"id": str(uuid.uuid4()), "name": tag.name, "confidence": tag.confidence} for tag in result.tags
            ]
            await self._publisher.publish_reply(
                reply_to,
                {
                    "event_id": str(uuid.uuid4()),
                    "event_type": "TagGenerationCompleted",
                    "payload": {
                        "success": not result.is_empty,
                        "article_id": article_id,
                        "tags": tags_payload,
                        "inference_ms": inference_ms,
                    },
                    "metadata": {"correlation_id": correlation_id or ""},
                },
            )

        return result
