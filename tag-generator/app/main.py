import gc
import logging
import os
import time
from datetime import datetime, UTC
from typing import Optional, Dict, Any
from dataclasses import dataclass

import psycopg2
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
    batch_limit: int = 100  # articles per processing cycle
    progress_log_interval: int = 10  # log progress every N articles
    enable_gc_collection: bool = True  # enable manual garbage collection
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

        logger.debug("Database connection string prepared")
        return dsn

    def _create_database_connection(self) -> Connection:
        """Create database connection with retry logic."""
        dsn = self._get_database_dsn()

        for attempt in range(self.config.max_connection_retries):
            try:
                logger.info(f"Attempting database connection (attempt {attempt + 1}/{self.config.max_connection_retries})")
                conn = psycopg2.connect(dsn)
                logger.info("Database connected successfully")
                return conn

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
        last_created_at = datetime.now(UTC).isoformat()
        last_id = "ffffffff-ffff-ffff-ffff-ffffffffffff"  # Max UUID for descending order

        logger.info(f"Starting article processing from {last_created_at}")
        return last_created_at, last_id

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
            logger.debug(f"Processing article {article_id}")

            # Extract tags
            tags = self.tag_extractor.extract_tags(title, content)
            logger.debug(f"Extracted {len(tags)} tags for article {article_id}: {tags}")

            # Insert tags
            result = self.tag_inserter.upsert_tags(conn, article_id, tags)

            if result.get("success"):
                logger.debug(f"Successfully processed article {article_id} with {result.get('tags_processed', 0)} tags")
                return True
            else:
                logger.warning(f"Tag insertion reported failure for article {article_id}")
                return False

        except Exception as e:
            logger.error(f"Error processing article {article_id}: {e}")
            return False

    def _process_article_batch(self, conn: Connection) -> Dict[str, Any]:
        """
        Process a batch of articles for tag generation.

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

        while batch_stats["total_processed"] < self.config.batch_limit:
            try:
                # Fetch articles
                logger.debug("Fetching articles from database...")
                articles = self.article_fetcher.fetch_articles(conn, last_created_at, last_id)

                if not articles:
                    if batch_stats["total_processed"] == 0:
                        logger.info("No new articles found to process")
                    else:
                        logger.info(f"Finished processing batch of {batch_stats['total_processed']} articles")
                    break

                logger.info(f"Fetched {len(articles)} articles")

                # Process each article
                for article in articles:
                    if batch_stats["total_processed"] >= self.config.batch_limit:
                        logger.info(f"Reached batch limit of {self.config.batch_limit} articles")
                        break

                    success = self._process_single_article(conn, article)

                    if success:
                        batch_stats["successful"] += 1
                    else:
                        batch_stats["failed"] += 1

                    batch_stats["total_processed"] += 1

                    # Update cursor position
                    last_created_at = article["created_at"]
                    last_id = article["id"]
                    batch_stats["last_created_at"] = last_created_at
                    batch_stats["last_id"] = last_id

                    # Log progress
                    if batch_stats["total_processed"] % self.config.progress_log_interval == 0:
                        logger.info(f"Processed {batch_stats['total_processed']} articles...")

                        # Optional garbage collection
                        if self.config.enable_gc_collection:
                            gc.collect()

                # Break if we've reached the batch limit
                if batch_stats["total_processed"] >= self.config.batch_limit:
                    break

            except Exception as e:
                logger.error(f"Error during batch processing: {e}")
                break

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
        Run a single processing cycle.

        Returns:
            Dictionary with cycle results
        """
        logger.info("Starting tag generation processing cycle")

        conn = None
        try:
            # Create database connection
            conn = self._create_database_connection()

            # Process articles batch
            batch_stats = self._process_article_batch(conn)

            # Log summary
            self._log_batch_summary(batch_stats)

            return {
                "success": True,
                "batch_stats": batch_stats
            }

        except Exception as e:
            logger.error(f"Processing cycle failed: {e}")
            return {
                "success": False,
                "error": str(e)
            }

        finally:
            if conn:
                logger.debug("Closing database connection")
                conn.close()

            # Final garbage collection
            if self.config.enable_gc_collection:
                gc.collect()

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

                if result["success"]:
                    logger.info(
                        f"Cycle {cycle_count} completed successfully. "
                        f"Sleeping for {self.config.processing_interval} seconds..."
                    )
                    time.sleep(self.config.processing_interval)
                else:
                    logger.error(
                        f"Cycle {cycle_count} failed: {result.get('error', 'Unknown error')}. "
                        f"Retrying in {self.config.error_retry_interval} seconds..."
                    )
                    time.sleep(self.config.error_retry_interval)

        except KeyboardInterrupt:
            logger.info("Service stopped by user")
        except Exception as e:
            logger.error(f"Service crashed: {e}")
            raise

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