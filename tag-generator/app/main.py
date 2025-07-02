import gc
import os
import time
from datetime import datetime, UTC, timedelta
from typing import Optional, Dict, Any, List, cast
from dataclasses import dataclass
from contextlib import contextmanager

import psycopg2
import psycopg2.extensions
import structlog
from psycopg2.extensions import connection as Connection

from article_fetcher.fetch import ArticleFetcher
from tag_extractor.extract import TagExtractor
from tag_inserter.upsert_tags import TagInserter
from tag_generator.logging_config import setup_logging

# Configure logging
setup_logging()
logger = structlog.get_logger(__name__)


@dataclass
class TagGeneratorConfig:
    """Configuration for the tag generation service."""

    processing_interval: int = 60  # seconds between processing batches
    error_retry_interval: int = 60  # seconds to wait after errors
    batch_limit: int = 75  # articles per processing cycle
    progress_log_interval: int = 10  # log progress every N articles
    enable_gc_collection: bool = True  # enable manual garbage collection
    memory_cleanup_interval: int = 25  # articles between memory cleanup
    max_connection_retries: int = 3  # max database connection retries
    connection_retry_delay: float = 5.0  # seconds between connection attempts
    # Health monitoring
    health_check_interval: int = 10  # cycles between health checks
    max_consecutive_empty_cycles: int = 20  # max cycles with 0 articles before warning


class DatabaseConnectionError(Exception):
    """Custom exception for database connection errors."""

    pass


