from typing import List, Dict, Optional, Any, TypedDict
from dataclasses import dataclass
from contextlib import contextmanager

import psycopg2
import psycopg2.extras
from psycopg2.extensions import connection as Connection, cursor as Cursor
import structlog

logger = structlog.get_logger(__name__)


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


class BatchResult(TypedDict):
    success: bool
    processed_articles: int
    failed_articles: int
    errors: List[str]
    message: Optional[str]


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

    def _insert_tags(self, cursor: Cursor, tags: List[str], feed_id: str) -> None:
        """
        Insert tags into the feed_tags table, ignoring duplicates.

        Args:
            cursor: Database cursor
            tags: List of tag names to insert
            feed_id: Feed UUID to associate tags with
        """
        if not tags:
            return

        try:
            tag_rows = [(feed_id, tag.strip(), 0.5) for tag in tags]  # Default confidence 0.5
            psycopg2.extras.execute_batch(
                cursor,
                """
                INSERT INTO feed_tags (feed_id, tag_name, confidence)
                VALUES (%s::uuid, %s, %s)
                ON CONFLICT (feed_id, tag_name) DO NOTHING
                """,
                tag_rows,
                page_size=self.config.page_size,
            )
            logger.debug("Inserted tags into feed_tags table", count=len(tag_rows))

        except psycopg2.Error as e:
            logger.error("Failed to insert tags", error=e)
            raise DatabaseError(f"Failed to insert tags: {e}") from e

    def _get_tag_ids(self, cursor: Cursor, tags: List[str], feed_id: str) -> Dict[str, str]:
        """
        Retrieve tag IDs for the given tag names from feed_tags table.

        Args:
            cursor: Database cursor
            tags: List of tag names
            feed_id: Feed UUID to filter tags by

        Returns:
            Dictionary mapping tag names to their UUIDs
        """
        if not tags:
            return {}

        try:
            clean_tags = [tag.strip() for tag in tags]
            cursor.execute(
                "SELECT id, tag_name FROM feed_tags WHERE tag_name = ANY(%s) AND feed_id = %s::uuid", 
                (clean_tags, feed_id)
            )

            id_map = {name: str(tag_id) for tag_id, name in cursor.fetchall()}

            # Check if all tags were found
            missing_tags = set(clean_tags) - set(id_map.keys())
            if missing_tags:
                logger.warning("Some tags were not found in database", missing_tags=missing_tags)

            logger.debug("Retrieved tag IDs", count=len(id_map))
            return id_map

        except psycopg2.Error as e:
            logger.error("Failed to retrieve tag IDs", error=e)
            raise DatabaseError(f"Failed to retrieve tag IDs: {e}") from e

    def _insert_article_tags(
        self, cursor: Cursor, article_id: str, tag_ids: Dict[str, str]
    ) -> None:
        """
        Insert article-tag relationships into the article_tags table.

        Args:
            cursor: Database cursor
            article_id: Article UUID as string
            tag_ids: Dictionary mapping tag names to their UUIDs
        """
        if not tag_ids:
            return

        try:
            rel_rows = [(article_id, tag_id) for tag_id in tag_ids.values()]
            psycopg2.extras.execute_batch(
                cursor,
                """
                INSERT INTO article_tags (article_id, feed_tag_id)
                VALUES (%s::uuid, %s::uuid)
                ON CONFLICT (article_id, feed_tag_id) DO NOTHING
                """,
                rel_rows,
                page_size=self.config.page_size,
            )
            logger.debug("Inserted article-tag relationships", count=len(rel_rows))

        except psycopg2.Error as e:
            logger.error("Failed to insert article-tag relationships", error=e)
            raise DatabaseError(
                f"Failed to insert article-tag relationships: {e}"
            ) from e

    def upsert_tags(
        self, conn: Connection, article_id: str, tags: List[str]
    ) -> Dict[str, Any]:
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
            return {
                "success": True,
                "tags_processed": 0,
                "message": "No valid tags to process",
            }

        logger.info("Processing unique tags for article", count=len(unique_tags), article_id=article_id)

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
                    "processed_tags": list(tag_id_map.keys()),
                }

                logger.info("Successfully processed tags for article", count=len(tag_id_map), article_id=article_id)
                return result

        except Exception as e:
            # Rollback on any error
            try:
                conn.rollback()
                logger.error("Transaction rolled back due to error", error=e)
            except Exception as rollback_error:
                logger.error("Failed to rollback transaction", error=rollback_error)

            raise

    def batch_upsert_tags(
        self, conn: Connection, article_tags: List[Dict[str, Any]]
    ) -> BatchResult:
        """
        Batch process multiple article-tag operations in a single transaction.

        Args:
            conn: Database connection
            article_tags: List of dictionaries with 'article_id' and 'tags' keys

        Returns:
            Dictionary with batch operation results
        """
        if not article_tags:
            return {
                "success": True,
                "processed_articles": 0,
                "failed_articles": 0,
                "errors": [],
                "message": "No articles to process",
            }

        logger.info("Starting batch processing of articles in single transaction", count=len(article_tags))

        results: BatchResult = {
            "success": True,
            "processed_articles": 0,
            "failed_articles": 0,
            "errors": [],
            "message": None,
        }

        try:
            with self._get_cursor(conn) as cursor:
                # Process all articles in a single transaction
                all_tags = set()  # Collect all unique tags first
                valid_article_tags = []

                # Validate and collect all data
                for item in article_tags:
                    try:
                        article_id = item.get("article_id")
                        if not article_id:
                            raise ValueError("Missing article_id in batch item")

                        tags = item.get("tags", [])
                        if not tags or not isinstance(tags, list):
                            continue  # Skip articles with no valid tags

                        # Clean and validate tags
                        clean_tags = [
                            tag.strip()
                            for tag in tags
                            if isinstance(tag, str) and tag.strip()
                        ]
                        if not clean_tags:
                            continue

                        valid_article_tags.append(
                            {"article_id": article_id, "tags": clean_tags}
                        )
                        all_tags.update(clean_tags)

                    except Exception as e:
                        results["failed_articles"] = results.get("failed_articles", 0) + 1
                        error_msg = f"Failed to validate article {item.get('article_id', 'unknown')}: {e}"
                        logger.error("Failed to validate article", article_id=item.get('article_id', 'unknown'), error=e)

                if not valid_article_tags:
                    logger.warning("No valid article-tag combinations found")
                    return {
                        "success": True,
                        "processed_articles": 0,
                        "failed_articles": 0,
                        "errors": [],
                        "message": "No valid articles to process",
                    }

                logger.info("Processing valid articles with unique tags", valid_articles=len(valid_article_tags), unique_tags=len(all_tags))

                # Step 1: Insert all unique tags at once
                self._insert_tags(cursor, list(all_tags))

                # Step 2: Get all tag IDs at once
                tag_id_map = self._get_tag_ids(cursor, list(all_tags))

                if not tag_id_map:
                    raise DatabaseError("No tag IDs could be retrieved")

                # Step 3: Insert all article-tag relationships
                all_relationships = []
                for article_data in valid_article_tags:
                    article_id = article_data["article_id"]
                    article_tags_list = article_data["tags"]

                    for tag in article_tags_list:
                        if tag in tag_id_map:
                            all_relationships.append((article_id, tag_id_map[tag]))

                if all_relationships:
                    # Batch insert all relationships at once
                    psycopg2.extras.execute_batch(
                        cursor,
                        """
                        INSERT INTO article_tags (article_id, tag_id)
                        VALUES (%s::uuid, %s)
                        ON CONFLICT (article_id, tag_id) DO NOTHING
                        """,
                        all_relationships,
                        page_size=self.config.page_size,
                    )
                    logger.info("Inserted article-tag relationships", count=len(all_relationships))

                # Commit the entire batch transaction
                conn.commit()

                results["processed_articles"] = len(valid_article_tags)
                logger.info("Successfully batch processed articles", count=results['processed_articles'])

        except Exception as e:
            # Rollback on any error
            try:
                conn.rollback()
                logger.error("Batch transaction rolled back due to error", error=e)
            except Exception as rollback_error:
                logger.error("Failed to rollback batch transaction", error=rollback_error)

            results["success"] = False
            results["failed_articles"] = len(article_tags)
            error_msg = f"Batch processing failed: {e}"
            results["errors"].append(error_msg)
            logger.error("Batch processing failed", error=e)

        if results["failed_articles"] > 0:
            results["success"] = False

        logger.info(
            f"Batch processing completed: {results['processed_articles']} successful, {results['failed_articles']} failed"
        )
        return results

    def batch_upsert_tags_no_commit(
        self, conn: Connection, article_tags: List[Dict[str, Any]]
    ) -> BatchResult:
        """
        Batch process multiple article-tag operations without auto-committing.
        Transaction management is left to the caller.

        Args:
            conn: Database connection (caller manages transaction)
            article_tags: List of dictionaries with 'article_id' and 'tags' keys

        Returns:
            Dictionary with batch operation results
        """
        if not article_tags:
            return {
                "success": True,
                "processed_articles": 0,
                "failed_articles": 0,
                "errors": [],
                "message": "No articles to process",
            }

        logger.info(
            "Starting batch processing of articles (caller manages transaction)",
            count=len(article_tags),
        )

        results: BatchResult = {
            "success": True,
            "processed_articles": 0,
            "failed_articles": 0,
            "errors": [],
            "message": None,
        }

        try:
            with self._get_cursor(conn) as cursor:
                # Process all articles in the current transaction (no commit here)
                all_tags = set()  # Collect all unique tags first
                valid_article_tags = []

                # Validate and collect all data
                for item in article_tags:
                    try:
                        article_id = item.get("article_id")
                        if not article_id:
                            raise ValueError("Missing article_id in batch item")

                        tags = item.get("tags", [])
                        if not tags or not isinstance(tags, list):
                            continue  # Skip articles with no valid tags

                        # Clean and validate tags
                        clean_tags = [
                            tag.strip()
                            for tag in tags
                            if isinstance(tag, str) and tag.strip()
                        ]
                        if not clean_tags:
                            continue

                        valid_article_tags.append(
                            {"article_id": article_id, "tags": clean_tags}
                        )
                        all_tags.update(clean_tags)

                    except Exception as e:
                        results["failed_articles"] = results.get("failed_articles", 0) + 1
                        error_msg = f"Failed to validate article {item.get('article_id', 'unknown')}: {e}"
                        results["errors"].append(error_msg)
                        logger.error(
                            "Failed to validate article",
                            article_id=item.get("article_id", "unknown"),
                            error=e,
                        )

                if not valid_article_tags:
                    logger.warning("No valid article-tag combinations found")
                    return {
                        "success": True,
                        "processed_articles": 0,
                        "failed_articles": 0,
                        "errors": [],
                        "message": "No valid articles to process",
                    }

                logger.info(
                    "Processing valid articles with unique tags",
                    valid_articles=len(valid_article_tags),
                    unique_tags=len(all_tags),
                )

                # Step 1: Insert all unique tags at once
                self._insert_tags(cursor, list(all_tags))

                # Step 2: Get all tag IDs at once
                tag_id_map = self._get_tag_ids(cursor, list(all_tags))

                if not tag_id_map:
                    raise DatabaseError("No tag IDs could be retrieved")

                # Step 3: Insert all article-tag relationships
                all_relationships = []
                for article_data in valid_article_tags:
                    article_id = article_data["article_id"]
                    article_tags_list = article_data["tags"]

                    for tag in article_tags_list:
                        if tag in tag_id_map:
                            all_relationships.append((article_id, tag_id_map[tag]))

                if all_relationships:
                    # Batch insert all relationships at once
                    psycopg2.extras.execute_batch(
                        cursor,
                        """
                        INSERT INTO article_tags (article_id, tag_id)
                        VALUES (%s::uuid, %s)
                        ON CONFLICT (article_id, tag_id) DO NOTHING
                        """,
                        all_relationships,
                        page_size=self.config.page_size,
                    )
                    logger.info(
                        "Inserted article-tag relationships",
                        count=len(all_relationships),
                    )

                # DO NOT commit here - let caller manage transaction
                results["processed_articles"] = len(valid_article_tags)
                logger.info(
                    "Successfully batch processed articles (transaction pending)",
                    count=results["processed_articles"],
                )

        except Exception as e:
            # DO NOT rollback here - let caller manage transaction
            results["success"] = False
            results["failed_articles"] = len(article_tags)
            error_msg = f"Batch processing failed: {e}"
            results["errors"].append(error_msg)
            logger.error("Batch processing failed", error=e)
            # Re-raise to let caller handle transaction rollback
            raise

        if results["failed_articles"] > 0:
            results["success"] = False

        logger.info(
            "Batch processing completed (no commit)",
            successful=results["processed_articles"],
            failed=results["failed_articles"],
        )
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
