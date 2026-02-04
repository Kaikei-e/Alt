"""Tests for low-quality tag regeneration functionality."""

from unittest.mock import MagicMock

import pytest

from article_fetcher.fetch import ArticleFetcher


class TestArticleFetcherLowConfidence:
    """Tests for fetching low-confidence articles."""

    def test_fetch_low_confidence_articles_returns_articles_below_threshold(self):
        """Should return articles with average tag confidence below threshold."""
        fetcher = ArticleFetcher()
        mock_conn = MagicMock()
        mock_cursor = mock_conn.cursor.return_value.__enter__.return_value
        mock_cursor.fetchall.return_value = [
            {
                "id": "article-1",
                "title": "Low Quality Article",
                "content": "Content here",
                "created_at": "2024-01-01T00:00:00Z",
                "feed_id": "feed-1",
                "url": "https://example.com/1",
                "avg_confidence": 0.35,
            },
            {
                "id": "article-2",
                "title": "Another Low Quality",
                "content": "More content",
                "created_at": "2024-01-02T00:00:00Z",
                "feed_id": "feed-1",
                "url": "https://example.com/2",
                "avg_confidence": 0.42,
            },
        ]

        articles = fetcher.fetch_low_confidence_articles(mock_conn, confidence_threshold=0.5, limit=100)

        assert len(articles) == 2
        assert articles[0]["id"] == "article-1"
        assert articles[0]["avg_confidence"] == 0.35
        mock_cursor.execute.assert_called_once()

    def test_fetch_low_confidence_articles_uses_correct_query(self):
        """Should query articles grouped by avg confidence below threshold."""
        fetcher = ArticleFetcher()
        mock_conn = MagicMock()
        mock_cursor = mock_conn.cursor.return_value.__enter__.return_value
        mock_cursor.fetchall.return_value = []

        fetcher.fetch_low_confidence_articles(mock_conn, confidence_threshold=0.5, limit=50)

        call_args = mock_cursor.execute.call_args[0]
        query = call_args[0]
        params = call_args[1]

        # Verify query structure
        assert "AVG" in query.upper()
        assert "article_tags" in query.lower()
        assert "confidence" in query.lower()
        assert "HAVING" in query.upper()
        # Verify parameters
        assert 0.5 in params
        assert 50 in params

    def test_fetch_low_confidence_articles_default_threshold(self):
        """Should use default threshold of 0.5 if not specified."""
        fetcher = ArticleFetcher()
        mock_conn = MagicMock()
        mock_cursor = mock_conn.cursor.return_value.__enter__.return_value
        mock_cursor.fetchall.return_value = []

        fetcher.fetch_low_confidence_articles(mock_conn)

        call_args = mock_cursor.execute.call_args[0]
        params = call_args[1]
        assert params[0] == 0.5  # default threshold

    def test_fetch_low_confidence_articles_respects_limit(self):
        """Should respect the limit parameter."""
        fetcher = ArticleFetcher()
        mock_conn = MagicMock()
        mock_cursor = mock_conn.cursor.return_value.__enter__.return_value
        mock_cursor.fetchall.return_value = []

        fetcher.fetch_low_confidence_articles(mock_conn, limit=25)

        call_args = mock_cursor.execute.call_args[0]
        params = call_args[1]
        assert 25 in params

    def test_fetch_low_confidence_articles_orders_by_confidence_asc(self):
        """Should order results by confidence ascending (lowest first)."""
        fetcher = ArticleFetcher()
        mock_conn = MagicMock()
        mock_cursor = mock_conn.cursor.return_value.__enter__.return_value
        mock_cursor.fetchall.return_value = []

        fetcher.fetch_low_confidence_articles(mock_conn)

        call_args = mock_cursor.execute.call_args[0]
        query = call_args[0]
        assert "ORDER BY" in query.upper()
        assert "ASC" in query.upper()

    def test_fetch_low_confidence_articles_handles_db_error(self):
        """Should raise ArticleFetchError on database error."""
        import psycopg2

        from article_fetcher.fetch import ArticleFetchError

        fetcher = ArticleFetcher()
        mock_conn = MagicMock()
        mock_cursor = mock_conn.cursor.return_value.__enter__.return_value
        mock_cursor.execute.side_effect = psycopg2.Error("DB error")

        with pytest.raises(ArticleFetchError):
            fetcher.fetch_low_confidence_articles(mock_conn)