class TagGeneratorService:
    """Main service class for tag generation operations."""

    def __init__(self, config: Optional[TagGeneratorConfig] = None):
        self.config = config or TagGeneratorConfig()
        self.article_fetcher = ArticleFetcher()
        self.tag_extractor = TagExtractor()
        self.tag_inserter = TagInserter()

        # Persistent cursor position for pagination between cycles
        self.last_processed_created_at: Optional[str] = None
        self.last_processed_id: Optional[str] = None

        # Health monitoring
        self.consecutive_empty_cycles = 0
        self.total_cycles = 0
        self.total_articles_processed = 0
        self.last_health_check_cycle: int = 0

        # Using direct database connections (connection pool disabled due to hanging issues)
        self._connection_pool = None

        logger.info("Tag Generator Service initialized")
        logger.info(f"Configuration: {self.config}")

    def _get_database_dsn(self) -> str:
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

    def _get_database_connection(self):
        """Get database connection using direct connection."""
        return self._create_direct_connection_context()

    @contextmanager
    def _create_direct_connection_context(self):
        """Create direct database connection as context manager."""
        conn = None
        try:
            conn = self._create_direct_connection()
            yield conn
        finally:
            if conn:
                try:
                    conn.close()
                except Exception as e:
                    logger.warning(f"Error closing direct connection: {e}")

    def _create_direct_connection(self) -> Connection:
        """Create direct database connection with retry logic."""
        assert self.config is not None
        dsn = self._get_database_dsn()

        for attempt in range(self.config.max_connection_retries):
            try:
                logger.info(
                    f"Attempting database connection (attempt {attempt + 1}/{self.config.max_connection_retries})"
                )
                conn = psycopg2.connect(dsn)

                # Ensure connection starts in a clean state
                try:
                    # First, check if we're in a transaction and rollback if needed
                    if conn.status != psycopg2.extensions.STATUS_READY:
                        conn.rollback()

                    # Ensure autocommit is enabled
                    if not conn.autocommit:
                        conn.autocommit = True

                    logger.info("Database connected successfully")
                    return conn
                except Exception as setup_error:
                    logger.warning(f"Failed to setup connection state: {setup_error}")
                    # If we can't set up the connection properly, close it and try again
                    try:
                        conn.close()
                    except Exception:
                        pass
                    raise setup_error

            except psycopg2.Error as e:
                logger.error(f"Database connection failed (attempt {attempt + 1}): {e}")

                if attempt < self.config.max_connection_retries - 1:
                    logger.info(
                        f"Retrying in {self.config.connection_retry_delay} seconds..."
                    )
                    time.sleep(self.config.connection_retry_delay)
                else:
                    raise DatabaseConnectionError(
                        f"Failed to connect after {self.config.max_connection_retries} attempts"
                    ) from e
        raise DatabaseConnectionError("Failed to establish database connection after multiple retries")

    def _get_initial_cursor_position(self) -> tuple[str, str]:
        """Get initial cursor position for pagination with robust recovery mechanisms."""
        if self.last_processed_created_at and self.last_processed_id:
            # Check for cursor poisoning (timestamp in the future or too old)
            try:
                cursor_time = datetime.fromisoformat(
                    self.last_processed_created_at.replace("Z", "+00:00")
                )
                current_time = datetime.now(UTC)
                time_diff = cursor_time - current_time

                # Check for various cursor poisoning scenarios
                cursor_is_poisoned = False
                reason = ""

                if time_diff.total_seconds() > 3600:  # More than 1 hour in future
                    cursor_is_poisoned = True
                    reason = (
                        f"cursor {time_diff.total_seconds() / 3600:.1f} hours in future"
                    )
                elif time_diff.total_seconds() < -86400 * 30:  # More than 30 days old
                    cursor_is_poisoned = True
                    reason = (
                        f"cursor {abs(time_diff.total_seconds()) / 86400:.1f} days old"
                    )

                if cursor_is_poisoned:
                    logger.warning(f"Detected cursor poisoning: {reason}")
                    logger.warning("Switching to untagged article recovery mode")
                    return self._get_recovery_cursor_position()
                else:
                    # Continue from where we left off
                    last_created_at = self.last_processed_created_at
                    last_id = self.last_processed_id
                    logger.info(
                        f"Continuing article processing from cursor: {last_created_at}, ID: {last_id}"
                    )
                    return last_created_at, last_id

            except (ValueError, TypeError) as e:
                logger.warning(
                    f"Invalid cursor timestamp format: {self.last_processed_created_at}, error: {e}"
                )
                logger.warning("Switching to recovery mode due to invalid format")
                return self._get_recovery_cursor_position()
        else:
            # First run - use recovery mode to find untagged articles
            logger.info("First run - searching for untagged articles")
            return self._get_recovery_cursor_position()

    def _get_recovery_cursor_position(self) -> tuple[str, str]:
        """Get cursor position for recovery mode - prioritizes untagged articles."""
        try:
            with self._get_database_connection() as conn:
                # Try to find the most recent untagged article
                query = """
                    SELECT
                        a.id::text AS id,
                        a.created_at
                    FROM articles a
                    LEFT JOIN article_tags at ON a.id = at.article_id
                    WHERE at.article_id IS NULL
                    ORDER BY a.created_at DESC, a.id DESC
                    LIMIT 1
                """

                with conn.cursor() as cursor:
                    cursor.execute(query)
                    result = cursor.fetchone()

                    if result:
                        # Start from just after the most recent untagged article
                        most_recent_untagged_time = result[1]
                        if isinstance(most_recent_untagged_time, str):
                            start_time = most_recent_untagged_time
                        else:
                            start_time = most_recent_untagged_time.isoformat()

                        # Add a small buffer to ensure we catch this article
                        start_time_dt = datetime.fromisoformat(
                            start_time.replace("Z", "+00:00")
                        )
                        start_time_dt += timedelta(microseconds=1)
                        start_time = start_time_dt.isoformat()

                        logger.info(
                            f"Recovery mode: Starting from most recent untagged article at {start_time}"
                        )
                        return start_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"
                    else:
                        # No untagged articles found, start from a reasonable past date
                        past_date = datetime.now(UTC) - timedelta(
                            days=7
                        )  # Look back 7 days
                        start_time = past_date.isoformat()
                        logger.info(
                            f"Recovery mode: No untagged articles found, starting from {start_time}"
                        )
                        return start_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

        except Exception as e:
            logger.error(f"Failed to determine recovery cursor position: {e}")
            # Fallback to a reasonable past date
            past_date = datetime.now(UTC) - timedelta(days=1)
            start_time = past_date.isoformat()
            logger.warning(f"Using fallback recovery cursor: {start_time}")
            return start_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

    def _fetch_untagged_articles_fallback(
        self, conn: Connection
    ) -> List[Dict[str, Any]]:
        """
        Fallback method to fetch untagged articles when cursor pagination fails.

        Args:
            conn: Database connection

        Returns:
            List of untagged articles
        """
        assert self.config is not None
        try:
            # Use the ArticleFetcher's method for fetching untagged articles
            untagged_articles = self.article_fetcher.fetch_articles_by_status(
                conn, has_tags=False, limit=self.config.batch_limit
            )

            logger.info(
                f"Fallback method retrieved {len(untagged_articles)} untagged articles"
            )
            return untagged_articles

        except Exception as e:
            logger.error(f"Fallback method failed to fetch untagged articles: {e}")
            return []

    def _cleanup_memory(self) -> None:
        """Explicit memory cleanup to prevent accumulation."""
        assert self.config is not None
        if self.config.enable_gc_collection:
            gc.collect()

    def _process_single_article(
        self, conn: Connection, article: Dict[str, Any]
    ) -> bool:
        """
        Process a single article for tag extraction and insertion.

        Args:
            conn: Database connection
            article: Article dictionary with id, title, content, created_at

        Returns:
            True if successful, False otherwise
        """
        assert self.config is not None
        article_id = article["id"]
        title = article["title"]
        content = article["content"]

        try:
            # Extract tags
            tags = self.tag_extractor.extract_tags(title, content)

            # Insert tags
            result = self.tag_inserter.upsert_tags(conn, article_id, tags)

            if result.get("success"):
                return True
            else:
                logger.warning(
                    f"Tag insertion reported failure for article {article_id}"
                )
                return False

        except Exception as e:
            logger.error(f"Error processing article {article_id}: {e}")
            return False

    def _process_article_batch(self, conn: Connection) -> Dict[str, Any]:
        """
        Process a batch of articles for tag generation using true batch processing.
        Includes fallback mechanism for cursor pagination failures.

        Args:
            conn: Database connection

        Returns:
            Dictionary with batch processing results
        """
        assert self.config is not None
        last_created_at, last_id = self._get_initial_cursor_position()

        batch_stats = {
            "total_processed": 0,
            "successful": 0,
            "failed": 0,
            "last_created_at": last_created_at,
            "last_id": last_id,
        }

        # Collect articles for batch processing (keep autocommit for fetching)
        articles_to_process: List[Dict[str, Any]] = []
        fetch_attempts = 0
        max_empty_fetches = 3  # Allow 3 empty fetches before switching to fallback

        while len(articles_to_process) < int(self.config.batch_limit):
            try:
                # Fetch articles using cursor pagination
                articles = self.article_fetcher.fetch_articles(
                    conn, last_created_at, last_id
                )

                if not articles:
                    fetch_attempts += 1
                    logger.info(
                        f"No articles found with cursor pagination (attempt {fetch_attempts})"
                    )

                    # If we consistently get no results, try fallback approach
                    if (
                        fetch_attempts >= max_empty_fetches
                        and len(articles_to_process) == 0
                    ):
                        logger.warning(
                            "Cursor pagination consistently failing, switching to untagged article fallback"
                        )
                        fallback_articles = self._fetch_untagged_articles_fallback(conn)
                        if fallback_articles:
                            articles_to_process.extend(
                                fallback_articles[: self.config.batch_limit]
                            )
                            logger.info(
                                f"Fallback method found {len(fallback_articles)} untagged articles"
                            )
                            # Update cursor based on the last article processed
                            if articles_to_process:
                                last_article = articles_to_process[-1]
                                if isinstance(last_article["created_at"], str):
                                    last_created_at = last_article["created_at"]
                                else:
                                    last_created_at = last_article[
                                        "created_at"
                                    ].isoformat()
                                last_id = last_article["id"]
                            break
                        else:
                            logger.info(
                                "No untagged articles found via fallback method"
                            )
                            break
                    else:
                        logger.info(
                            f"No more articles found. Collected {len(articles_to_process)} articles for batch processing"
                        )
                        break

                logger.info(f"Fetched {len(articles)} articles")
                fetch_attempts = 0  # Reset counter on successful fetch

                # Add articles to batch, respecting the batch limit
                assert self.config.batch_limit is not None
                for article in articles:
                    if len(articles_to_process) >= self.config.batch_limit:
                        logger.info(
                            f"Reached batch limit of {self.config.batch_limit} articles"
                        )
                        break

                    articles_to_process.append(article)

                    # Update cursor position for next fetch (convert datetime to string)
                    if isinstance(article["created_at"], str):
                        last_created_at = article["created_at"]
                    else:
                        last_created_at = article["created_at"].isoformat()
                    last_id = article["id"]

                # Break if we've reached the batch limit
                if len(articles_to_process) >= self.config.batch_limit:
                    break

            except Exception as e:
                logger.error(f"Error during article collection: {e}")
                # Try fallback method on exception
                if len(articles_to_process) == 0:
                    logger.warning("Attempting fallback method due to fetch error")
                    try:
                        fallback_articles = self._fetch_untagged_articles_fallback(conn)
                        if fallback_articles:
                            articles_to_process.extend(
                                fallback_articles[: self.config.batch_limit]
                            )
                            logger.info(
                                f"Fallback method recovered {len(fallback_articles)} articles"
                            )
                    except Exception as fallback_error:
                        logger.error(f"Fallback method also failed: {fallback_error}")
                break

        # Start explicit transaction for batch processing only
        try:
            if conn.autocommit:
                conn.autocommit = False

            if articles_to_process:
                logger.info(
                    f"Processing batch of {len(articles_to_process)} articles..."
                )
                batch_stats = self._process_articles_as_batch(conn, articles_to_process)
                # Ensure string format for batch stats
                batch_stats["last_created_at"] = last_created_at
                batch_stats["last_id"] = last_id

                # Update persistent cursor position for next cycle (ensure string format)
                self.last_processed_created_at = last_created_at
                self.last_processed_id = last_id
                logger.info(
                    f"Updated cursor position for next cycle: {self.last_processed_created_at}, ID: {last_id}"
                )

                # Commit the transaction only if batch processing was successful
                if cast(int, batch_stats.get("successful", 0)) > 0:
                    conn.commit()
                else:
                    conn.rollback()
                    logger.warning(
                        "Transaction rolled back due to batch processing failure"
                    )
            else:
                # No articles to process, still commit to end transaction cleanly
                conn.commit()

        except Exception as e:
            logger.error(f"Error during batch processing: {e}")
            try:
                conn.rollback()
            except Exception as rollback_error:
                logger.error(f"Failed to rollback transaction: {rollback_error}")
            raise
        finally:
            # Reset autocommit mode
            try:
                if not conn.autocommit:
                    conn.autocommit = True
            except Exception as e:
                logger.warning(f"Failed to restore autocommit mode: {e}")

        return batch_stats

    def _process_articles_as_batch(
        self, conn: Connection, articles: List[Dict[str, Any]]
    ) -> Dict[str, Any]:
        """
        Process multiple articles as a single batch transaction.
        Note: Transaction management is handled by the caller.

        Args:
            conn: Database connection (should already be in transaction mode)
            articles: List of articles to process

        Returns:
            Dictionary with batch processing results
        """
        batch_stats: Dict[str, int] = {"total_processed": 0, "successful": 0, "failed": 0}

        # Prepare batch data for tag insertion
        article_tags_batch = []

        # Extract tags for all articles first
        for i, article in enumerate(articles):
            try:
                article_id = article["id"]
                title = article["title"]
                content = article["content"]

                tags = self.tag_extractor.extract_tags(title, content)

                if tags:  # Only include articles that have tags
                    article_tags_batch.append({"article_id": article_id, "tags": tags})

                # Log progress during tag extraction
                if (i + 1) % self.config.progress_log_interval == 0:
                    logger.info(
                        f"Extracted tags for {i + 1}/{len(articles)} articles..."
                    )

                # Periodic memory cleanup during batch processing
                if (i + 1) % self.config.memory_cleanup_interval == 0:
                    self._cleanup_memory()

            except Exception as e:
                logger.error(
                    f"Error extracting tags for article {article.get('id', 'unknown')}: {e}"
                )
                batch_stats["failed"] += 1
                continue

        # Perform batch upsert of all tags in the current transaction
        if article_tags_batch:
            try:
                logger.info(
                    f"Upserting tags for {len(article_tags_batch)} articles in current transaction..."
                )

                # Use the batch upsert method (transaction managed by caller)
                result = self.tag_inserter.batch_upsert_tags_no_commit(
                    conn, article_tags_batch
                )

                batch_stats["successful"] = result.get("processed_articles", 0)
                batch_stats["failed"] += result.get("failed_articles", 0)
                batch_stats["total_processed"] = len(articles)

                if result.get("success"):
                    logger.info(
                        f"Successfully batch processed {batch_stats['successful']} articles"
                    )
                else:
                    logger.warning(
                        f"Batch processing completed with {batch_stats['failed']} failures"
                    )
                    # If batch processing failed, raise exception to trigger rollback
                    if batch_stats["failed"] > 0:
                        raise DatabaseConnectionError(
                            f"Batch processing failed for {batch_stats['failed']} articles"
                        )

            except Exception as e:
                logger.error(f"Batch upsert failed: {e}")
                batch_stats["failed"] = len(articles)
                batch_stats["total_processed"] = len(articles)
                # Re-raise to trigger transaction rollback at higher level
                raise
        else:
            logger.warning("No articles with tags to process")
            batch_stats["total_processed"] = len(articles)

        return batch_stats

    def _log_batch_summary(self, stats: Dict[str, Any]) -> None:
        """Log summary of batch processing results."""
        assert self.config is not None
        logger.info(
            f"Batch completed: {stats['total_processed']} total, "
            f"{stats['successful']} successful, {stats['failed']} failed"
        )

        if stats["failed"] > 0:
            failure_rate = (stats["failed"] / stats["total_processed"]) * 100
            logger.warning(f"Failure rate: {failure_rate:.1f}%")

    def run_processing_cycle(self) -> Dict[str, Any]:
        """
        Run a single processing cycle with explicit transaction management.

        Returns:
            Dictionary with cycle results
        """
        assert self.config is not None
        logger.info("Starting tag generation processing cycle")

        batch_stats = {
            "success": False,
            "total_processed": 0,
            "successful": 0,
            "failed": 0,
            "error": None,
        }

        try:
            # Use database connection (pooled or direct)
            logger.info("Acquiring database connection...")

            with self._get_database_connection() as conn:
                logger.info("Database connection acquired successfully")

                # Ensure connection starts in autocommit mode
                if not conn.autocommit:
                    conn.autocommit = True

                # Process articles batch (handles its own transaction)
                logger.info("Starting article batch processing...")
                processing_stats = self._process_article_batch(conn)

                # Update batch stats with processing results
                batch_stats.update(processing_stats)
                batch_stats["success"] = (
                    processing_stats.get("successful", 0) > 0
                    or processing_stats.get("total_processed", 0) == 0
                )

                logger.info("Article batch processing completed")
                self._log_batch_summary(batch_stats)
                return batch_stats

        except Exception as e:
            error_msg = f"Processing cycle failed: {e}"
            logger.error(error_msg)
            batch_stats["error"] = str(e)
            batch_stats["success"] = False
            return batch_stats

    def run_service(self) -> None:
        """Run the tag generation service continuously with health monitoring."""
        assert self.config is not None
        logger.info("Starting Tag Generation Service")
        logger.info("Service will run continuously. Press Ctrl+C to stop.")

        try:
            while True:
                self.total_cycles += 1
                logger.info(f"=== Processing Cycle {self.total_cycles} ===")

                # Run processing cycle
                result = self._run_processing_cycle_with_monitoring()

                if result.get("success", False):
                    articles_processed = result.get("successful", 0)
                    total_in_batch = result.get("total_processed", 0)

                    # Update health monitoring
                    if total_in_batch == 0:
                        self.consecutive_empty_cycles += 1
                    else:
                        self.consecutive_empty_cycles = 0
                        self.total_articles_processed += articles_processed

                    # Perform health check periodically
                    if (
                        self.total_cycles - self.last_health_check_cycle
                    ) >= self.config.health_check_interval:
                        self._perform_health_check()
                        self.last_health_check_cycle = self.total_cycles

                    logger.info(
                        f"Cycle {self.total_cycles} completed successfully. "
                        f"Processed: {articles_processed}/{total_in_batch} articles. "
                        f"Empty cycles: {self.consecutive_empty_cycles}. "
                        f"Sleeping for {self.config.processing_interval} seconds..."
                    )
                    time.sleep(self.config.processing_interval)
                else:
                    self.consecutive_empty_cycles += 1
                    logger.error(
                        f"Cycle {self.total_cycles} failed: {result.get('error', 'Unknown error')}. "
                        f"Failed: {result.get('failed', 0)}/{result.get('total_processed', 0)} articles. "
                        f"Retrying in {self.config.error_retry_interval} seconds..."
                    )
                    time.sleep(self.config.error_retry_interval)

        except KeyboardInterrupt:
            logger.info("Service stopped by user")
        except Exception as e:
            logger.error(f"Service crashed: {e}")
            raise
        finally:
            # Cleanup connection pool
            self._cleanup()

    def _run_processing_cycle_with_monitoring(self) -> Dict[str, Any]:
        """Run processing cycle with enhanced monitoring."""
        return self.run_processing_cycle()

    def _perform_health_check(self) -> None:
        """Perform health check and log service status."""
        assert self.config is not None
        logger.debug("=== HEALTH CHECK ===")
        logger.debug(f"Total cycles completed: {self.total_cycles}")
        logger.debug(f"Total articles processed: {self.total_articles_processed}")
        logger.debug(f"Consecutive empty cycles: {self.consecutive_empty_cycles}")
        logger.debug(
            f"Average articles per cycle: {self.total_articles_processed / max(1, self.total_cycles - self.consecutive_empty_cycles):.1f}"
        )

        # Warning for too many empty cycles
        if self.consecutive_empty_cycles >= self.config.max_consecutive_empty_cycles:
            logger.warning(
                f"⚠️  SERVICE HEALTH WARNING: {self.consecutive_empty_cycles} consecutive empty cycles detected!"
            )
            logger.warning(
                "This may indicate cursor poisoning, database issues, or no untagged articles available"
            )
            logger.warning(
                "Consider investigating database state or restarting service if issues persist"
            )

            # Try to get untagged article count for diagnosis
            try:
                with self._get_database_connection() as conn:
                    untagged_count = self.article_fetcher.count_untagged_articles(conn)
                    logger.info(
                        f"Diagnostic: {untagged_count} untagged articles found in database"
                    )
            except Exception as e:
                logger.error(
                    f"Failed to get untagged article count for diagnostics: {e}"
                )

        logger.debug("Health check finished")

    def _cleanup(self) -> None:
        """Cleanup resources."""
        logger.info("Service cleanup completed")


def main():
    """Main entry point for the tag generation service."""

    logger.info("Hello from tag-generator!")

    try:
        # Create and configure service
        config = TagGeneratorConfig()
        service = TagGeneratorService(config)

        # Run service
        service.run_service()

    except Exception as e:
        logger.error("Failed to start service", error=e)
        return 1

    return 0


if __name__ == "__main__":
    exit(main())