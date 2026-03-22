"""Tests for poison-pill (empty-extraction) skip logic in BatchProcessor.

Articles that repeatedly return tags=[] (e.g. too-short content, suspicious
patterns) should be skipped after max_empty_extraction_retries consecutive
empty results within the same process lifetime.
"""

from dataclasses import dataclass, field
from unittest.mock import MagicMock

from tag_generator.batch_processor import BatchProcessor
from tag_generator.config import TagGeneratorConfig


@dataclass
class FakeOutcome:
    """Minimal stand-in for TagExtractionOutcome."""

    tags: list[str] = field(default_factory=list)
    confidence: float = 0.0
    tag_count: int = 0
    inference_ms: float = 10.0
    language: str = "en"
    model_name: str = "test"
    sanitized_length: int = 100
    tag_confidences: dict = field(default_factory=dict)
    embedding_backend: str = "test"
    embedding_metadata: dict = field(default_factory=dict)


def _make_article(article_id: str = "art-1", feed_id: str = "feed-1") -> dict:
    return {
        "id": article_id,
        "title": f"Title {article_id}",
        "content": f"Content for {article_id}",
        "feed_id": feed_id,
    }


def _empty_outcome() -> FakeOutcome:
    return FakeOutcome(tags=[], tag_count=0)


def _good_outcome() -> FakeOutcome:
    return FakeOutcome(
        tags=["ml", "ai"],
        confidence=0.9,
        tag_count=2,
        tag_confidences={"ml": 0.9, "ai": 0.8},
    )


class _Harness:
    """Shared fixture builder for BatchProcessor tests."""

    def __init__(self, max_retries: int = 3):
        config = TagGeneratorConfig()
        config.max_empty_extraction_retries = max_retries
        config.batch_limit = 75
        config.progress_log_interval = 10
        config.memory_cleanup_interval = 25
        config.enable_gc_collection = False

        self.article_fetcher = MagicMock()
        self.tag_extractor = MagicMock()
        self.tag_inserter = MagicMock()
        self.cascade_controller = MagicMock()
        self.cursor_manager = MagicMock()

        # Default: cascade says no refine needed
        self.cascade_controller.evaluate.return_value = MagicMock(needs_refine=False, as_dict=lambda: {})

        # Default: batch upsert succeeds
        self.tag_inserter.batch_upsert_tags_no_commit.return_value = {
            "success": True,
            "processed_articles": 0,  # overridden per test
            "failed_articles": 0,
            "errors": [],
        }

        self.processor = BatchProcessor(
            config,
            self.article_fetcher,
            self.tag_extractor,
            self.tag_inserter,
            self.cascade_controller,
            self.cursor_manager,
        )


