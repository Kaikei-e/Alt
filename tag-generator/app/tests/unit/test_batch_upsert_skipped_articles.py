"""Tests for batch upsert behavior with skipped articles (missing feed_id).

These tests verify that the skipped_articles counter is NOT added to failed_articles,
ensuring that batches with some or all skipped articles don't incorrectly report failure.
"""

from unittest.mock import MagicMock, patch


class TestBatchUpsertSkippedArticles:
    """Test that skipped articles (due to missing feed_id) don't cause batch failure."""

    def test_empty_article_list_returns_success(self):
        """Empty article list should return success."""
        from tag_inserter.upsert_tags import TagInserter

        conn = MagicMock()
        inserter = TagInserter()

        result = inserter.batch_upsert_tags_no_commit(conn, [])

        assert result["success"] is True
        assert result["processed_articles"] == 0
        assert result["failed_articles"] == 0

    def test_no_valid_tags_returns_success(self):
        """Articles with empty or invalid tags should not cause failure."""
        from tag_inserter.upsert_tags import TagInserter

        conn = MagicMock()
        cursor_mock = MagicMock()
        conn.cursor.return_value = cursor_mock
        cursor_mock.__enter__ = MagicMock(return_value=cursor_mock)
        cursor_mock.__exit__ = MagicMock(return_value=False)
        cursor_mock.close = MagicMock()

        inserter = TagInserter()
        article_tags = [
            {"article_id": "article-1", "tags": []},  # Empty tags
            {"article_id": "article-2", "tags": ["", "  "]},  # Whitespace-only tags
        ]

        result = inserter.batch_upsert_tags_no_commit(conn, article_tags)

        # No valid articles to process, so should succeed with 0 processed
        assert result["success"] is True
        assert result["processed_articles"] == 0
        assert result["failed_articles"] == 0

    @patch("tag_inserter.upsert_tags.psycopg2.extras.execute_batch")
    def test_skipped_articles_logged_as_info_not_warning(self, mock_execute_batch):
        """Verify that skipped articles are logged at INFO level, not WARNING."""
        from tag_inserter.upsert_tags import TagInserter

        conn = MagicMock()
        cursor_mock = MagicMock()
        conn.cursor.return_value = cursor_mock
        cursor_mock.__enter__ = MagicMock(return_value=cursor_mock)
        cursor_mock.__exit__ = MagicMock(return_value=False)
        cursor_mock.close = MagicMock()

        # Mock fetchone to return None (no feed_id) for all articles
        cursor_mock.fetchone.return_value = None
        cursor_mock.execute = MagicMock()

        inserter = TagInserter()
        article_tags = [
            {"article_id": "article-1", "tags": ["tag1"]},
        ]

        with patch("tag_inserter.upsert_tags.logger") as mock_logger:
            inserter.batch_upsert_tags_no_commit(conn, article_tags)

            # Check that info was called (not warning) for skipped articles
            info_calls = [call for call in mock_logger.info.call_args_list if "Skipped articles" in str(call)]
            warning_calls = [call for call in mock_logger.warning.call_args_list if "Skipped articles" in str(call)]

            # Should be logged as info, not warning
            assert len(info_calls) >= 1, "Skipped articles should be logged at INFO level"
            assert len(warning_calls) == 0, "Skipped articles should NOT be logged at WARNING level"

    @patch("tag_inserter.upsert_tags.psycopg2.extras.execute_batch")
    def test_skipped_articles_not_added_to_failed_count(self, mock_execute_batch):
        """Verify that skipped articles are NOT counted as failures."""
        from tag_inserter.upsert_tags import TagInserter

        conn = MagicMock()
        cursor_mock = MagicMock()
        conn.cursor.return_value = cursor_mock
        cursor_mock.__enter__ = MagicMock(return_value=cursor_mock)
        cursor_mock.__exit__ = MagicMock(return_value=False)
        cursor_mock.close = MagicMock()

        # Mock fetchone to return None (no feed_id) for all articles
        cursor_mock.fetchone.return_value = None
        cursor_mock.execute = MagicMock()

        inserter = TagInserter()
        article_tags = [
            {"article_id": "article-1", "tags": ["tag1"]},
            {"article_id": "article-2", "tags": ["tag2"]},
        ]

        result = inserter.batch_upsert_tags_no_commit(conn, article_tags)

        # All articles were skipped (no feed_id), but should NOT be counted as failures
        assert result["failed_articles"] == 0, "Skipped articles should NOT be counted as failures"
        assert result["processed_articles"] == 0, "No articles were actually processed"
        # Success should be True since there were no actual failures
        assert result["success"] is True, "Batch should succeed even if all articles were skipped"
