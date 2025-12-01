"""Article processing logic for tag generation."""

from typing import TYPE_CHECKING, Any

import structlog
from psycopg2.extensions import connection as Connection

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from tag_extractor.extract import TagExtractor
    from tag_generator.cascade import CascadeController
    from tag_inserter.upsert_tags import TagInserter


class ArticleProcessor:
    """Handles processing of individual articles for tag generation."""

    def __init__(
        self,
        tag_extractor: "TagExtractor",
        tag_inserter: "TagInserter",
        cascade_controller: "CascadeController",
    ):
        """Initialize article processor with dependencies."""
        self.tag_extractor = tag_extractor
        self.tag_inserter = tag_inserter
        self.cascade_controller = cascade_controller

    def get_feed_id_from_url(self, conn: Connection, article_url: str) -> str | None:
        """
        Get feed_id from article URL by matching with feed.link.

        Args:
            conn: Database connection
            article_url: Article URL string

        Returns:
            Feed ID as string if found, None otherwise
        """
        try:
            with conn.cursor() as cursor:
                cursor.execute(
                    """
                    SELECT id::text
                    FROM feeds
                    WHERE link = %s
                    ORDER BY created_at DESC, id DESC
                    LIMIT 1
                    """,
                    (article_url,),
                )
                result = cursor.fetchone()
                if result:
                    return result[0]
                return None
        except Exception as e:
            logger.warning("Failed to get feed_id from URL", url=article_url, error=str(e))
            return None

    def process_single_article(self, conn: Connection, article: dict[str, Any]) -> bool:
        """
        Process a single article for tag extraction and insertion.

        Args:
            conn: Database connection
            article: Article dictionary with id, title, content, created_at, feed_id, url

        Returns:
            True if successful, False otherwise
        """
        article_id = article["id"]
        title = article["title"]
        content = article["content"]
        feed_id = article.get("feed_id")
        article_url = article.get("url")

        # If feed_id is missing, try to get it from article URL
        if not feed_id and article_url:
            feed_id = self.get_feed_id_from_url(conn, article_url)
            if feed_id:
                logger.info("Resolved feed_id from article URL", article_id=article_id, feed_id=feed_id)

        # Skip if feed_id is still missing
        if not feed_id:
            logger.warning(
                "Skipping article: feed_id is missing and could not be resolved from URL",
                article_id=article_id,
                url=article_url,
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
