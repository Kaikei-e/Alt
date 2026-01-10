"""Tests for batch processor mutual recursion prevention."""

from unittest.mock import MagicMock, patch

import pytest

from tag_generator.batch_processor import BatchProcessor
from tag_generator.config import TagGeneratorConfig


class TestBatchProcessorRecursionGuard:
    """Test cases for preventing mutual recursion between forward and backfill processing."""

    @pytest.fixture
    def mock_dependencies(self):
        """Create mock dependencies for BatchProcessor."""
        config = TagGeneratorConfig()
        article_fetcher = MagicMock()
        tag_extractor = MagicMock()
        tag_inserter = MagicMock()
        cascade_controller = MagicMock()
        cursor_manager = MagicMock()

        return {
            "config": config,
            "article_fetcher": article_fetcher,
            "tag_extractor": tag_extractor,
            "tag_inserter": tag_inserter,
            "cascade_controller": cascade_controller,
            "cursor_manager": cursor_manager,
        }

    @pytest.fixture
    def batch_processor(self, mock_dependencies):
        """Create a BatchProcessor with mocked dependencies."""
        return BatchProcessor(
            mock_dependencies["config"],
            mock_dependencies["article_fetcher"],
            mock_dependencies["tag_extractor"],
            mock_dependencies["tag_inserter"],
            mock_dependencies["cascade_controller"],
            mock_dependencies["cursor_manager"],
        )

    def test_forward_does_not_recurse_to_backfill_when_called_from_backfill(self, batch_processor, mock_dependencies):
        """When forward is called from backfill, it should NOT call backfill again."""
        # Setup: backfill not completed, no new articles
        batch_processor.backfill_completed = False
        mock_dependencies["article_fetcher"].fetch_new_articles.return_value = []
        mock_dependencies["cursor_manager"].get_forward_cursor_position.return_value = (
            "2024-01-01T00:00:00Z",
            "test-id",
        )

        mock_conn = MagicMock()
        mock_conn.autocommit = True

        # Call forward with _from_backfill=True
        with patch.object(batch_processor, "process_article_batch_backfill") as mock_backfill:
            result = batch_processor.process_article_batch_forward(
                mock_conn, mock_dependencies["cursor_manager"], _from_backfill=True
            )

        # Assert: backfill should NOT have been called
        mock_backfill.assert_not_called()
        assert result["total_processed"] == 0

    def test_forward_calls_backfill_when_not_from_backfill(self, batch_processor, mock_dependencies):
        """When forward is called normally (not from backfill), it should call backfill."""
        # Setup: backfill not completed, no new articles
        batch_processor.backfill_completed = False
        mock_dependencies["article_fetcher"].fetch_new_articles.return_value = []
        mock_dependencies["cursor_manager"].get_forward_cursor_position.return_value = (
            "2024-01-01T00:00:00Z",
            "test-id",
        )

        mock_conn = MagicMock()
        mock_conn.autocommit = True

        # Call forward without _from_backfill (default False)
        with patch.object(batch_processor, "process_article_batch_backfill") as mock_backfill:
            mock_backfill.return_value = {"total_processed": 5, "successful": 5, "failed": 0}
            batch_processor.process_article_batch_forward(mock_conn, mock_dependencies["cursor_manager"])

        # Assert: backfill SHOULD have been called
        mock_backfill.assert_called_once()

    def test_backfill_passes_from_backfill_flag_to_forward(self, batch_processor, mock_dependencies):
        """Backfill should pass _from_backfill=True when calling forward."""
        # Setup: hybrid mode active (forward cursors exist)
        mock_dependencies["cursor_manager"].forward_cursor_created_at = "2024-01-01T00:00:00Z"
        mock_dependencies["cursor_manager"].forward_cursor_id = "test-forward-id"
        mock_dependencies["cursor_manager"].get_initial_cursor_position.return_value = (
            "2024-01-01T00:00:00Z",
            "test-id",
        )
        mock_dependencies["article_fetcher"].count_untagged_articles.return_value = 0

        mock_conn = MagicMock()
        mock_conn.autocommit = True

        # Spy on the forward method to check the argument
        call_args_list = []

        def capturing_forward(*args, **kwargs):
            call_args_list.append((args, kwargs))
            return {"successful": 0, "total_processed": 0}

        with patch.object(batch_processor, "process_article_batch_forward", side_effect=capturing_forward):
            batch_processor.process_article_batch_backfill(mock_conn, mock_dependencies["cursor_manager"])

        # Check that forward was called with _from_backfill=True
        assert len(call_args_list) > 0, "process_article_batch_forward should have been called"
        _, kwargs = call_args_list[0]
        assert kwargs.get("_from_backfill") is True, "_from_backfill should be True when called from backfill"

    def test_no_infinite_recursion_with_empty_articles(self, batch_processor, mock_dependencies):
        """Ensure no infinite recursion when both forward and backfill return empty."""
        # Setup the recursion trigger conditions
        batch_processor.backfill_completed = False
        mock_dependencies["cursor_manager"].forward_cursor_created_at = "2024-01-01T00:00:00Z"
        mock_dependencies["cursor_manager"].forward_cursor_id = "test-forward-id"
        mock_dependencies["cursor_manager"].get_forward_cursor_position.return_value = (
            "2024-01-01T00:00:00Z",
            "test-id",
        )
        mock_dependencies["cursor_manager"].get_initial_cursor_position.return_value = (
            "2024-01-01T00:00:00Z",
            "test-id",
        )

        # Both fetch methods return empty
        mock_dependencies["article_fetcher"].fetch_new_articles.return_value = []
        mock_dependencies["article_fetcher"].fetch_articles.return_value = []
        mock_dependencies["article_fetcher"].count_untagged_articles.return_value = 0

        mock_conn = MagicMock()
        mock_conn.autocommit = True

        # This should NOT raise RecursionError
        # Previously this would recurse infinitely
        result = batch_processor.process_article_batch_backfill(mock_conn, mock_dependencies["cursor_manager"])

        # Should complete without error and return stats
        assert "total_processed" in result
        assert result["total_processed"] == 0
