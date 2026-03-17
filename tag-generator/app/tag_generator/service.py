"""Main service class for tag generation operations."""

from __future__ import annotations

from typing import Any

import structlog

from tag_extractor.extract import TagExtractionOutcome, TagExtractor
from tag_generator.batch_processor import BatchProcessor
from tag_generator.cascade import CascadeController
from tag_generator.config import TagGeneratorConfig
from tag_generator.cursor_manager import CursorManager
from tag_generator.domain.models import TagExtractionResult
from tag_generator.driver.connect_client_factory import create_backend_client
from tag_generator.exceptions import TagExtractionError
from tag_generator.health_monitor import HealthMonitor
from tag_generator.scheduler import ProcessingScheduler

logger = structlog.get_logger(__name__)


class TagGeneratorService:
    """Main service class for tag generation operations."""

    def __init__(self, config: TagGeneratorConfig | None = None):
        """Initialize tag generator service with configuration."""
        self.config = config or TagGeneratorConfig()

        # Pluggable drivers — always use Connect-RPC client.
        self.article_fetcher: Any
        self.tag_inserter: Any

        result = create_backend_client()

        if result is not None:
            client, auth_headers = result
            from tag_generator.driver.connect_article_fetcher import ConnectArticleFetcher
            from tag_generator.driver.connect_tag_inserter import ConnectTagInserter

            logger.info("Using backend API mode for article/tag operations")
            self.article_fetcher = ConnectArticleFetcher(client, auth_headers)
            self.tag_inserter = ConnectTagInserter(client, auth_headers)
        else:
            raise RuntimeError(
                "Backend API client could not be created. "
                "Ensure BACKEND_API_URL and SERVICE_SECRET environment variables are set. "
                "Legacy database mode has been removed."
            )

        # Initialize dependencies (shared across modes)
        self.tag_extractor = TagExtractor()
        self.tag_extractor.warmup()
        self.cascade_controller = CascadeController()

        # Initialize managers
        self.cursor_manager = CursorManager()
        self.batch_processor = BatchProcessor(
            self.config,
            self.article_fetcher,
            self.tag_extractor,
            self.tag_inserter,
            self.cascade_controller,
            self.cursor_manager,
        )
        self.health_monitor = HealthMonitor(self.config, self.article_fetcher)

        logger.info("Tag Generator Service initialized")
        logger.info("Tag Generator Service configured", config=str(self.config))

    # ------------------------------------------------------------------
    # Article processing
    # ------------------------------------------------------------------

    def _process_single_article(self, conn: Any, article: dict[str, Any]) -> bool:
        """Process a single article for tag extraction and insertion.

        Used by StreamEventHandler for real-time event-driven tag generation.
        """
        article_id = article["id"]
        title = article.get("title", "")
        content = article.get("content", "")
        feed_id = article.get("feed_id")
        if feed_id is None:
            logger.warning("Article missing feed_id", article_id=article_id)
            feed_id = ""

        try:
            outcome: TagExtractionOutcome = self.tag_extractor.extract_tags_with_metrics(title, content)
        except (TagExtractionError, Exception) as exc:  # pragma: no cover - failure path tested via mocks
            logger.error("Tag extraction failed for article", article_id=article_id, error=str(exc))
            return False

        extraction_result = TagExtractionResult.from_outcome(article_id, outcome)

        if extraction_result.is_empty:
            logger.info("No tags extracted for article", article_id=article_id)
            return False

        try:
            result = self.tag_inserter.upsert_tags(conn, article_id, extraction_result.tag_names, feed_id)
            success = bool(result.get("success"))
            if not success:
                logger.warning("Tag upsert failed for article", article_id=article_id)
            return success
        except Exception as exc:  # pragma: no cover - failure path tested via mocks
            logger.error("Tag upsert raised exception for article", article_id=article_id, error=str(exc))
            return False

    # ------------------------------------------------------------------
    # Public service API
    # ------------------------------------------------------------------

    def _log_batch_summary(self, stats: dict[str, Any]) -> None:
        """Log summary of batch processing results."""
        logger.info(
            "Batch completed",
            total_processed=stats["total_processed"],
            successful=stats["successful"],
            failed=stats["failed"],
        )

        if stats["failed"] > 0:
            failure_rate = (stats["failed"] / stats["total_processed"]) * 100
            logger.warning("High failure rate", failure_rate=round(failure_rate, 1))

    def run_processing_cycle(self) -> dict[str, Any]:
        """
        Run a single processing cycle.

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
            # Process articles batch (conn=None, API mode handles everything)
            logger.info("Starting article batch processing...")
            processing_stats = self.batch_processor.process_article_batch(None, self.cursor_manager)

            # Update batch stats with processing results
            batch_stats.update(processing_stats)
            # Success if:
            # - At least one article was successfully processed, OR
            # - No articles were processed (nothing to do), OR
            # - No articles failed (all were skipped or tags couldn't be extracted)
            batch_stats["success"] = (
                processing_stats.get("successful", 0) > 0
                or processing_stats.get("total_processed", 0) == 0
                or processing_stats.get("failed", 0) == 0
            )

            logger.info("Article batch processing completed")
            self._log_batch_summary(batch_stats)

            return batch_stats

        except Exception as e:
            logger.error("Processing cycle failed", error=str(e))
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
