"""Unit tests for BatchProcessor lifecycle, backfill pagination, cursor updates, and error handling."""

from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any
from unittest.mock import MagicMock, patch

import pytest

from tag_generator.batch_processor import BatchProcessor
from tag_generator.config import TagGeneratorConfig
from tag_generator.exceptions import BatchProcessingError


@dataclass
class _FakeOutcome:
    tags: list[str] = field(default_factory=lambda: ["rust", "python"])
    confidence: float = 0.85
    tag_count: int = 2
    inference_ms: float = 12.0
    language: str = "en"
    model_name: str = "test-model"
    sanitized_length: int = 120
    tag_confidences: dict[str, float] = field(default_factory=lambda: {"rust": 0.9, "python": 0.8})
    embedding_backend: str = "test"
    embedding_metadata: dict[str, Any] = field(default_factory=dict)


def _article(article_id: str, created_at: str | datetime = "2026-10-20T12:00:00Z") -> dict[str, Any]:
    return {
        "id": article_id,
        "title": f"Title {article_id}",
        "content": f"Content for {article_id}",
        "created_at": created_at,
        "feed_id": "feed-1",
    }


def _build_processor(batch_limit: int = 3) -> tuple[BatchProcessor, dict[str, MagicMock]]:
    config = TagGeneratorConfig(
        batch_limit=batch_limit,
        progress_log_interval=2,
        memory_cleanup_interval=2,
        enable_gc_collection=True,
    )
    fetcher = MagicMock()
    extractor = MagicMock()
    inserter = MagicMock()
    cascade = MagicMock()
    cursor_manager = MagicMock()

    cursor_manager.forward_cursor_created_at = None
    cursor_manager.forward_cursor_id = None
    cursor_manager.get_initial_cursor_position.return_value = ("2026-10-24T00:00:00Z", "init-id")
    cursor_manager.get_forward_cursor_position.return_value = ("2026-10-24T00:00:00Z", "fwd-init-id")

    extractor.extract_tags_with_metrics.return_value = _FakeOutcome()
    cascade.evaluate.return_value = MagicMock(needs_refine=True, as_dict=lambda: {"needs_refine": True})
    inserter.batch_upsert_tags_no_commit.side_effect = lambda _conn, batch: {
        "success": True,
        "processed_articles": len(batch),
        "failed_articles": 0,
        "errors": [],
    }

    processor = BatchProcessor(
        config=config,
        article_fetcher=fetcher,
        tag_extractor=extractor,
        tag_inserter=inserter,
        cascade_controller=cascade,
        cursor_manager=cursor_manager,
    )
    mocks = {
        "fetcher": fetcher,
        "extractor": extractor,
        "inserter": inserter,
        "cascade": cascade,
        "cursor_manager": cursor_manager,
    }
    return processor, mocks


class TestProcessArticleBatchStrategyDispatch:
    """Tests for process_article_batch switching between backfill and forward modes."""

    def test_dispatches_to_backfill_when_backfill_not_completed(self):
        processor, mocks = _build_processor()
        processor.backfill_completed = False

        with (
            patch.object(processor, "process_article_batch_backfill", return_value={"mode": "backfill"}) as mock_bf,
            patch.object(processor, "process_article_batch_forward") as mock_fwd,
        ):
            result = processor.process_article_batch(None, mocks["cursor_manager"])

        assert result == {"mode": "backfill"}
        mock_bf.assert_called_once_with(None, mocks["cursor_manager"])
        mock_fwd.assert_not_called()

    def test_dispatches_to_forward_when_backfill_completed(self):
        processor, mocks = _build_processor()
        processor.backfill_completed = True

        with (
            patch.object(processor, "process_article_batch_backfill") as mock_bf,
            patch.object(processor, "process_article_batch_forward", return_value={"mode": "forward"}) as mock_fwd,
        ):
            result = processor.process_article_batch(None, mocks["cursor_manager"])

        assert result == {"mode": "forward"}
        mock_fwd.assert_called_once_with(None, mocks["cursor_manager"])
        mock_bf.assert_not_called()


