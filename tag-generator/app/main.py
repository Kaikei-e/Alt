import gc
import logging
import os
import time
from datetime import datetime, UTC
from typing import Optional, Dict, Any, List, Union
from dataclasses import dataclass
from contextlib import contextmanager

import psycopg2
import psycopg2.extensions
from psycopg2.extras import DictCursor
from psycopg2.extensions import connection as Connection

from article_fetcher.fetch import ArticleFetcher
from tag_extractor.extract import TagExtractor
from tag_inserter.upsert_tags import TagInserter

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

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

        # Using direct database connections (connection pool disabled due to hanging issues)
        self._connection_pool = None

        logger.info("Tag Generator Service initialized")
        logger.info(f"Configuration: {self.config}")

    def _get_database_dsn(self) -> str:
        """Build database connection string from environment variables."""
        required_vars = [
            'DB_TAG_GENERATOR_USER',
            'DB_TAG_GENERATOR_PASSWORD',
            'DB_HOST',
            'DB_PORT',
            'DB_NAME'
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
        dsn = self._get_database_dsn()

        for attempt in range(self.config.max_connection_retries):
            try:
                logger.info(f"Attempting database connection (attempt {attempt + 1}/{self.config.max_connection_retries})")
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
                    logger.info(f"Retrying in {self.config.connection_retry_delay} seconds...")
                    time.sleep(self.config.connection_retry_delay)
                else:
                    raise DatabaseConnectionError(f"Failed to connect after {self.config.max_connection_retries} attempts") from e

        # This should never be reached due to the exception above, but needed for type checking
        raise DatabaseConnectionError("Database connection failed")

    def _get_initial_cursor_position(self) -> tuple[str, str]:
        """Get initial cursor position for pagination."""
        if self.last_processed_created_at and self.last_processed_id:
            # Continue from where we left off
            last_created_at = self.last_processed_created_at
            last_id = self.last_processed_id
            logger.info(f"Continuing article processing from cursor: {last_created_at}, ID: {last_id}")
        else:
            # First run - start from current time
            last_created_at = datetime.now(UTC).isoformat()
            last_id = "ffffffff-ffff-ffff-ffff-ffffffffffff"  # Max UUID for descending order
            logger.info(f"Starting initial article processing from {last_created_at}")

        return last_created_at, last_id

    def _cleanup_memory(self) -> None:
        """Explicit memory cleanup to prevent accumulation."""
        if self.config.enable_gc_collection:
            gc.collect()

    def _process_single_article(
        self,
        conn: Connection,
        article: Dict[str, Any]
    ) -> bool:
        """
        Process a single article for tag extraction and insertion.

        Args:
            conn: Database connection
            article: Article dictionary with id, title, content, created_at

        Returns:
            True if successful, False otherwise
        """
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
                logger.warning(f"Tag insertion reported failure for article {article_id}")
                return False

        except Exception as e:
            logger.error(f"Error processing article {article_id}: {e}")
            return False

    def _process_article_batch(self, conn: Connection) -> Dict[str, Any]:
        """
        Process a batch of articles for tag generation using true batch processing.

        Args:
            conn: Database connection

        Returns:
            Dictionary with batch processing results
        """
        last_created_at, last_id = self._get_initial_cursor_position()

        batch_stats = {
            "total_processed": 0,
            "successful": 0,
            "failed": 0,
            "last_created_at": last_created_at,
            "last_id": last_id
        }

        # Collect articles for batch processing (keep autocommit for fetching)
        articles_to_process = []

        while len(articles_to_process) < self.config.batch_limit:
            try:
                # Fetch articles
                articles = self.article_fetcher.fetch_articles(conn, last_created_at, last_id)

                if not articles:
                    logger.info(f"No more articles found. Collected {len(articles_to_process)} articles for batch processing")
                    break

                logger.info(f"Fetched {len(articles)} articles")

                # Add articles to batch, respecting the batch limit
                for article in articles:
                    if len(articles_to_process) >= self.config.batch_limit:
                        logger.info(f"Reached batch limit of {self.config.batch_limit} articles")
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
                break

        # Start explicit transaction for batch processing only
        try:
            if conn.autocommit:
                conn.autocommit = False

            if articles_to_process:
                logger.info(f"Processing batch of {len(articles_to_process)} articles...")
                batch_stats = self._process_articles_as_batch(conn, articles_to_process)
                # Ensure string format for batch stats
                if isinstance(last_created_at, str):
                    batch_stats["last_created_at"] = last_created_at
                else:
                    batch_stats["last_created_at"] = last_created_at.isoformat()
                batch_stats["last_id"] = last_id

                # Update persistent cursor position for next cycle (ensure string format)
                if isinstance(last_created_at, str):
                    self.last_processed_created_at = last_created_at
                else:
                    self.last_processed_created_at = last_created_at.isoformat()
                self.last_processed_id = last_id
                logger.info(f"Updated cursor position for next cycle: {self.last_processed_created_at}, ID: {last_id}")

                # Commit the transaction only if batch processing was successful
                if batch_stats.get("successful", 0) > 0:
                    conn.commit()
                else:
                    conn.rollback()
                    logger.warning("Transaction rolled back due to batch processing failure")
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

    def _process_articles_as_batch(self, conn: Connection, articles: List[Dict[str, Any]]) -> Dict[str, Any]:
        """
        Process multiple articles as a single batch transaction.
        Note: Transaction management is handled by the caller.

        Args:
            conn: Database connection (should already be in transaction mode)
            articles: List of articles to process

        Returns:
            Dictionary with batch processing results
        """
        batch_stats = {
            "total_processed": 0,
            "successful": 0,
            "failed": 0
        }

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
                    article_tags_batch.append({
                        "article_id": article_id,
                        "tags": tags
                    })

                # Log progress during tag extraction
                if (i + 1) % self.config.progress_log_interval == 0:
                    logger.info(f"Extracted tags for {i + 1}/{len(articles)} articles...")

                # Periodic memory cleanup during batch processing
                if (i + 1) % self.config.memory_cleanup_interval == 0:
                    self._cleanup_memory()

            except Exception as e:
                logger.error(f"Error extracting tags for article {article.get('id', 'unknown')}: {e}")
                batch_stats["failed"] += 1
                continue

        # Perform batch upsert of all tags in the current transaction
        if article_tags_batch:
            try:
                logger.info(f"Upserting tags for {len(article_tags_batch)} articles in current transaction...")

                # Use the batch upsert method (transaction managed by caller)
                result = self.tag_inserter.batch_upsert_tags_no_commit(conn, article_tags_batch)

                batch_stats["successful"] = result.get("processed_articles", 0)
                batch_stats["failed"] += result.get("failed_articles", 0)
                batch_stats["total_processed"] = len(articles)

                if result.get("success"):
                    logger.info(f"Successfully batch processed {batch_stats['successful']} articles")
                else:
                    logger.warning(f"Batch processing completed with {batch_stats['failed']} failures")
                    # If batch processing failed, raise exception to trigger rollback
                    if batch_stats["failed"] > 0:
                        raise DatabaseConnectionError(f"Batch processing failed for {batch_stats['failed']} articles")

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
        logger.info(
            f"Batch completed: {stats['total_processed']} total, "
            f"{stats['successful']} successful, {stats['failed']} failed"
        )

        if stats['failed'] > 0:
            failure_rate = (stats['failed'] / stats['total_processed']) * 100
            logger.warning(f"Failure rate: {failure_rate:.1f}%")

    def run_processing_cycle(self) -> Dict[str, Any]:
        """
        Run a single processing cycle with explicit transaction management.

        Returns:
            Dictionary with cycle results
        """
        logger.info("Starting tag generation processing cycle")

        batch_stats = {
            "success": False,
            "total_processed": 0,
            "successful": 0,
            "failed": 0,
            "error": None
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
                batch_stats["success"] = processing_stats.get("successful", 0) > 0 or processing_stats.get("total_processed", 0) == 0

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
        """Run the tag generation service continuously."""
        logger.info("Starting Tag Generation Service")
        logger.info("Service will run continuously. Press Ctrl+C to stop.")

        cycle_count = 0

        try:
            while True:
                cycle_count += 1
                logger.info(f"=== Processing Cycle {cycle_count} ===")

                # Run processing cycle
                result = self.run_processing_cycle()

                if result.get("success", False):
                    logger.info(
                        f"Cycle {cycle_count} completed successfully. "
                        f"Processed: {result.get('successful', 0)}/{result.get('total_processed', 0)} articles. "
                        f"Sleeping for {self.config.processing_interval} seconds..."
                    )
                    time.sleep(self.config.processing_interval)
                else:
                    logger.error(
                        f"Cycle {cycle_count} failed: {result.get('error', 'Unknown error')}. "
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
        logger.error(f"Failed to start service: {e}")
        return 1

    return 0

if __name__ == "__main__":
    exit(main())