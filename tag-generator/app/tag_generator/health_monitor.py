"""Health monitoring for tag generator service."""

from typing import TYPE_CHECKING, Any

import structlog

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from tag_generator.config import TagGeneratorConfig


class HealthMonitor:
    """Monitors health of the tag generator service."""

    def __init__(
        self,
        config: "TagGeneratorConfig",
        database_manager: Any,
        article_fetcher: Any,
    ):
        """Initialize health monitor with dependencies."""
        self.config = config
        self.database_manager = database_manager
        self.article_fetcher = article_fetcher

        # Health monitoring state
        self.consecutive_empty_cycles = 0
        self.total_cycles = 0
        self.total_articles_processed = 0
        self.last_health_check_cycle: int = 0

    def record_cycle(self, articles_processed: int) -> None:
        """Record a processing cycle."""
        self.total_cycles += 1
        if articles_processed == 0:
            self.consecutive_empty_cycles += 1
        else:
            self.consecutive_empty_cycles = 0
            self.total_articles_processed += articles_processed

    def perform_health_check(self) -> None:
        """Perform health check and log service status."""
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
            logger.warning("This may indicate cursor poisoning, database issues, or no untagged articles available")
            logger.warning("Consider investigating database state or restarting service if issues persist")

            # Try to get untagged article count for diagnosis
            try:
                with self.database_manager.get_connection() as conn:
                    untagged_count = self.article_fetcher.count_untagged_articles(conn)
                    logger.info(f"Diagnostic: {untagged_count} untagged articles found in database")
            except Exception as e:
                logger.error(f"Failed to get untagged article count for diagnostics: {e}")

        logger.debug("Health check finished")

    def should_perform_health_check(self) -> bool:
        """Check if health check should be performed."""
        return (self.total_cycles - self.last_health_check_cycle) >= self.config.health_check_interval

    def mark_health_check_completed(self) -> None:
        """Mark health check as completed."""
        self.last_health_check_cycle = self.total_cycles
