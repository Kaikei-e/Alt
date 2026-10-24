"""Unit tests for TagGeneratorService, HealthMonitor, ProcessingScheduler health checks, and GET /health."""

from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient

from tag_extractor.extract import TagExtractionOutcome
from tag_generator.config import TagGeneratorConfig
from tag_generator.health_monitor import HealthMonitor
from tag_generator.scheduler import ProcessingScheduler
from tag_generator.service import TagGeneratorService


def _make_service(config: TagGeneratorConfig | None = None) -> TagGeneratorService:
    return TagGeneratorService(
        config=config or TagGeneratorConfig(),
        article_fetcher=MagicMock(),
        tag_inserter=MagicMock(),
    )


class TestTagGeneratorServiceCycleAndSingleArticle:
    """Tests for TagGeneratorService.run_processing_cycle and _process_single_article."""

    def test_run_processing_cycle_succeeds_when_articles_processed(self):
        service = _make_service()
        with patch.object(
            service.batch_processor,
            "process_article_batch",
            return_value={"total_processed": 5, "successful": 4, "failed": 1},
        ):
            stats = service.run_processing_cycle()

        assert stats["success"] is True
        assert stats["total_processed"] == 5
        assert stats["successful"] == 4
        assert stats["failed"] == 1
        assert stats["error"] is None

    def test_run_processing_cycle_succeeds_when_nothing_to_process(self):
        service = _make_service()
        with patch.object(
            service.batch_processor,
            "process_article_batch",
            return_value={"total_processed": 0, "successful": 0, "failed": 0},
        ):
            stats = service.run_processing_cycle()

        assert stats["success"] is True
        assert stats["total_processed"] == 0

    def test_run_processing_cycle_succeeds_when_zero_failed_even_if_zero_successful(self):
        """When all articles in batch had empty tag extraction (successful=0, failed=0), cycle is still successful."""
        service = _make_service()
        with patch.object(
            service.batch_processor,
            "process_article_batch",
            return_value={"total_processed": 3, "successful": 0, "failed": 0},
        ):
            stats = service.run_processing_cycle()

        assert stats["success"] is True

    def test_run_processing_cycle_marks_failure_when_all_processed_articles_failed(self):
        service = _make_service()
        with patch.object(
            service.batch_processor,
            "process_article_batch",
            return_value={"total_processed": 2, "successful": 0, "failed": 2},
        ):
            stats = service.run_processing_cycle()

        assert stats["success"] is False
        assert stats["failed"] == 2

    def test_run_processing_cycle_catches_exception_and_returns_error_dict(self):
        service = _make_service()
        with patch.object(
            service.batch_processor,
            "process_article_batch",
            side_effect=RuntimeError("datahub unavailable"),
        ):
            stats = service.run_processing_cycle()

        assert stats["success"] is False
        assert stats["error"] == "datahub unavailable"

    def test_process_single_article_returns_false_without_upsert_when_extraction_empty(self):
        service = _make_service()
        empty_outcome = TagExtractionOutcome(
            tags=[],
            confidence=0.0,
            tag_count=0,
            inference_ms=2.0,
            language="und",
            model_name="test-model",
            sanitized_length=10,
        )
        article = {"id": "art-empty", "title": "Short", "content": "Too short", "feed_id": None}

        with patch.object(service.tag_extractor, "extract_tags_with_metrics", return_value=empty_outcome):
            result = service._process_single_article(None, article)

        assert result is False
        service.tag_inserter.upsert_tags.assert_not_called()  # type: ignore[attr-defined]

    def test_process_single_article_returns_false_when_upsert_reports_failure(self):
        service = _make_service()
        outcome = TagExtractionOutcome(
            tags=["ml"],
            confidence=0.8,
            tag_count=1,
            inference_ms=5.0,
            language="en",
            model_name="test-model",
            sanitized_length=80,
            tag_confidences={"ml": 0.8},
        )
        article = {"id": "art-1", "title": "Title", "content": "Content", "feed_id": None}
        service.tag_inserter.upsert_tags.return_value = {"success": False}  # type: ignore[attr-defined]

        with patch.object(service.tag_extractor, "extract_tags_with_metrics", return_value=outcome):
            result = service._process_single_article(None, article)

        assert result is False
        service.tag_inserter.upsert_tags.assert_called_once_with(None, "art-1", ["ml"], "")  # type: ignore[attr-defined]


