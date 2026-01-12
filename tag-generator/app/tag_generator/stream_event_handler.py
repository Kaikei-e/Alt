"""Event handler for tag-generator Redis Streams consumer."""

from typing import TYPE_CHECKING

import structlog

from tag_generator.stream_consumer import Event, EventHandler

if TYPE_CHECKING:
    from tag_generator.service import TagGeneratorService

logger = structlog.get_logger(__name__)


class TagGeneratorEventHandler(EventHandler):
    """Handles events for tag-generator service."""

    EVENT_TYPE_ARTICLE_CREATED = "ArticleCreated"

    def __init__(self, service: "TagGeneratorService") -> None:
        self.service = service

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
        """
        # Note: TagGeneratorService.process_single_article needs to be async
        # or wrapped in an executor. For now, we assume sync processing.
        # The actual implementation will depend on the service's interface.

        # Placeholder for integration with TagGeneratorService
        # In the full implementation, this would call:
        # self.service.process_single_article(article_id)

        logger.info(
            "article_processed_for_tags",
            article_id=article_id,
        )
