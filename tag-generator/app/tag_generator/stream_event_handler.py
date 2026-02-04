"""Event handler for tag-generator Redis Streams consumer."""

import time
import uuid
from typing import TYPE_CHECKING

import structlog

from tag_generator.stream_consumer import Event, EventHandler, StreamConsumer

if TYPE_CHECKING:
    from tag_generator.service import TagGeneratorService

logger = structlog.get_logger(__name__)


class TagGeneratorEventHandler(EventHandler):
    """Handles events for tag-generator service."""

    EVENT_TYPE_ARTICLE_CREATED = "ArticleCreated"
    EVENT_TYPE_TAG_GENERATION_REQUESTED = "TagGenerationRequested"

    def __init__(
        self,
        service: "TagGeneratorService",
        stream_consumer: StreamConsumer | None = None,
    ) -> None:
        self.service = service
        self.stream_consumer = stream_consumer

    async def handle_event(self, event: Event) -> None:
        """Handle a single event based on its type."""
        logger.info(
            "handling_event",
            event_id=event.event_id,
            event_type=event.event_type,
            message_id=event.message_id,
        )

        if event.event_type == self.EVENT_TYPE_ARTICLE_CREATED:
            await self._handle_article_created(event)
        elif event.event_type == self.EVENT_TYPE_TAG_GENERATION_REQUESTED:
            await self._handle_tag_generation_requested(event)
        else:
            logger.debug(
                "ignoring_unknown_event",
                event_type=event.event_type,
            )

    async def _handle_article_created(self, event: Event) -> None:
        """Process an ArticleCreated event."""
        article_id = event.payload.get("article_id")
        title = event.payload.get("title", "")

        if not article_id:
            logger.warning(
                "missing_article_id",
                event_id=event.event_id,
            )
            return

        logger.info(
            "processing_article_created",
            article_id=article_id,
            title=title[:50] if title else "",
        )

        try:
            # Process article for tag generation
            # The service will fetch content from DB and generate tags
            await self._process_article(article_id)
        except Exception as e:
            logger.error(
                "tag_generation_failed",
                article_id=article_id,
                error=str(e),
            )
            raise

    async def _process_article(self, article_id: str) -> None:
        """Process a single article for tag generation.

        This delegates to the TagGeneratorService for the actual processing.
        Uses an executor to run the sync method without blocking the event loop.
        """
        import asyncio

        loop = asyncio.get_event_loop()
        await loop.run_in_executor(
            None,
            self._process_article_sync,
            article_id,
        )

    def _process_article_sync(self, article_id: str) -> None:
        """Synchronous wrapper for processing an article."""
        with self.service.database_manager.get_connection() as conn:
            # Fetch article by ID
            article = self.service.article_fetcher.fetch_article_by_id(conn, article_id)
            if article:
                success = self.service._process_single_article(conn, article)
                if success:
                    logger.info(
                        "article_processed_for_tags",
                        article_id=article_id,
                    )
                else:
                    logger.warning(
                        "article_tag_processing_failed",
                        article_id=article_id,
                    )
            else:
                logger.warning(
                    "article_not_found",
                    article_id=article_id,
                )

    async def _handle_tag_generation_requested(self, event: Event) -> None:
        """Handle a synchronous tag generation request with reply.

        This processes tag generation inline (without database fetch)
        and publishes a reply to the reply_to stream.
        """
        reply_to = event.metadata.get("reply_to")
        correlation_id = event.metadata.get("correlation_id")

        if not reply_to:
            logger.warning(
                "missing_reply_to_in_tag_generation_request",
                event_id=event.event_id,
            )
            return

        if not self.stream_consumer:
            logger.error(
                "stream_consumer_not_configured_for_reply",
                event_id=event.event_id,
            )
            return

        article_id = event.payload.get("article_id", "")
        title = event.payload.get("title", "")
        content = event.payload.get("content", "")
        feed_id = event.payload.get("feed_id", "")

        logger.info(
            "processing_tag_generation_request",
            article_id=article_id,
            correlation_id=correlation_id,
            reply_to=reply_to,
            title_preview=title[:50] if title else "",
        )

        start_time = time.perf_counter()

        try:
            # Generate tags inline using title and content from the request
            tags, inference_ms = await self._generate_tags_inline(article_id, title, content, feed_id)

            # Publish success reply
            await self._publish_reply(
                reply_to,
                correlation_id,
                {
                    "success": True,
                    "article_id": article_id,
                    "tags": tags,
                    "inference_ms": inference_ms,
                },
            )
        except Exception as e:
            elapsed_ms = (time.perf_counter() - start_time) * 1000
            logger.error(
                "tag_generation_request_failed",
                article_id=article_id,
                error=str(e),
                elapsed_ms=elapsed_ms,
            )
            # Publish error reply
            await self._publish_reply(
                reply_to,
                correlation_id,
                {
                    "success": False,
                    "article_id": article_id,
                    "error_message": str(e),
                    "inference_ms": elapsed_ms,
                },
            )

    async def _generate_tags_inline(
        self, article_id: str, title: str, content: str, feed_id: str
    ) -> tuple[list[dict], float]:
        """Generate tags without database access.

        Returns:
            Tuple of (tags_list, inference_ms)
        """
        import asyncio

        loop = asyncio.get_event_loop()

        def generate_sync() -> tuple[list[dict], float]:
            start = time.perf_counter()

            # Use tag extractor directly
            outcome = self.service.tag_extractor.extract_tags_with_metrics(title, content)

            inference_ms = (time.perf_counter() - start) * 1000

            # Convert tags to list of dicts with ID and confidence
            tags = []
            for tag_name in outcome.tags:
                confidence = outcome.tag_confidences.get(tag_name, 0.5)
                tags.append(
                    {
                        "id": str(uuid.uuid4()),
                        "name": tag_name,
                        "confidence": confidence,
                    }
                )

            return tags, inference_ms

        return await loop.run_in_executor(None, generate_sync)

    async def _publish_reply(
        self,
        reply_to: str,
        correlation_id: str | None,
        payload: dict,
    ) -> None:
        """Publish a reply event to the reply stream."""
        if not self.stream_consumer:
            logger.error("cannot_publish_reply_no_consumer")
            return

        event_data = {
            "event_id": str(uuid.uuid4()),
            "event_type": "TagGenerationCompleted",
            "payload": payload,
            "metadata": {"correlation_id": correlation_id or ""},
        }

        await self.stream_consumer.publish_reply(reply_to, event_data)

        logger.info(
            "tag_generation_reply_published",
            reply_to=reply_to,
            correlation_id=correlation_id,
            success=payload.get("success", False),
        )
