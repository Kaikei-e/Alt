"""Main service class for tag generation operations."""

from typing import Any

import structlog

from article_fetcher.fetch import ArticleFetcher
from tag_extractor.extract import TagExtractor
from tag_generator.batch_processor import BatchProcessor
from tag_generator.cascade import CascadeController
from tag_generator.config import TagGeneratorConfig
from tag_generator.cursor_manager import CursorManager
from tag_generator.database import DatabaseManager
from tag_generator.health_monitor import HealthMonitor
from tag_generator.scheduler import ProcessingScheduler
from tag_inserter.upsert_tags import TagInserter

logger = structlog.get_logger(__name__)


class TagGeneratorService:
    """Main service class for tag generation operations."""

    def __init__(self, config: TagGeneratorConfig | None = None):
        """Initialize tag generator service with configuration."""
        self.config = config or TagGeneratorConfig()

        # Initialize dependencies
        self.article_fetcher = ArticleFetcher()
        self.tag_extractor = TagExtractor()
        self.tag_inserter = TagInserter()
        self.cascade_controller = CascadeController()

        # Initialize managers
        self.database_manager = DatabaseManager(self.config)
        self.cursor_manager = CursorManager(self.database_manager)
        self.batch_processor = BatchProcessor(
            self.config,
            self.article_fetcher,
            self.tag_extractor,
            self.tag_inserter,
            self.cascade_controller,
            self.cursor_manager,
        )
        self.health_monitor = HealthMonitor(self.config, self.database_manager, self.article_fetcher)

        logger.info("Tag Generator Service initialized")
        logger.info(f"Configuration: {self.config}")

    def _log_batch_summary(self, stats: dict[str, Any]) -> None:
        """Log summary of batch processing results."""
        logger.info(
            f"Batch completed: {stats['total_processed']} total, "
            f"{stats['successful']} successful, {stats['failed']} failed"
        )

        if stats["failed"] > 0:
            failure_rate = (stats["failed"] / stats["total_processed"]) * 100
            logger.warning(f"Failure rate: {failure_rate:.1f}%")

    def run_processing_cycle(self) -> dict[str, Any]:
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
            "error": None,
        }

        try:
            # Use database connection
            logger.info("Acquiring database connection...")

            with self.database_manager.get_connection() as conn:
                logger.info("Database connection acquired successfully")

                # Ensure connection starts in autocommit mode
                if not conn.autocommit:
                    conn.autocommit = True

                # Process articles batch (handles its own transaction)
                logger.info("Starting article batch processing...")
                processing_stats = self.batch_processor.process_article_batch(conn, self.cursor_manager)

                # Update batch stats with processing results
                batch_stats.update(processing_stats)
                batch_stats["success"] = (
                    processing_stats.get("successful", 0) > 0 or processing_stats.get("total_processed", 0) == 0
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
        logger.info("Starting Tag Generation Service")
        logger.info("Service will run continuously. Press Ctrl+C to stop.")

        scheduler = ProcessingScheduler(self)
        scheduler.run_forever()

    def _run_processing_cycle_with_monitoring(self) -> dict[str, Any]:
        """Run processing cycle with enhanced monitoring."""
        return self.run_processing_cycle()

    def _cleanup(self) -> None:
        """Cleanup resources."""
        logger.info("Service cleanup completed")
