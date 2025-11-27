"""
Tag fetcher module for retrieving tags from database.
Used by the batch API endpoint to fetch tags for multiple articles.
"""

import os
from contextlib import contextmanager
from datetime import datetime
from typing import Any

import psycopg2
import psycopg2.extras
import structlog

logger = structlog.get_logger(__name__)


def _get_database_dsn() -> str:
    """Build database connection string from environment variables."""
    required_vars = [
        "DB_TAG_GENERATOR_USER",
        "DB_TAG_GENERATOR_PASSWORD",
        "DB_HOST",
        "DB_PORT",
        "DB_NAME",
    ]

    missing_vars = [var for var in required_vars if not os.getenv(var)]
    if missing_vars:
        raise ValueError(f"Missing required environment variables: {missing_vars}")

    dsn = (
        f"postgresql://{os.getenv('DB_TAG_GENERATOR_USER')}:"
        f"{os.getenv('DB_TAG_GENERATOR_PASSWORD')}@"
        f"{os.getenv('DB_HOST')}:{os.getenv('DB_PORT')}/"
        f"{os.getenv('DB_NAME')}"
    )

    return dsn


@contextmanager
def _get_database_connection():
    """Get database connection context manager."""
    dsn = _get_database_dsn()
    conn = None
    try:
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        yield conn
    except psycopg2.Error as e:
        logger.error("Database connection failed", error=str(e))
        raise
    finally:
        if conn:
            conn.close()


def fetch_tags_by_article_ids(article_ids: list[str]) -> dict[str, list[dict[str, Any]]]:
    """
    Fetch tags for multiple articles by their IDs.

    Args:
        article_ids: List of article UUIDs as strings

    Returns:
        Dictionary mapping article_id to list of tag dictionaries with:
        - tag: tag name
        - confidence: confidence score (0.0-1.0)
        - source: source of the tag (default: "ml_model")
        - updated_at: timestamp when tag was created/updated
    """
    if not article_ids:
        return {}

    logger.info("Fetching tags for articles", article_count=len(article_ids))

    try:
        with _get_database_connection() as conn:
            with conn.cursor(cursor_factory=psycopg2.extras.DictCursor) as cursor:
                # Query to get tags for articles
                # JOIN article_tags and feed_tags to get tag names and confidence
                query = """
                    SELECT
                        a.id::text as article_id,
                        ft.tag_name,
                        ft.confidence,
                        ft.created_at as updated_at
                    FROM articles a
                    INNER JOIN article_tags at ON a.id = at.article_id
                    INNER JOIN feed_tags ft ON at.feed_tag_id = ft.id
                    WHERE a.id = ANY(%s::uuid[])
                    ORDER BY ft.confidence DESC
                """

                cursor.execute(query, (article_ids,))
                rows = cursor.fetchall()

                # Group tags by article_id
                result: dict[str, list[dict[str, Any]]] = {}
                for row in rows:
                    article_id = row["article_id"]
                    if article_id not in result:
                        result[article_id] = []

                    # Convert datetime to ISO format string
                    updated_at = row["updated_at"]
                    if isinstance(updated_at, datetime):
                        updated_at_str = updated_at.isoformat() + "Z"
                    else:
                        updated_at_str = updated_at.isoformat() if hasattr(updated_at, "isoformat") else str(updated_at)

                    result[article_id].append(
                        {
                            "tag": row["tag_name"],
                            "confidence": float(row["confidence"]),
                            "source": "ml_model",
                            "updated_at": updated_at_str,
                        }
                    )

                logger.info(
                    "Fetched tags for articles",
                    article_count=len(article_ids),
                    articles_with_tags=len(result),
                    total_tags=sum(len(tags) for tags in result.values()),
                )

                return result

    except Exception as e:
        logger.error("Failed to fetch tags by article IDs", error=str(e), article_count=len(article_ids))
        raise
