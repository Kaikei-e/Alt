from dataclasses import dataclass
from typing import Any

import psycopg2
import psycopg2.extras
import structlog
from psycopg2.extensions import connection as Connection
from psycopg2.extras import DictCursor

logger = structlog.get_logger(__name__)


@dataclass
class ArticleFetcherConfig:
    """Configuration for article fetching operations."""

    batch_size: int = 500
    max_retries: int = 3
    retry_delay: float = 1.0
    timeout: int = 30


class ArticleFetchError(Exception):
    """Custom exception for article fetching errors."""

    pass


class ArticleFetcher:
    """A class for efficiently fetching articles from the database."""

    def __init__(self, config: ArticleFetcherConfig | None = None):
        self.config = config or ArticleFetcherConfig()
        logger.info("ArticleFetcher initialized", batch_size=self.config.batch_size)

    def _validate_cursor_params(self, last_created_at: str, last_id: str) -> None:
        """Validate cursor parameters for pagination."""
        if not isinstance(last_created_at, str) or not last_created_at.strip():
            raise ValueError("last_created_at must be a non-empty string")

        if not isinstance(last_id, str) or not last_id.strip():
            raise ValueError("last_id must be a non-empty string")

    def _build_fetch_query(self, untagged_only: bool = True) -> str:
        """Build the SQL query for fetching articles."""
        untagged_filter = ""
        if untagged_only:
            untagged_filter = """
                LEFT JOIN article_tags at ON articles.id = at.article_id
                WHERE at.article_id IS NULL
                AND (
            """
        else:
            untagged_filter = "WHERE ("

        return f"""
            SELECT
                articles.id::text AS id,
                articles.title,
                articles.content,
                articles.created_at,
                COALESCE(articles.feed_id::text, NULL) AS feed_id,
                articles.url
            FROM articles
            {untagged_filter}
                (articles.created_at < %s)
                OR (articles.created_at = %s AND articles.id::text < %s)
            )
            ORDER BY articles.created_at DESC, articles.id DESC
            LIMIT %s
        """

    def fetch_articles(
        self,
        conn: Connection,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
        untagged_only: bool = True,
    ) -> list[dict[str, Any]]:
        """
        Fetch articles from the database using cursor-based pagination.
        By default, fetches only untagged articles in descending order (newest to oldest).

        Args:
            conn: Database connection
            last_created_at: ISO timestamp string for pagination
            last_id: Article ID string for pagination
            custom_batch_size: Override default batch size if provided
            untagged_only: If True, fetch only untagged articles (default: True)

        Returns:
            List of article dictionaries with id, title, content, created_at

        Raises:
            ValueError: If input parameters are invalid
            ArticleFetchError: If database operations fail
        """
        # Validate inputs
        self._validate_cursor_params(last_created_at, last_id)

        batch_size = custom_batch_size or self.config.batch_size
        if batch_size <= 0:
            raise ValueError("batch_size must be positive")

        logger.debug(
            "Fetching articles with cursor",
            created_at=last_created_at,
            id=last_id,
            batch_size=batch_size,
            untagged_only=untagged_only,
        )

        try:
            query = self._build_fetch_query(untagged_only=untagged_only)

            with conn.cursor(cursor_factory=DictCursor) as cursor:
                cursor.execute(query, (last_created_at, last_created_at, last_id, batch_size))

                rows = cursor.fetchall()

                logger.info("Fetched articles by cursor", count=len(rows), untagged_only=untagged_only)

                # Convert DictRow to regular dict for better type safety
                articles = [dict(row) for row in rows]

                return articles

        except psycopg2.Error as e:
            # Ensure the exception object is converted to a string for safe JSON
            # serialisation
            logger.error("Failed to fetch articles", error=str(e))
            # Propagate a domain-specific error with a stable message for callers/tests
            raise ArticleFetchError("Failed to fetch articles") from e
        except Exception as e:
            logger.error("Unexpected error while fetching articles", error=str(e))
            raise ArticleFetchError("Failed to fetch articles") from e

    def fetch_articles_by_status(
        self, conn: Connection, has_tags: bool = False, limit: int | None = None
    ) -> list[dict[str, Any]]:
        """
        Fetch articles based on whether they have tags or not.

        Args:
            conn: Database connection
            has_tags: If True, fetch articles with tags; if False, fetch articles
                without tags
            limit: Maximum number of articles to fetch

        Returns:
            List of article dictionaries
        """
        batch_size = limit or self.config.batch_size

        if has_tags:
            query = """
                SELECT DISTINCT
                    a.id::text AS id,
                    a.title,
                    a.content,
                    a.created_at,
                    COALESCE(a.feed_id::text, NULL) AS feed_id,
                    a.url
                FROM articles a
                INNER JOIN article_tags at ON a.id = at.article_id
                ORDER BY a.created_at DESC, a.id DESC
                LIMIT %s
            """
        else:
            query = """
                SELECT
                    a.id::text AS id,
                    a.title,
                    a.content,
                    a.created_at,
                    COALESCE(a.feed_id::text, NULL) AS feed_id,
                    a.url
                FROM articles a
                LEFT JOIN article_tags at ON a.id = at.article_id
                WHERE at.article_id IS NULL
                ORDER BY a.created_at DESC, a.id DESC
                LIMIT %s
            """

        try:
            with conn.cursor(cursor_factory=DictCursor) as cursor:
                cursor.execute(query, (batch_size,))
                rows = cursor.fetchall()

                logger.info("Fetched articles by tag status", count=len(rows), has_tags=has_tags)

                return [dict(row) for row in rows]

        except psycopg2.Error as e:
            logger.error("Failed to fetch articles by tag status", error=str(e))
            raise ArticleFetchError("Failed to fetch articles") from e

    def fetch_article_by_id(self, conn: Connection, article_id: str) -> dict[str, Any] | None:
        """Fetch a single article by ID.

        Args:
            conn: Database connection
            article_id: Article UUID string

        Returns:
            Article dictionary or None if not found
        """
        if not article_id or not isinstance(article_id, str):
            raise ValueError("article_id must be a non-empty string")

        query = """
            SELECT
                articles.id::text AS id,
                articles.title,
                articles.content,
                articles.created_at,
                COALESCE(articles.feed_id::text, NULL) AS feed_id,
                articles.url
            FROM articles
            WHERE articles.id = %s::uuid
        """

        try:
            with conn.cursor(cursor_factory=DictCursor) as cursor:
                cursor.execute(query, (article_id,))
                row = cursor.fetchone()
                if row:
                    logger.info("Fetched article by id", article_id=article_id)
                    return dict(row)
                logger.info("Article not found", article_id=article_id)
                return None
        except psycopg2.Error as e:
            logger.error("Failed to fetch article by id", article_id=article_id, error=str(e))
            raise ArticleFetchError("Failed to fetch article by id") from e

    def fetch_new_articles(
        self,
        conn: Connection,
        last_created_at: str,
        last_id: str,
        custom_batch_size: int | None = None,
    ) -> list[dict[str, Any]]:
        """Fetch untagged articles newer than the provided cursor in ascending order."""

        self._validate_cursor_params(last_created_at, last_id)

        batch_size = custom_batch_size or self.config.batch_size
        if batch_size <= 0:
            raise ValueError("batch_size must be positive")

        query = """
            SELECT
                a.id::text AS id,
                a.title,
                a.content,
                a.created_at,
                COALESCE(a.feed_id::text, NULL) AS feed_id,
                a.url
            FROM articles a
            LEFT JOIN article_tags at ON a.id = at.article_id
            WHERE at.article_id IS NULL AND (
                a.created_at > %s OR (a.created_at = %s AND a.id::text > %s)
            )
            ORDER BY a.created_at ASC, a.id ASC
            LIMIT %s
        """

        try:
            with conn.cursor(cursor_factory=DictCursor) as cursor:
                cursor.execute(query, (last_created_at, last_created_at, last_id, batch_size))
                rows = cursor.fetchall()
                logger.info("Fetched forward articles by cursor", count=len(rows))
                return [dict(row) for row in rows]
        except psycopg2.Error as e:
            logger.error("Failed to fetch new articles", error=str(e))
            raise ArticleFetchError("Failed to fetch articles") from e

    def count_untagged_articles(self, conn: Connection) -> int:
        """
        Count the number of articles without tags.

        Args:
            conn: Database connection

        Returns:
            Number of untagged articles
        """
        query = """
            SELECT COUNT(*)
            FROM articles a
            LEFT JOIN article_tags at ON a.id = at.article_id
            WHERE at.article_id IS NULL
        """

        try:
            with conn.cursor() as cursor:
                cursor.execute(query)
                result = cursor.fetchone()

                if result is None:
                    logger.warning("No result returned from count query for untagged articles")
                    return 0

                count: int = result[0]
                logger.info("Found untagged articles", count=count)
                return count

        except psycopg2.Error as e:
            logger.error("Failed to count untagged articles", error=str(e))
            raise ArticleFetchError("Failed to fetch articles") from e

    def fetch_all_articles_for_regeneration(
        self,
        conn: Connection,
        offset: int = 0,
        limit: int | None = None,
    ) -> list[dict[str, Any]]:
        """
        Fetch all articles for full regeneration.

        Uses offset-based pagination for sequential processing of all articles.
        This method is optimized for batch regeneration tasks where we need to
        process every article in the database.

        Args:
            conn: Database connection
            offset: Number of articles to skip (for pagination)
            limit: Maximum number of articles to fetch (default: batch_size from config)

        Returns:
            List of article dictionaries with id, title, content, created_at

        Raises:
            ArticleFetchError: If database operations fail
        """
        batch_size = limit or self.config.batch_size

        query = """
            SELECT
                a.id::text AS id,
                a.title,
                a.content,
                a.created_at
            FROM articles a
            WHERE a.content IS NOT NULL AND a.content != ''
            ORDER BY a.id
            OFFSET %s
            LIMIT %s
        """

        try:
            with conn.cursor(cursor_factory=DictCursor) as cursor:
                cursor.execute(query, (offset, batch_size))
                rows = cursor.fetchall()

                logger.info(
                    "Fetched articles for regeneration",
                    count=len(rows),
                    offset=offset,
                    limit=batch_size,
                )

                return [dict(row) for row in rows]

        except psycopg2.Error as e:
            logger.error(
                "Failed to fetch articles for regeneration",
                error=str(e),
                offset=offset,
            )
            raise ArticleFetchError("Failed to fetch articles for regeneration") from e

    def fetch_low_confidence_articles(
        self,
        conn: Connection,
        confidence_threshold: float = 0.5,
        limit: int | None = None,
    ) -> list[dict[str, Any]]:
        """
        Fetch articles with average tag confidence below the threshold.

        This method retrieves articles that have been tagged but have low-quality
        tags based on the average confidence score. These articles are candidates
        for tag regeneration.

        Args:
            conn: Database connection
            confidence_threshold: Maximum average confidence to include (default: 0.5)
            limit: Maximum number of articles to fetch (default: batch_size from config)

        Returns:
            List of article dictionaries including avg_confidence

        Raises:
            ArticleFetchError: If database operations fail
        """
        batch_size = limit or self.config.batch_size

        query = """
            SELECT
                a.id::text AS id,
                a.title,
                a.content,
                a.created_at,
                COALESCE(a.feed_id::text, NULL) AS feed_id,
                a.url,
                AVG(ft.confidence) AS avg_confidence
            FROM articles a
            INNER JOIN article_tags at ON a.id = at.article_id
            INNER JOIN feed_tags ft ON at.feed_tag_id = ft.id
            GROUP BY a.id, a.title, a.content, a.created_at, a.feed_id, a.url
            HAVING AVG(ft.confidence) < %s
            ORDER BY AVG(ft.confidence) ASC
            LIMIT %s
        """

        try:
            with conn.cursor(cursor_factory=DictCursor) as cursor:
                cursor.execute(query, (confidence_threshold, batch_size))
                rows = cursor.fetchall()

                logger.info(
                    "Fetched low-confidence articles",
                    count=len(rows),
                    threshold=confidence_threshold,
                )

                return [dict(row) for row in rows]

        except psycopg2.Error as e:
            logger.error(
                "Failed to fetch low-confidence articles",
                error=str(e),
                threshold=confidence_threshold,
            )
            raise ArticleFetchError("Failed to fetch low-confidence articles") from e


# Maintain backward compatibility
def fetch_articles(conn: Connection, last_created_at: str, last_id: str) -> list[dict[str, Any]]:
    """
    Legacy function for backward compatibility.

    Args:
        conn: Database connection
        last_created_at: ISO timestamp string for pagination
        last_id: Article ID string for pagination

    Returns:
        List of article dictionaries
    """
    fetcher = ArticleFetcher()
    return fetcher.fetch_articles(conn, last_created_at, last_id)