class TestHealthMonitorAndSchedulerIntegration:
    """Tests for HealthMonitor diagnostics and ProcessingScheduler health check triggering."""

    def test_perform_health_check_queries_untagged_count_when_empty_cycles_reach_threshold(self):
        config = TagGeneratorConfig(max_consecutive_empty_cycles=3, health_check_interval=2)
        fetcher = MagicMock()
        fetcher.count_untagged_articles.return_value = 42
        monitor = HealthMonitor(config, fetcher)

        for _ in range(3):
            monitor.record_cycle(0)

        monitor.perform_health_check()

        fetcher.count_untagged_articles.assert_called_once_with(None)

    def test_perform_health_check_swallows_diagnostic_fetcher_exception(self):
        config = TagGeneratorConfig(max_consecutive_empty_cycles=2)
        fetcher = MagicMock()
        fetcher.count_untagged_articles.side_effect = RuntimeError("rpc error")
        monitor = HealthMonitor(config, fetcher)

        monitor.record_cycle(0)
        monitor.record_cycle(0)

        # Should not raise when count_untagged_articles fails
        monitor.perform_health_check()
        fetcher.count_untagged_articles.assert_called_once_with(None)

    def test_scheduler_run_cycle_triggers_and_marks_health_check_at_interval(self):
        config = TagGeneratorConfig(health_check_interval=2, max_consecutive_empty_cycles=5)
        service = _make_service(config)
        scheduler = ProcessingScheduler(service, sleep_fn=MagicMock())
        cycle_result = {"success": True, "successful": 1, "total_processed": 1, "failed": 0}

        with (
            patch.object(service, "_run_processing_cycle_with_monitoring", return_value=cycle_result),
            patch.object(
                service.health_monitor, "perform_health_check", wraps=service.health_monitor.perform_health_check
            ) as spy_hc,
        ):
            # Cycle 1: total_cycles=1 < health_check_interval=2 -> no health check yet
            scheduler.run_cycle()
            assert spy_hc.call_count == 0
            assert service.health_monitor.last_health_check_cycle == 0

            # Cycle 2: total_cycles=2 >= health_check_interval=2 -> health check runs and marks completion
            scheduler.run_cycle()
            assert spy_hc.call_count == 1
            assert service.health_monitor.last_health_check_cycle == 2


class TestAuthServiceHealthEndpoint:
    """Tests for GET /health liveness reporting in auth_service."""

    @pytest.fixture()
    def health_client(self, monkeypatch: pytest.MonkeyPatch):
        import auth_service

        monkeypatch.setattr(auth_service, "_background_service_healthy", True)
        monkeypatch.setattr(auth_service, "_consumer_health", {})
        return TestClient(auth_service.app), auth_service

    def test_returns_503_when_background_batch_service_is_unhealthy(self, health_client):
        client, auth_service = health_client
        auth_service._background_service_healthy = False

        resp = client.get("/health")

        assert resp.status_code == 503
        assert "Background tag generation is unhealthy" in resp.json()["detail"]

    def test_returns_503_when_stream_consumer_is_down_and_recovers_to_200(self, health_client):
        client, auth_service = health_client
        auth_service._mark_consumer_health("articles", True)
        auth_service._mark_consumer_health("tags", False)

        resp_down = client.get("/health")
        assert resp_down.status_code == 503
        assert "Stream consumers are down: tags" in resp_down.json()["detail"]

        # Once the supervised consumer recovers, /health returns 200 again
        auth_service._mark_consumer_health("tags", True)
        resp_recovered = client.get("/health")
        assert resp_recovered.status_code == 200
        assert resp_recovered.json() == {"status": "healthy", "service": "tag-generator"}
