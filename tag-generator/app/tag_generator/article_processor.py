"""Article processing logic for tag generation."""

from typing import TYPE_CHECKING, Any

import structlog

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from tag_extractor.extract import TagExtractor
    from tag_generator.cascade import CascadeController
    from tag_generator.ports import TagInserterPort


class ArticleProcessor:
    """Handles processing of individual articles for tag generation."""

    def __init__(
        self,
        tag_extractor: "TagExtractor",
        tag_inserter: "TagInserterPort",
        cascade_controller: "CascadeController",
    ):
        """Initialize article processor with dependencies."""
        self.tag_extractor = tag_extractor
        self.tag_inserter = tag_inserter
        self.cascade_controller = cascade_controller

    def process_single_article(self, conn: Any, article: dict[str, Any]) -> bool:
        """
        Process a single article for tag extraction and insertion.

        Args:
            conn: Connection (unused in API mode, kept for interface compatibility)
            article: Article dictionary with id, title, content, created_at, feed_id, url

        Returns:
            True if successful, False otherwise
        """
        article_id = article["id"]
        title = article["title"]
        content = article["content"]
        feed_id = article.get("feed_id")

        # Skip if feed_id is missing
        if not feed_id:
            logger.warning(
                "Skipping article: feed_id is missing",
                article_id=article_id,
            )
            return False

        try:
            # Extract tags with lightweight metrics for cascade decisions
            outcome = self.tag_extractor.extract_tags_with_metrics(title, content)

            if not outcome.tags:
                logger.warning("Skipping article: no tags extracted", article_id=article_id)
                return False

            # Record cascade decision for metrics / downstream tracing
            decision = self.cascade_controller.evaluate(outcome)
            logger.info(
                "Cascade decision for article",
                article_id=article_id,
                **decision.as_dict(),
            )

            # Insert tags with confidences
            result = self.tag_inserter.upsert_tags(conn, article_id, outcome.tags, feed_id, outcome.tag_confidences)

            if result.get("success"):
                return True
            else:
                logger.warning(f"Tag insertion reported failure for article {article_id}")
                return False

        except Exception as e:
            logger.error(f"Error processing article {article_id}: {e}")
            return False
