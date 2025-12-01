import time
from collections.abc import Callable
from typing import TYPE_CHECKING, Any

import structlog

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from main import TagGeneratorService


class ProcessingScheduler:
    """Coordinate the recurring processing loop for tag generation."""

    def __init__(self, service: "TagGeneratorService", sleep_fn: Callable[[float], None] | None = None):
        self.service = service
        self._sleep = sleep_fn or time.sleep

    def calculate_next_sleep(self, processing_stats: dict[str, Any]) -> int:
        """Derive the delay before the next cycle based on recent work."""
        assert self.service.config is not None

        if processing_stats.get("has_more_pending"):
            return 0

        if processing_stats.get("total_processed", 0) > 0:
            return 0

        return self.service.config.processing_interval

    def run_cycle(self) -> int:
        """Run a single processing cycle and return the sleep interval."""
        service = self.service
        assert service.config is not None

        service.total_cycles += 1
        logger.info(f"=== Processing Cycle {service.total_cycles} ===")

        result = service._run_processing_cycle_with_monitoring()

        if result.get("success", False):
            articles_processed = result.get("successful", 0)
            total_in_batch = result.get("total_processed", 0)

            if total_in_batch == 0:
                service.consecutive_empty_cycles += 1
            else:
                service.consecutive_empty_cycles = 0
                service.total_articles_processed += articles_processed

            if (service.total_cycles - service.last_health_check_cycle) >= service.config.health_check_interval:
                service._perform_health_check()
                service.last_health_check_cycle = service.total_cycles

            sleep_interval = self.calculate_next_sleep(result)
            logger.info(
                f"Cycle {service.total_cycles} completed successfully. "
                f"Processed: {articles_processed}/{total_in_batch} articles. "
                f"Empty cycles: {service.consecutive_empty_cycles}. "
                f"Sleeping for {sleep_interval} seconds..."
            )
            return sleep_interval

        service.consecutive_empty_cycles += 1
        logger.error(
            f"Cycle {service.total_cycles} failed: {result.get('error', 'Unknown error')}. "
            f"Failed: {result.get('failed', 0)}/{result.get('total_processed', 0)} articles. "
            f"Retrying in {service.config.error_retry_interval} seconds..."
        )
        return service.config.error_retry_interval

    def run_forever(self) -> None:
        """Continuously run processing cycles with appropriate delays."""
        try:
            while True:
                sleep_interval = self.run_cycle()
                self._sleep(sleep_interval)
        except KeyboardInterrupt:
            logger.info("Service stopped by user")
        except Exception as e:
            logger.error(f"Service crashed: {e}")
            raise
        finally:
            self.service._cleanup()