class TestEmptyExtractionTracking:
    """Tests for the process-local empty-extraction skip mechanism."""

    # ------------------------------------------------------------------ 1
    def test_empty_extraction_increments_failure_count(self):
        """tags=[] should increment the per-article empty-extraction counter."""
        h = _Harness(max_retries=3)
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()

        article = _make_article("poison-1")
        h.processor.process_articles_as_batch(None, [article])

        assert h.processor._empty_extraction_counts.get("poison-1", 0) == 1

    # ------------------------------------------------------------------ 2
    def test_article_at_retry_threshold_is_skipped_on_next_batch(self):
        """Once an article reaches the threshold, the NEXT batch should skip it."""
        h = _Harness(max_retries=2)
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()

        article = _make_article("poison-1")

        # First two batches: article is processed (and fails extraction)
        h.processor.process_articles_as_batch(None, [article])
        h.processor.process_articles_as_batch(None, [article])
        assert h.processor._empty_extraction_counts["poison-1"] == 2

        # Third batch: article should be filtered out before processing
        h.tag_extractor.extract_tags_with_metrics.reset_mock()
        stats = h.processor.process_articles_as_batch(None, [article])

        # extract should NOT be called since the article was pre-filtered
        h.tag_extractor.extract_tags_with_metrics.assert_not_called()
        # total_processed should be 0 (only skipped articles in the batch)
        assert stats["total_processed"] == 0

    # ------------------------------------------------------------------ 3
    def test_article_below_retry_threshold_is_still_processed(self):
        """Articles below the threshold should still be attempted."""
        h = _Harness(max_retries=3)
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()

        article = _make_article("almost-poison")

        # One failure — still below threshold of 3
        h.processor.process_articles_as_batch(None, [article])
        assert h.processor._empty_extraction_counts["almost-poison"] == 1

        # Second attempt: should still be processed
        h.tag_extractor.extract_tags_with_metrics.reset_mock()
        h.processor.process_articles_as_batch(None, [article])

        h.tag_extractor.extract_tags_with_metrics.assert_called_once()
        assert h.processor._empty_extraction_counts["almost-poison"] == 2

    # ------------------------------------------------------------------ 4
    def test_skip_state_persists_across_batch_calls_for_same_processor(self):
        """The empty-extraction counter survives across multiple batch calls."""
        h = _Harness(max_retries=2)
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()

        article = _make_article("persistent-poison")

        # Accumulate failures across separate batch calls
        h.processor.process_articles_as_batch(None, [article])
        h.processor.process_articles_as_batch(None, [article])

        # Counter should be 2 after two batch calls
        assert h.processor._empty_extraction_counts["persistent-poison"] == 2

        # Third call: should be skipped
        h.tag_extractor.extract_tags_with_metrics.reset_mock()
        h.processor.process_articles_as_batch(None, [article])
        h.tag_extractor.extract_tags_with_metrics.assert_not_called()

    # ------------------------------------------------------------------ 5
    def test_success_before_threshold_clears_failure_count(self):
        """A successful extraction before reaching threshold resets the counter."""
        h = _Harness(max_retries=3)

        article = _make_article("recoverable")

        # First call: empty extraction
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()
        h.processor.process_articles_as_batch(None, [article])
        assert h.processor._empty_extraction_counts["recoverable"] == 1

        # Second call: successful extraction — counter should reset
        h.tag_extractor.extract_tags_with_metrics.return_value = _good_outcome()
        h.tag_inserter.batch_upsert_tags_no_commit.return_value = {
            "success": True,
            "processed_articles": 1,
            "failed_articles": 0,
            "errors": [],
        }
        h.processor.process_articles_as_batch(None, [article])

        assert "recoverable" not in h.processor._empty_extraction_counts

    # ------------------------------------------------------------------ 6
    def test_empty_extraction_does_not_increment_failed_stat(self):
        """Empty extraction (tags=[]) should NOT increment the 'failed' stat.

        Only exceptions during extraction increment 'failed'. This preserves
        the existing behaviour where empty results are silently skipped.
        """
        h = _Harness(max_retries=10)  # high threshold so article is not skipped
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()

        article = _make_article("short-content")
        stats = h.processor.process_articles_as_batch(None, [article])

        assert stats["failed"] == 0

    # ------------------------------------------------------------------ 7
    def test_skipped_articles_do_not_contribute_to_total_processed(self):
        """Articles skipped due to empty-extraction threshold should not
        appear in total_processed. Only articles actually attempted count."""
        h = _Harness(max_retries=1)
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()

        poison = _make_article("poison-a")
        normal = _make_article("normal-b")

        # First batch: both are processed; poison fails extraction
        h.processor.process_articles_as_batch(None, [poison])
        assert h.processor._empty_extraction_counts["poison-a"] == 1

        # Second batch: poison should be skipped, normal should be processed
        h.tag_extractor.extract_tags_with_metrics.reset_mock()
        h.tag_extractor.extract_tags_with_metrics.return_value = _good_outcome()
        h.tag_inserter.batch_upsert_tags_no_commit.return_value = {
            "success": True,
            "processed_articles": 1,
            "failed_articles": 0,
            "errors": [],
        }
        stats = h.processor.process_articles_as_batch(None, [poison, normal])

        # Only normal-b was processed; poison-a was skipped
        assert stats["total_processed"] == 1
        # extract_tags_with_metrics should be called once (for normal-b only)
        h.tag_extractor.extract_tags_with_metrics.assert_called_once()

    # ------------------------------------------------------------------ 8
    def test_process_articles_as_batch_returns_skipped_count(self):
        """batch stats must include 'skipped' count for visibility."""
        h = _Harness(max_retries=1)
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()

        articles = [_make_article("p1"), _make_article("p2")]
        # First pass: both fail extraction → counts reach threshold
        h.processor.process_articles_as_batch(None, articles)
        # Second pass: both should be skipped
        stats = h.processor.process_articles_as_batch(None, articles)
        assert stats.get("skipped") == 2


class TestForwardHeadOfLineBlocking:
    """Tests for forward processing pagination past poison pills."""

    # ------------------------------------------------------------------ 9
    def test_forward_processing_pages_past_known_poison_pills(self):
        """When all first-page articles are poison-pilled, forward processing
        must paginate to next pages to find processable articles."""
        h = _Harness(max_retries=1)
        h.processor.backfill_completed = True

        poison1 = {**_make_article("poison-1"), "created_at": "2026-03-22T10:00:00Z"}
        poison2 = {**_make_article("poison-2"), "created_at": "2026-03-21T10:00:00Z"}
        good1 = {**_make_article("good-1"), "created_at": "2026-03-20T10:00:00Z"}

        # Pre-poison the first two articles
        h.tag_extractor.extract_tags_with_metrics.return_value = _empty_outcome()
        h.processor.process_articles_as_batch(None, [poison1, poison2])

        # Setup: forward cursor
        h.cursor_manager.get_forward_cursor_position.return_value = ("2026-03-23T00:00:00Z", "start")

        # First page (fetch_new_articles): returns the 2 poisons
        h.article_fetcher.fetch_new_articles.return_value = [poison1, poison2]

        # Second page (fetch_articles with cursor): returns the good article
        h.article_fetcher.fetch_articles.return_value = [good1]

        # Good article: successful extraction
        def side_effect(title, content):
            return _good_outcome()

        h.tag_extractor.extract_tags_with_metrics.side_effect = side_effect
        h.tag_inserter.batch_upsert_tags_no_commit.return_value = {
            "success": True,
            "processed_articles": 1,
            "failed_articles": 0,
            "errors": [],
        }

        stats = h.processor.process_article_batch_forward(None, h.cursor_manager)

        # Good article must have been processed
        assert stats.get("successful", 0) >= 1
        # fetch_articles must have been called to get past the poison page
        h.article_fetcher.fetch_articles.assert_called_once()
