from typing import List, Dict, Any, Optional
from dataclasses import dataclass

import psycopg2
import psycopg2.extras
from psycopg2.extras import DictCursor
from psycopg2.extensions import connection as Connection
import structlog

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

    def __init__(self, config: Optional[ArticleFetcherConfig] = None):
        self.config = config or ArticleFetcherConfig()
        logger.info("ArticleFetcher initialized", batch_size=self.config.batch_size)

    def _validate_cursor_params(self, last_created_at: str, last_id: str) -> None:
        """Validate cursor parameters for pagination."""
        if not isinstance(last_created_at, str) or not last_created_at.strip():
            raise ValueError("last_created_at must be a non-empty string")

        if not isinstance(last_id, str) or not last_id.strip():
            raise ValueError("last_id must be a non-empty string")

    def _build_fetch_query(self) -> str:
        """Build the SQL query for fetching articles."""
        return """
            SELECT
                id::text AS id,
                title,
                content,
                created_at
            FROM articles
            WHERE
                (created_at < %s)
                OR (created_at = %s AND id::text < %s)
            ORDER BY created_at DESC, id DESC
            LIMIT %s
        """

    def fetch_articles(
        self,
        conn: Connection,
        last_created_at: str,
        last_id: str,
        custom_batch_size: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """
        Fetch articles from the database using cursor-based pagination.

        Args:
            conn: Database connection
            last_created_at: ISO timestamp string for pagination
            last_id: Article ID string for pagination
            custom_batch_size: Override default batch size if provided

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

        logger.debug("Fetching articles with cursor", created_at=last_created_at, id=last_id, batch_size=batch_size)

        try:
            query = self._build_fetch_query()

            with conn.cursor(cursor_factory=DictCursor) as cursor:
                cursor.execute(
                    query, (last_created_at, last_created_at, last_id, batch_size)
                )

                rows = cursor.fetchall()

                logger.info("Successfully fetched articles", count=len(rows))

                # Convert DictRow to regular dict for better type safety
                articles = [dict(row) for row in rows]

                return articles

        except psycopg2.Error as e:
            logger.error("Failed to fetch articles", error=e)
            raise ArticleFetchError(str(e)) from e
        except Exception as e:
            logger.error("Unexpected error while fetching articles", error=e)
            raise ArticleFetchError(str(e)) from e

    def fetch_articles_by_status(
        self, conn: Connection, has_tags: bool = False, limit: Optional[int] = None
    ) -> List[Dict[str, Any]]:
        """
        Fetch articles based on whether they have tags or not.

        Args:
            conn: Database connection
            has_tags: If True, fetch articles with tags; if False, fetch articles without tags
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
                    a.created_at
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
                    a.created_at
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
            logger.error("Failed to fetch articles by tag status", error=e)
            raise ArticleFetchError(str(e)) from e

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
            logger.error("Failed to count untagged articles", error=e)
            raise ArticleFetchError(str(e)) from e


# Maintain backward compatibility
def fetch_articles(
    conn: Connection, last_created_at: str, last_id: str
) -> List[Dict[str, Any]]:
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
