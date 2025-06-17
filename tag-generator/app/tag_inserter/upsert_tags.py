import logging
from typing import List, Dict, Optional, Any
from dataclasses import dataclass
from contextlib import contextmanager

import psycopg2
import psycopg2.extras
from psycopg2.extensions import connection as Connection, cursor as Cursor

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

@dataclass
class TagInserterConfig:
    """Configuration for tag insertion operations."""
    batch_size: int = 1000
    page_size: int = 200
    max_retries: int = 3
    retry_delay: float = 1.0

class DatabaseError(Exception):
    """Custom exception for database-related errors."""
    pass

class TagInserter:
    """A class for efficiently inserting and managing tags in the database."""

    def __init__(self, config: Optional[TagInserterConfig] = None):
        self.config = config or TagInserterConfig()

    @contextmanager
    def _get_cursor(self, conn: Connection):
        """Context manager for database cursor with proper cleanup."""
        cursor = None
        try:
            cursor = conn.cursor()
            yield cursor
        except Exception as e:
            if cursor:
                cursor.close()
            raise DatabaseError(f"Database operation failed: {e}") from e
        finally:
            if cursor:
                cursor.close()

    def _validate_inputs(self, article_id: str, tags: List[str]) -> None:
        """Validate input parameters."""
        if not article_id or not isinstance(article_id, str):
            raise ValueError("article_id must be a non-empty string")

        if not tags or not isinstance(tags, list):
            raise ValueError("tags must be a non-empty list")

        if not all(isinstance(tag, str) and tag.strip() for tag in tags):
            raise ValueError("All tags must be non-empty strings")

    def _insert_tags(self, cursor: Cursor, tags: List[str]) -> None:
        """
        Insert tags into the tags table, ignoring duplicates.

        Args:
            cursor: Database cursor
            tags: List of tag names to insert
        """
        if not tags:
            return

        try:
            tag_rows = [(tag.strip(),) for tag in tags]
            psycopg2.extras.execute_batch(
                cursor,
                """
                INSERT INTO tags (name)
                VALUES (%s)
                ON CONFLICT (name) DO NOTHING
                """,
                tag_rows,
                page_size=self.config.page_size
            )
            logger.debug(f"Inserted {len(tag_rows)} tags into tags table")

        except psycopg2.Error as e:
            logger.error(f"Failed to insert tags: {e}")
            raise DatabaseError(f"Failed to insert tags: {e}") from e

    def _get_tag_ids(self, cursor: Cursor, tags: List[str]) -> Dict[str, int]:
        """
        Retrieve tag IDs for the given tag names.

        Args:
            cursor: Database cursor
            tags: List of tag names

        Returns:
            Dictionary mapping tag names to their IDs
        """
        if not tags:
            return {}

        try:
            clean_tags = [tag.strip() for tag in tags]
            cursor.execute(
                "SELECT id, name FROM tags WHERE name = ANY(%s)",
                (clean_tags,)
            )

            id_map = {name: tag_id for tag_id, name in cursor.fetchall()}

            # Check if all tags were found
            missing_tags = set(clean_tags) - set(id_map.keys())
            if missing_tags:
                logger.warning(f"Some tags were not found in database: {missing_tags}")

            logger.debug(f"Retrieved {len(id_map)} tag IDs")
            return id_map

        except psycopg2.Error as e:
            logger.error(f"Failed to retrieve tag IDs: {e}")
            raise DatabaseError(f"Failed to retrieve tag IDs: {e}") from e

    def _insert_article_tags(self, cursor: Cursor, article_id: str, tag_ids: Dict[str, int]) -> None:
        """
        Insert article-tag relationships into the article_tags table.

        Args:
            cursor: Database cursor
            article_id: Article UUID as string
            tag_ids: Dictionary mapping tag names to their IDs
        """
        if not tag_ids:
            return

        try:
            rel_rows = [(article_id, tag_id) for tag_id in tag_ids.values()]
            psycopg2.extras.execute_batch(
                cursor,
                """
                INSERT INTO article_tags (article_id, tag_id)
                VALUES (%s::uuid, %s)
                ON CONFLICT (article_id, tag_id) DO NOTHING
                """,
                rel_rows,
                page_size=self.config.page_size
            )
            logger.debug(f"Inserted {len(rel_rows)} article-tag relationships")

        except psycopg2.Error as e:
            logger.error(f"Failed to insert article-tag relationships: {e}")
            raise DatabaseError(f"Failed to insert article-tag relationships: {e}") from e

    def upsert_tags(self, conn: Connection, article_id: str, tags: List[str]) -> Dict[str, Any]:
        """
        Upsert tags into the tags table and create article-tag relationships.

        Args:
            conn: Database connection
            article_id: Article UUID as string
            tags: List of tag names

        Returns:
            Dictionary with operation results

        Raises:
            ValueError: If input parameters are invalid
            DatabaseError: If database operations fail
        """
        # Validate inputs
        self._validate_inputs(article_id, tags)

        # Remove duplicates and empty tags
        unique_tags = list(set(tag.strip() for tag in tags if tag.strip()))

        if not unique_tags:
            logger.warning("No valid tags provided after cleaning")
            return {"success": True, "tags_processed": 0, "message": "No valid tags to process"}

        logger.info(f"Processing {len(unique_tags)} unique tags for article {article_id}")

        try:
            with self._get_cursor(conn) as cursor:
                # Step 1: Insert tags (ignoring duplicates)
                self._insert_tags(cursor, unique_tags)

                # Step 2: Get tag IDs
                tag_id_map = self._get_tag_ids(cursor, unique_tags)

                if not tag_id_map:
                    raise DatabaseError("No tag IDs could be retrieved")

                # Step 3: Insert article-tag relationships
                self._insert_article_tags(cursor, article_id, tag_id_map)

                # Commit the transaction
                conn.commit()

                result = {
                    "success": True,
                    "tags_processed": len(tag_id_map),
                    "article_id": article_id,
                    "processed_tags": list(tag_id_map.keys())
                }

                logger.info(f"Successfully processed {len(tag_id_map)} tags for article {article_id}")
                return result

        except Exception as e:
            # Rollback on any error
            try:
                conn.rollback()
                logger.error(f"Transaction rolled back due to error: {e}")
            except Exception as rollback_error:
                logger.error(f"Failed to rollback transaction: {rollback_error}")

            raise

    def batch_upsert_tags(self, conn: Connection, article_tags: List[Dict[str, Any]]) -> Dict[str, Any]:
        """
        Batch process multiple article-tag operations.

        Args:
            conn: Database connection
            article_tags: List of dictionaries with 'article_id' and 'tags' keys

        Returns:
            Dictionary with batch operation results
        """
        if not article_tags:
            return {"success": True, "processed_articles": 0, "message": "No articles to process"}

        logger.info(f"Starting batch processing of {len(article_tags)} articles")

        results = {
            "success": True,
            "processed_articles": 0,
            "failed_articles": 0,
            "errors": []
        }

        for i, item in enumerate(article_tags):
            try:
                article_id = item.get("article_id")
                if not article_id:
                    raise ValueError("Missing article_id in batch item")

                tags = item.get("tags", [])

                self.upsert_tags(conn, article_id, tags)
                results["processed_articles"] += 1

                # Log progress for large batches
                if (i + 1) % 100 == 0:
                    logger.info(f"Processed {i + 1}/{len(article_tags)} articles")

            except Exception as e:
                results["failed_articles"] += 1
                error_msg = f"Failed to process article {item.get('article_id', 'unknown')}: {e}"
                results["errors"].append(error_msg)
                logger.error(error_msg)

                # Continue processing other articles
                continue

        if results["failed_articles"] > 0:
            results["success"] = False

        logger.info(f"Batch processing completed: {results['processed_articles']} successful, {results['failed_articles']} failed")
        return results

# Maintain backward compatibility
def upsert_tags(conn: Connection, article_id: str, tags: List[str]) -> None:
    """
    Legacy function for backward compatibility.

    Args:
        conn: Database connection
        article_id: Article UUID as string
        tags: List of tag names
    """
    inserter = TagInserter()
    inserter.upsert_tags(conn, article_id, tags)