class TestTagInserterConfidenceComparison:
    """Tests for confidence-based tag update logic."""

    def test_upsert_tags_with_comparison_updates_higher_confidence(self):
        """Should update tags when new confidence is higher."""
        from tag_inserter.upsert_tags import TagInserter

        inserter = TagInserter()
        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        # _get_cursor uses conn.cursor() directly
        mock_conn.cursor.return_value = mock_cursor

        # Set up side effects for fetchone calls:
        # 1. First call: get feed_id for article -> (feed_id, url)
        mock_cursor.fetchone.side_effect = [
            ("feed-1", "https://example.com/1"),  # feed_id lookup
        ]
        # fetchall for existing tags returns empty, meaning new tag
        mock_cursor.fetchall.return_value = []

        article_tags = [
            {
                "article_id": "article-1",
                "tags": ["tag1"],
                "tag_confidences": {"tag1": 0.8},
            }
        ]

        inserter.batch_upsert_tags_with_comparison(mock_conn, article_tags)

        # Verify execute was called
        assert mock_cursor.execute.called

    def test_upsert_tags_with_comparison_skips_lower_confidence(self):
        """Should not update tags when new confidence is lower."""
        from tag_inserter.upsert_tags import TagInserter

        inserter = TagInserter()
        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value = mock_cursor

        # Set up side effects for fetchone calls
        mock_cursor.fetchone.side_effect = [
            ("feed-1", "https://example.com/1"),  # feed_id lookup
        ]
        # Existing tag with higher confidence (0.9)
        mock_cursor.fetchall.return_value = [
            ("tag1", "tag-id-1", 0.9)  # (tag_name, id, confidence)
        ]

        article_tags = [
            {
                "article_id": "article-1",
                "tags": ["tag1"],
                "tag_confidences": {"tag1": 0.5},  # new lower confidence
            }
        ]

        result = inserter.batch_upsert_tags_with_comparison(mock_conn, article_tags)

        # Should report skipped
        assert result.get("skipped_lower_confidence", 0) == 1

    def test_upsert_tags_with_comparison_updates_when_higher(self):
        """Should update tags when new confidence is higher than existing."""
        from tag_inserter.upsert_tags import TagInserter

        inserter = TagInserter()
        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value = mock_cursor

        # Set up side effects for fetchone calls
        mock_cursor.fetchone.side_effect = [
            ("feed-1", "https://example.com/1"),  # feed_id lookup
        ]
        # Existing tag with lower confidence (0.3)
        mock_cursor.fetchall.return_value = [
            ("tag1", "tag-id-1", 0.3)  # (tag_name, id, confidence)
        ]

        article_tags = [
            {
                "article_id": "article-1",
                "tags": ["tag1"],
                "tag_confidences": {"tag1": 0.8},  # new higher confidence
            }
        ]

        result = inserter.batch_upsert_tags_with_comparison(mock_conn, article_tags)

        # Should report updated
        assert result.get("updated_higher_confidence", 0) == 1
        assert result.get("skipped_lower_confidence", 0) == 0

    def test_get_existing_tag_confidence_returns_confidence(self):
        """Should return existing tag confidence from database."""
        from tag_inserter.upsert_tags import TagInserter

        inserter = TagInserter()
        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.fetchone.return_value = (0.75,)

        confidence = inserter.get_existing_tag_confidence(mock_conn, "article-1", "tag-1")

        assert confidence == 0.75
        mock_cursor.execute.assert_called_once()

    def test_get_existing_tag_confidence_returns_none_if_not_found(self):
        """Should return None if tag doesn't exist."""
        from tag_inserter.upsert_tags import TagInserter

        inserter = TagInserter()
        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.fetchone.return_value = None

        confidence = inserter.get_existing_tag_confidence(mock_conn, "article-1", "tag-1")

        assert confidence is None


