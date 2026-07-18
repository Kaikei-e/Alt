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
        config: TagGeneratorConfig,
        article_fetcher: Any,
    ):
        """Initialize health monitor with dependencies."""
        self.config = config
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
        avg_divisor = max(1, self.total_cycles - self.consecutive_empty_cycles)
        avg_articles = self.total_articles_processed / avg_divisor
        logger.debug(
            "health_check",
            total_cycles=self.total_cycles,
            total_articles_processed=self.total_articles_processed,
            consecutive_empty_cycles=self.consecutive_empty_cycles,
            average_articles_per_cycle=round(avg_articles, 1),
        )

        # Warning for too many empty cycles
        if self.consecutive_empty_cycles >= self.config.max_consecutive_empty_cycles:
            logger.warning(
                "service_health_warning",
                consecutive_empty_cycles=self.consecutive_empty_cycles,
                hint="cursor poisoning, API issues, or no untagged articles",
            )

            # Try to get untagged article count for diagnosis via API
            try:
                untagged_count = self.article_fetcher.count_untagged_articles(None)
                logger.info("health_diagnostic", untagged_count=untagged_count)
            except Exception as e:
                logger.error(
                    "health_diagnostic_failed",
                    error=str(e),
                    exc_info=True,
                )

        logger.debug("health_check_finished")

    def should_perform_health_check(self) -> bool:
        """Check if health check should be performed."""
        return (self.total_cycles - self.last_health_check_cycle) >= self.config.health_check_interval

    def mark_health_check_completed(self) -> None:
        """Mark health check as completed."""
        self.last_health_check_cycle = self.total_cycles
