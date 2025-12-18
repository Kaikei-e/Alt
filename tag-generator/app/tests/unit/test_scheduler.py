from unittest.mock import Mock, patch

from main import TagGeneratorService
from tag_generator.scheduler import ProcessingScheduler


class TestProcessingScheduler:
    def test_calculate_next_sleep_immediate_after_work(self):
        service = TagGeneratorService()
        scheduler = ProcessingScheduler(service, sleep_fn=Mock())
        stats = {"total_processed": 3, "successful": 3, "failed": 0, "has_more_pending": False}

        assert scheduler.calculate_next_sleep(stats) == service.config.active_processing_interval

    def test_calculate_next_sleep_respects_interval_when_idle(self):
        service = TagGeneratorService()
        scheduler = ProcessingScheduler(service, sleep_fn=Mock())
        stats = {"total_processed": 0, "successful": 0, "failed": 0, "has_more_pending": False}

        assert scheduler.calculate_next_sleep(stats) == service.config.processing_interval

    def test_calculate_next_sleep_immediate_when_more_pending(self):
        service = TagGeneratorService()
        scheduler = ProcessingScheduler(service, sleep_fn=Mock())
        stats = {"total_processed": 0, "successful": 0, "failed": 0, "has_more_pending": True}

        assert scheduler.calculate_next_sleep(stats) == service.config.active_processing_interval

    def test_run_cycle_updates_health_and_returns_interval(self):
        service = TagGeneratorService()
        scheduler = ProcessingScheduler(service, sleep_fn=Mock())
        stats = {"success": True, "successful": 0, "total_processed": 0, "failed": 0}

        with patch.object(service, "_run_processing_cycle_with_monitoring", return_value=stats) as mock_cycle:
            sleep_interval = scheduler.run_cycle()

        mock_cycle.assert_called_once()
        assert sleep_interval == service.config.processing_interval
        assert service.health_monitor.total_cycles == 1
        assert service.health_monitor.consecutive_empty_cycles == 1

    def test_run_cycle_resets_empty_counter_on_work(self):
        service = TagGeneratorService()
        service.health_monitor.consecutive_empty_cycles = 3
        scheduler = ProcessingScheduler(service, sleep_fn=Mock())
        stats = {"success": True, "successful": 2, "total_processed": 2, "failed": 0}

        with patch.object(service, "_run_processing_cycle_with_monitoring", return_value=stats):
            sleep_interval = scheduler.run_cycle()

        assert sleep_interval == service.config.active_processing_interval
        assert service.health_monitor.consecutive_empty_cycles == 0
        assert service.health_monitor.total_articles_processed == 2

    def test_run_cycle_handles_failures_with_retry_delay(self):
        service = TagGeneratorService()
        scheduler = ProcessingScheduler(service, sleep_fn=Mock())
        stats = {"success": False, "successful": 0, "total_processed": 0, "failed": 1, "error": "boom"}

        with patch.object(service, "_run_processing_cycle_with_monitoring", return_value=stats):
            sleep_interval = scheduler.run_cycle()

        assert sleep_interval == service.config.error_retry_interval
        assert service.health_monitor.consecutive_empty_cycles == 1
