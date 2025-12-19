"""Main service class for tag generation operations."""

from __future__ import annotations

import time
from collections.abc import Iterator
from contextlib import contextmanager
from datetime import UTC, datetime
from typing import Any

import psycopg2
import psycopg2.extensions
import structlog
from psycopg2.extensions import connection as Connection

from article_fetcher.fetch import ArticleFetcher
from tag_extractor.extract import TagExtractionOutcome, TagExtractor
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

        # Legacy cursor state kept on the service for backward-compatible tests.
        self.last_processed_created_at: str | None = None
        self.last_processed_id: str | None = None
        self.forward_cursor_created_at: str | None = None
        self.forward_cursor_id: str | None = None
        # Legacy backfill state flag for tests (BatchProcessor manages its own state in production).
        self.backfill_completed: bool = False

        logger.info("Tag Generator Service initialized")
        logger.info(f"Configuration: {self.config}")

    # ------------------------------------------------------------------
    # Database helpers (compatibility layer)
    # ------------------------------------------------------------------

    def _get_database_dsn(self) -> str:
        """Build database DSN using the DatabaseManager (for tests and logging)."""
        return self.database_manager.get_database_dsn()

    def _create_direct_connection(self) -> Connection:
        """Create direct database connection with retry logic and logging.

        This is a compatibility helper used by unit and integration tests.
        """
        dsn = self._get_database_dsn()

        for attempt in range(self.config.max_connection_retries):
            try:
                logger.info(
                    "Attempting direct database connection",
                    attempt=attempt + 1,
                    max_retries=self.config.max_connection_retries,
                )
                conn = psycopg2.connect(dsn)

                # Ensure connection starts in a clean state
                try:
                    if conn.status != psycopg2.extensions.STATUS_READY:
                        conn.rollback()

                    if not conn.autocommit:
                        conn.autocommit = True
                except Exception as setup_error:
                    logger.warning("Failed to setup direct connection state", error=str(setup_error))
                    try:
                        conn.close()
                    except Exception:
                        pass
                    raise

                logger.info("Direct database connection established successfully")
                return conn

            except psycopg2.Error as e:  # pragma: no cover - error path tested via mocks
                logger.error("Database connection failed", error=str(e))

                if attempt < self.config.max_connection_retries - 1:
                    logger.info(
                        "Retrying database connection",
                        delay=self.config.connection_retry_delay,
                    )
                    time.sleep(self.config.connection_retry_delay)
                else:
                    raise

        # Should never reach here
        raise psycopg2.Error("Failed to establish database connection")

    @contextmanager
    def _get_database_connection(self) -> Iterator[Connection]:
        """Context manager wrapper for DatabaseManager.get_connection (for tests)."""
        with self.database_manager.get_connection() as conn:
            yield conn

    # ------------------------------------------------------------------
    # Cursor helpers (compatibility layer)
    # ------------------------------------------------------------------

    def _get_initial_cursor_position(self) -> tuple[str, str]:
        """Get initial cursor position for backfill processing.

        This mirrors the behaviour of CursorManager.get_initial_cursor_position
        but operates on the service's legacy cursor attributes and does not
        require a database lookup, which keeps unit tests fast and deterministic.
        """
        if self.last_processed_created_at and self.last_processed_id:
            try:
                cursor_str = self.last_processed_created_at
                if cursor_str.endswith("Z"):
                    cursor_str = cursor_str.replace("Z", "+00:00")

                cursor_time = datetime.fromisoformat(cursor_str)
                if cursor_time.tzinfo is None:
                    cursor_time = cursor_time.replace(tzinfo=UTC)

                current_time = datetime.now(UTC)
                time_diff = cursor_time - current_time

                # Treat cursors in the far future or far past as poisoned.
                if time_diff.total_seconds() > 3600 or time_diff.total_seconds() < -86400 * 30:
                    # Recovery cursor: start from "now" with default UUID
                    recovery_time = datetime.now(UTC).isoformat()
                    return recovery_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

                # Valid stored cursor
                return self.last_processed_created_at, self.last_processed_id

            except (ValueError, TypeError):
                # Invalid format – fall back to recovery cursor
                recovery_time = datetime.now(UTC).isoformat()
                return recovery_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

        # First run – start from current time
        start_time = datetime.now(UTC).isoformat()
        return start_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

    def _get_forward_cursor_position(self, conn: Connection) -> tuple[str, str]:
        """Get forward cursor position using the cursor manager (compat wrapper)."""
        return self.cursor_manager.get_forward_cursor_position(conn)

    def _has_existing_tags(self, conn: Connection) -> bool:
        """Check whether any tags exist (compat wrapper around BatchProcessor)."""
        return self.batch_processor._has_existing_tags(conn)

    # ------------------------------------------------------------------
    # Article processing helpers (compatibility layer)
    # ------------------------------------------------------------------

    def _process_single_article(self, conn: Connection, article: dict[str, Any]) -> bool:
        """Process a single article for tag extraction and insertion."""
        article_id = article["id"]
        title = article.get("title", "")
        content = article.get("content", "")
        feed_id = article.get("feed_id")
        if feed_id is None:
            logger.warning("Article missing feed_id", article_id=article_id)
            feed_id = ""

        try:
            outcome: TagExtractionOutcome = self.tag_extractor.extract_tags_with_metrics(title, content)
        except Exception as exc:  # pragma: no cover - failure path tested via mocks
            logger.error("Tag extraction failed for article", article_id=article_id, error=str(exc))
            return False

        if not outcome.tags:
            logger.info("No tags extracted for article", article_id=article_id)
            return False

        try:
            result = self.tag_inserter.upsert_tags(conn, article_id, outcome.tags, feed_id)
            success = bool(result.get("success"))
            if not success:
                logger.warning("Tag upsert failed for article", article_id=article_id)
            return success
        except Exception as exc:  # pragma: no cover - failure path tested via mocks
            logger.error("Tag upsert raised exception for article", article_id=article_id, error=str(exc))
            return False

    def _process_articles_as_batch(self, conn: Connection, articles: list[dict[str, Any]]) -> dict[str, Any]:
        """Process multiple articles as a batch (delegates to BatchProcessor by default)."""
        return self.batch_processor.process_articles_as_batch(conn, articles)

    def _process_article_batch(self, conn: Connection) -> dict[str, Any]:
        """Process a batch of articles with simple forward/backfill behaviour for tests."""
        # Forward processing when existing tags are present
        if self._has_existing_tags(conn):
            start_created_at, start_id = self._get_forward_cursor_position(conn)
            articles = self.article_fetcher.fetch_new_articles(
                conn, start_created_at, start_id, self.config.batch_limit
            )

            if not articles:
                return {"total_processed": 0, "successful": 0, "failed": 0}

            # Start explicit transaction
            try:
                if conn.autocommit:
                    conn.autocommit = False
            except Exception:
                pass

            stats = self._process_articles_as_batch(conn, articles)

            last_article = articles[-1]
            last_created_at = (
                last_article["created_at"]
                if isinstance(last_article["created_at"], str)
                else last_article["created_at"].isoformat()
            )

            total_processed = stats.get("total_processed", len(articles))
            successful = stats.get("successful", 0)
            failed = stats.get("failed", 0)

            try:
                if successful > 0:
                    # Advance both forward and last-processed cursors
                    self.forward_cursor_created_at = last_created_at
                    self.forward_cursor_id = last_article["id"]
                    self.last_processed_created_at = last_created_at
                    self.last_processed_id = last_article["id"]
                    conn.commit()
                else:
                    conn.rollback()
                    logger.warning("Transaction rolled back due to forward batch failure")
            except Exception:
                pass
            finally:
                try:
                    if hasattr(conn, "autocommit") and not conn.autocommit:
                        conn.autocommit = True
                except Exception:
                    pass

            return {
                "total_processed": total_processed,
                "successful": successful,
                "failed": failed,
                "last_created_at": last_created_at,
                "last_id": last_article["id"],
            }

        # Backfill processing when no existing tags (or as default)
        start_created_at, start_id = self._get_initial_cursor_position()
        articles = self.article_fetcher.fetch_articles(conn, start_created_at, start_id)

        if not articles:
            return {
                "total_processed": 0,
                "successful": 0,
                "failed": 0,
                "last_created_at": start_created_at,
                "last_id": start_id,
            }

        try:
            if conn.autocommit:
                conn.autocommit = False
        except Exception:
            pass

        stats = self._process_articles_as_batch(conn, articles)

        last_article = articles[-1]
        newest_article = articles[0]

        last_created_at = (
            last_article["created_at"]
            if isinstance(last_article["created_at"], str)
            else last_article["created_at"].isoformat()
        )
        newest_created_at = (
            newest_article["created_at"]
            if isinstance(newest_article["created_at"], str)
            else newest_article["created_at"].isoformat()
        )

        total_processed = stats.get("total_processed", len(articles))
        successful = stats.get("successful", 0)
        failed = stats.get("failed", 0)

        try:
            if successful > 0:
                # Update legacy cursor state to reflect batch processing
                self.last_processed_created_at = last_created_at
                self.last_processed_id = last_article["id"]
                self.forward_cursor_created_at = newest_created_at
                self.forward_cursor_id = newest_article["id"]
                conn.commit()
            else:
                conn.rollback()
                logger.warning("Transaction rolled back due to backfill batch failure")
        except Exception:
            pass
        finally:
            try:
                if hasattr(conn, "autocommit") and not conn.autocommit:
                    conn.autocommit = True
            except Exception:
                pass

        return {
            "total_processed": total_processed,
            "successful": successful,
            "failed": failed,
            "last_created_at": last_created_at,
            "last_id": last_article["id"],
        }

    # ------------------------------------------------------------------
    # Public service API
    # ------------------------------------------------------------------

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