class TestBatchProcessorRegeneration:
    """Tests for regeneration batch processing."""

    def test_process_regeneration_batch_fetches_low_confidence_articles(self):
        """Should fetch articles with low confidence tags."""
        from tag_generator.batch_processor import BatchProcessor

        # Create mocks
        mock_config = MagicMock()
        mock_config.batch_limit = 100
        mock_config.enable_gc_collection = False
        mock_config.progress_log_interval = 10
        mock_config.memory_cleanup_interval = 50

        mock_article_fetcher = MagicMock()
        mock_article_fetcher.fetch_low_confidence_articles.return_value = []

        mock_tag_extractor = MagicMock()
        mock_tag_inserter = MagicMock()
        mock_cascade = MagicMock()
        mock_cursor_manager = MagicMock()

        processor = BatchProcessor(
            config=mock_config,
            article_fetcher=mock_article_fetcher,
            tag_extractor=mock_tag_extractor,
            tag_inserter=mock_tag_inserter,
            cascade_controller=mock_cascade,
            cursor_manager=mock_cursor_manager,
        )

        mock_conn = MagicMock()
        processor.process_regeneration_batch(mock_conn, confidence_threshold=0.5)

        mock_article_fetcher.fetch_low_confidence_articles.assert_called_once()
        call_kwargs = mock_article_fetcher.fetch_low_confidence_articles.call_args[1]
        assert call_kwargs.get("confidence_threshold") == 0.5

    def test_process_regeneration_batch_regenerates_tags(self):
        """Should extract new tags for low-confidence articles."""
        from tag_extractor.extract import TagExtractionOutcome
        from tag_generator.batch_processor import BatchProcessor

        mock_config = MagicMock()
        mock_config.batch_limit = 100
        mock_config.enable_gc_collection = False
        mock_config.progress_log_interval = 10
        mock_config.memory_cleanup_interval = 50

        mock_article_fetcher = MagicMock()
        mock_article_fetcher.fetch_low_confidence_articles.return_value = [
            {
                "id": "article-1",
                "title": "Test Article",
                "content": "Test content for regeneration",
                "created_at": "2024-01-01T00:00:00Z",
                "feed_id": "feed-1",
                "url": "https://example.com/1",
                "avg_confidence": 0.35,
            }
        ]

        mock_outcome = MagicMock(spec=TagExtractionOutcome)
        mock_outcome.tags = ["new_tag1", "new_tag2"]
        mock_outcome.tag_confidences = {"new_tag1": 0.8, "new_tag2": 0.75}
        mock_outcome.confidence = 0.8

        mock_tag_extractor = MagicMock()
        mock_tag_extractor.extract_tags_with_metrics.return_value = mock_outcome

        mock_tag_inserter = MagicMock()
        mock_tag_inserter.batch_upsert_tags_with_comparison.return_value = {
            "success": True,
            "processed_articles": 1,
            "failed_articles": 0,
        }

        mock_cascade = MagicMock()
        mock_cascade.evaluate.return_value = MagicMock(needs_refine=False, as_dict=lambda: {})

        mock_cursor_manager = MagicMock()

        processor = BatchProcessor(
            config=mock_config,
            article_fetcher=mock_article_fetcher,
            tag_extractor=mock_tag_extractor,
            tag_inserter=mock_tag_inserter,
            cascade_controller=mock_cascade,
            cursor_manager=mock_cursor_manager,
        )

        mock_conn = MagicMock()
        mock_conn.autocommit = True

        processor.process_regeneration_batch(mock_conn, confidence_threshold=0.5)

        # Verify tag extraction was called
        mock_tag_extractor.extract_tags_with_metrics.assert_called_once_with(
            "Test Article", "Test content for regeneration"
        )

    def test_process_regeneration_batch_uses_comparison_update(self):
        """Should use confidence comparison when upserting tags."""
        from tag_extractor.extract import TagExtractionOutcome
        from tag_generator.batch_processor import BatchProcessor

        mock_config = MagicMock()
        mock_config.batch_limit = 100
        mock_config.enable_gc_collection = False
        mock_config.progress_log_interval = 10
        mock_config.memory_cleanup_interval = 50

        mock_article_fetcher = MagicMock()
        mock_article_fetcher.fetch_low_confidence_articles.return_value = [
            {
                "id": "article-1",
                "title": "Test",
                "content": "Content",
                "created_at": "2024-01-01T00:00:00Z",
                "feed_id": "feed-1",
                "url": "https://example.com/1",
                "avg_confidence": 0.3,
            }
        ]

        mock_outcome = MagicMock(spec=TagExtractionOutcome)
        mock_outcome.tags = ["tag1"]
        mock_outcome.tag_confidences = {"tag1": 0.9}
        mock_outcome.confidence = 0.9

        mock_tag_extractor = MagicMock()
        mock_tag_extractor.extract_tags_with_metrics.return_value = mock_outcome

        mock_tag_inserter = MagicMock()
        mock_tag_inserter.batch_upsert_tags_with_comparison.return_value = {
            "success": True,
            "processed_articles": 1,
            "failed_articles": 0,
        }

        mock_cascade = MagicMock()
        mock_cascade.evaluate.return_value = MagicMock(needs_refine=False, as_dict=lambda: {})

        mock_cursor_manager = MagicMock()

        processor = BatchProcessor(
            config=mock_config,
            article_fetcher=mock_article_fetcher,
            tag_extractor=mock_tag_extractor,
            tag_inserter=mock_tag_inserter,
            cascade_controller=mock_cascade,
            cursor_manager=mock_cursor_manager,
        )

        mock_conn = MagicMock()
        mock_conn.autocommit = True

        processor.process_regeneration_batch(mock_conn, confidence_threshold=0.5)

        # Verify comparison-based upsert was used
        mock_tag_inserter.batch_upsert_tags_with_comparison.assert_called_once()

    def test_process_regeneration_batch_returns_stats(self):
        """Should return statistics about regeneration."""
        from tag_generator.batch_processor import BatchProcessor

        mock_config = MagicMock()
        mock_config.batch_limit = 100
        mock_config.enable_gc_collection = False
        mock_config.progress_log_interval = 10
        mock_config.memory_cleanup_interval = 50

        mock_article_fetcher = MagicMock()
        mock_article_fetcher.fetch_low_confidence_articles.return_value = []

        processor = BatchProcessor(
            config=mock_config,
            article_fetcher=mock_article_fetcher,
            tag_extractor=MagicMock(),
            tag_inserter=MagicMock(),
            cascade_controller=MagicMock(),
            cursor_manager=MagicMock(),
        )

        mock_conn = MagicMock()
        result = processor.process_regeneration_batch(mock_conn)

        assert "total_processed" in result
        assert "successful" in result
        assert "failed" in result


class TestRegenerateLowQualityTagsScript:
    """Tests for the CLI regeneration script."""

    def test_find_low_quality_articles_builds_correct_query(self):
        """Should build query to find articles with low avg confidence."""
        # This will be tested when we create the script
        pass

    def test_dry_run_does_not_modify_database(self):
        """Dry run mode should not commit changes."""
        # This will be tested when we create the script
        pass