class TestProcessArticleBatchBackfill:
    """Tests for multi-page collection, cursor updates, and completion state in backfill mode."""

    def test_collects_across_multiple_pages_and_caps_at_batch_limit(self):
        """Backfill fetches pages until batch_limit is reached and updates both cursors."""
        processor, mocks = _build_processor(batch_limit=3)
        mocks["fetcher"].count_untagged_articles.return_value = 10

        page1 = [
            _article("art-1", "2026-10-23T10:00:00Z"),
            _article("art-2", "2026-10-22T10:00:00Z"),
        ]
        page2 = [
            _article("art-3", datetime(2026, 10, 21, 10, 0, 0, tzinfo=UTC)),
            _article("art-4", "2026-10-20T10:00:00Z"),
        ]
        mocks["fetcher"].fetch_articles.side_effect = [page1, page2]

        stats = processor.process_article_batch_backfill(None, mocks["cursor_manager"])

        assert stats["total_processed"] == 3
        assert stats["successful"] == 3
        assert stats["failed"] == 0
        assert stats["has_more_pending"] is True
        assert mocks["fetcher"].fetch_articles.call_count == 2

        # Backward cursor moves to the 3rd article (oldest processed in batch)
        mocks["cursor_manager"].update_cursor_position.assert_called_once_with(
            "2026-10-21T10:00:00+00:00",
            "art-3",
        )
        # Forward cursor moves to the 1st article (newest in batch)
        mocks["cursor_manager"].update_forward_cursor_position.assert_called_once_with(
            "2026-10-23T10:00:00Z",
            "art-1",
        )

    def test_transitions_to_backfill_completed_after_three_consecutive_empty_fetches(self):
        """Three consecutive empty fetch_articles calls mark backfill_completed = True."""
        processor, mocks = _build_processor(batch_limit=5)
        mocks["fetcher"].count_untagged_articles.return_value = 5
        mocks["fetcher"].fetch_articles.return_value = []

        for expected_count in (1, 2):
            processor.process_article_batch_backfill(None, mocks["cursor_manager"])
            assert processor.consecutive_empty_backfill_fetches == expected_count
            assert processor.backfill_completed is False

        processor.process_article_batch_backfill(None, mocks["cursor_manager"])
        assert processor.consecutive_empty_backfill_fetches == 3
        assert processor.backfill_completed is True

    def test_hybrid_mode_merges_forward_stats_when_no_untagged_articles_remain(self):
        """When hybrid forward pass processes articles and count_untagged_articles is 0,
        backfill returns the forward processing counts instead of zeros."""
        processor, mocks = _build_processor(batch_limit=5)
        mocks["cursor_manager"].forward_cursor_created_at = "2026-10-23T00:00:00Z"
        mocks["cursor_manager"].forward_cursor_id = "art-fwd-1"
        mocks["fetcher"].count_untagged_articles.return_value = 0

        with patch.object(
            processor,
            "process_article_batch_forward",
            return_value={"total_processed": 2, "successful": 2, "failed": 0},
        ) as mock_forward:
            stats = processor.process_article_batch_backfill(None, mocks["cursor_manager"])

        mock_forward.assert_called_once_with(None, mocks["cursor_manager"], _from_backfill=True)
        assert stats["total_processed"] == 2
        assert stats["successful"] == 2
        assert stats["failed"] == 0
        assert stats["has_more_pending"] is False

    def test_hybrid_mode_returns_early_when_forward_reaches_batch_limit(self):
        """When hybrid forward pass fills the entire batch_limit, backfill returns immediately."""
        processor, mocks = _build_processor(batch_limit=2)
        mocks["cursor_manager"].forward_cursor_created_at = "2026-10-23T00:00:00Z"
        mocks["cursor_manager"].forward_cursor_id = "art-fwd-1"

        forward_result = {"total_processed": 2, "successful": 2, "failed": 0, "has_more_pending": True}
        with patch.object(processor, "process_article_batch_forward", return_value=forward_result):
            stats = processor.process_article_batch_backfill(None, mocks["cursor_manager"])

        assert stats is forward_result
        mocks["fetcher"].count_untagged_articles.assert_not_called()


class TestProcessArticlesAsBatchErrors:
    """Tests for per-article extraction exceptions and batch upsert failures."""

    def test_single_article_extraction_exception_increments_failed_and_continues_batch(self):
        """An exception during one article's tag extraction does not abort the rest of the batch."""
        processor, mocks = _build_processor()

        def extract_side_effect(title: str, _content: str) -> _FakeOutcome:
            if "art-bad" in title:
                raise RuntimeError("corrupted model tensor")
            return _FakeOutcome()

        mocks["extractor"].extract_tags_with_metrics.side_effect = extract_side_effect

        articles = [_article("art-ok-1"), _article("art-bad"), _article("art-ok-2")]
        stats = processor.process_articles_as_batch(None, articles)

        assert stats["total_processed"] == 3
        assert stats["successful"] == 2
        assert stats["failed"] == 1
        upserted_batch = mocks["inserter"].batch_upsert_tags_no_commit.call_args[0][1]
        assert [item["article_id"] for item in upserted_batch] == ["art-ok-1", "art-ok-2"]

    def test_batch_upsert_failure_raises_batch_processing_error(self):
        """When batch_upsert_tags_no_commit reports success=False with failed_articles > 0,
        BatchProcessingError is raised."""
        processor, mocks = _build_processor()
        mocks["inserter"].batch_upsert_tags_no_commit.side_effect = None
        mocks["inserter"].batch_upsert_tags_no_commit.return_value = {
            "success": False,
            "processed_articles": 0,
            "failed_articles": 2,
            "errors": ["rpc timeout"],
        }

        articles = [_article("art-1"), _article("art-2")]
        with pytest.raises(BatchProcessingError, match="Batch processing failed for 2 articles"):
            processor.process_articles_as_batch(None, articles)
