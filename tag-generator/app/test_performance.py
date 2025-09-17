"""
Performance test suite for tag-generator following TDD principles.
These tests define performance requirements that the refactored code must meet.
"""

import gc
import os
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from unittest.mock import Mock, patch

import psutil
import pytest

from main import TagGeneratorService
from tag_extractor.extract import TagExtractor
from tag_inserter.upsert_tags import TagInserter


class TestTagExtractorPerformance:
    """Test performance requirements for TagExtractor."""

    def test_should_process_multiple_articles_under_time_limit(self):
        """Tag extraction for 10 articles should complete under 30 seconds."""
        extractor = TagExtractor()

        # Sample articles for testing
        test_articles = [
            (
                "Tech News",
                "Machine learning and artificial intelligence are transforming the industry",
            ),
            (
                "Sports Update",
                "The championship game delivered an exciting finish for all fans",
            ),
            (
                "Weather Report",
                "Sunny skies and mild temperatures expected throughout the week",
            ),
            (
                "Health Tips",
                "Regular exercise and balanced nutrition contribute to overall wellness",
            ),
            (
                "Travel Guide",
                "Exploring beautiful destinations and discovering local culture experiences",
            ),
            (
                "Business News",
                "Stock market trends and economic indicators show positive growth",
            ),
            (
                "Science Discovery",
                "Researchers made breakthrough findings in quantum physics experiments",
            ),
            (
                "Food Review",
                "The restaurant offers authentic cuisine with fresh ingredients",
            ),
            (
                "Technology Update",
                "New software features enhance user experience and productivity",
            ),
            (
                "Education News",
                "Students demonstrate improved learning outcomes with innovative teaching methods",
            ),
        ]

        start_time = time.time()

        for title, content in test_articles:
            tags = extractor.extract_tags(title, content)
            assert isinstance(tags, list), "Tags should be returned as a list"

        end_time = time.time()
        processing_time = end_time - start_time

        # Should process 10 articles in under 30 seconds
        assert processing_time < 30.0, f"Processing took {processing_time:.2f}s, should be under 30s"

    def test_should_reuse_models_across_extractions(self):
        """Models should be loaded once and reused for multiple extractions."""
        extractor = TagExtractor()

        # First extraction should load models
        with patch.object(extractor, "_lazy_load_models", wraps=extractor._lazy_load_models) as mock_load:
            extractor.extract_tags("First title", "First content")
            assert mock_load.call_count == 1, "Models should be loaded on first call"

        # Subsequent extractions should reuse models
        with patch.object(extractor, "_lazy_load_models", wraps=extractor._lazy_load_models) as mock_load:
            extractor.extract_tags("Second title", "Second content")
            extractor.extract_tags("Third title", "Third content")
            assert mock_load.call_count == 0, "Models should not be reloaded"

    def test_should_handle_concurrent_extractions(self):
        """Should handle multiple concurrent tag extractions without race conditions."""
        extractor = TagExtractor()
        results = []
        errors = []

        def extract_tags_worker(title, content):
            try:
                tags = extractor.extract_tags(title, content)
                results.append(tags)
            except Exception as e:
                errors.append(e)

        threads = []
        test_data = [
            ("Tech News", "Machine learning and AI developments"),
            ("Sports Update", "Championship game results and statistics"),
            ("Weather Report", "Temperature and precipitation forecasts"),
            ("Health Tips", "Exercise and nutrition recommendations"),
            ("Travel Guide", "Destination reviews and travel advice"),
        ]

        for title, content in test_data:
            thread = threading.Thread(target=extract_tags_worker, args=(title, content))
            threads.append(thread)
            thread.start()

        for thread in threads:
            thread.join(timeout=10)

        assert len(errors) == 0, f"Concurrent extraction failed with errors: {errors}"
        assert len(results) == len(test_data), "All extractions should complete"

    def test_should_manage_memory_efficiently(self):
        """Memory usage should not grow excessively during batch processing."""
        extractor = TagExtractor()

        # Get initial memory usage
        process = psutil.Process(os.getpid())
        initial_memory = process.memory_info().rss / 1024 / 1024  # MB

        # Process multiple batches
        for batch in range(5):
            for i in range(20):
                extractor.extract_tags(f"Title {i}", f"Content for article {i} in batch {batch}")

            # Force garbage collection
            gc.collect()

        # Check final memory usage
        final_memory = process.memory_info().rss / 1024 / 1024  # MB
        memory_growth = final_memory - initial_memory

        # Memory growth should be reasonable (less than 500MB)
        assert memory_growth < 500, f"Memory grew by {memory_growth:.1f}MB, should be under 500MB"


class TestBatchProcessingPerformance:
    """Test performance requirements for batch processing."""

    def test_should_process_batch_faster_than_individual(self):
        """Batch processing should be faster than individual processing."""
        mock_conn = Mock()

        # Mock database responses
        mock_cursor = Mock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor
        mock_cursor.fetchall.return_value = [(1, "tag1"), (2, "tag2"), (3, "tag3")]

        inserter = TagInserter()

        # Test individual processing time
        start_time = time.time()
        for i in range(10):
            inserter.upsert_tags(mock_conn, f"article-{i}", [f"tag-{i}", f"tag-{i + 1}"], "test-feed")
        individual_time = time.time() - start_time

        # Reset mock
        mock_conn.reset_mock()
        mock_cursor.reset_mock()

        # Test batch processing time
        batch_data = [{"article_id": f"article-{i}", "tags": [f"tag-{i}", f"tag-{i + 1}"]} for i in range(10)]

        start_time = time.time()
        inserter.batch_upsert_tags(mock_conn, batch_data)
        batch_time = time.time() - start_time

        # Batch should be at least 30% faster
        assert batch_time < individual_time * 0.7, (
            f"Batch time {batch_time:.3f}s should be faster than individual {individual_time:.3f}s"
        )

    def test_should_handle_large_batches_efficiently(self):
        """Should handle large batches (1000+ articles) within reasonable time."""
        mock_conn = Mock()
        mock_cursor = Mock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

        # Mock tag ID responses
        tag_ids = [(i, f"tag-{i}") for i in range(1, 5001)]  # 5000 unique tags
        mock_cursor.fetchall.return_value = tag_ids

        inserter = TagInserter()

        # Create large batch
        large_batch = []
        for i in range(1000):
            large_batch.append(
                {
                    "article_id": f"article-{i}",
                    "tags": [f"tag-{i}", f"tag-{i + 1000}", f"tag-{i + 2000}"],
                }
            )

        start_time = time.time()
        result = inserter.batch_upsert_tags(mock_conn, large_batch)
        processing_time = time.time() - start_time

        # Should process 1000 articles in under 5 seconds
        assert processing_time < 5.0, f"Large batch took {processing_time:.2f}s, should be under 5s"
        assert result["success"] is True, "Large batch should complete successfully"


class TestServicePerformance:
    """Test performance requirements for the main service."""

    @patch("main.psycopg2.connect")
    def test_should_reuse_database_connections(self, mock_connect):
        """Service should reuse database connections efficiently."""
        mock_conn = Mock()
        mock_connect.return_value = mock_conn

        # Mock successful processing
        with patch.object(TagGeneratorService, "_process_article_batch") as mock_process:
            mock_process.return_value = {
                "total_processed": 5,
                "successful": 5,
                "failed": 0,
            }

            service = TagGeneratorService()

            # Run multiple cycles
            for _ in range(3):
                service.run_processing_cycle()

            # Should create and close connections for each cycle
            assert mock_connect.call_count == 3, "Should create connection for each cycle"
            assert mock_conn.close.call_count == 3, "Should close connection after each cycle"

    @patch("main.psycopg2.connect")
    def test_should_handle_connection_failures_gracefully(self, mock_connect):
        """Service should handle connection failures without crashing."""
        # First two attempts fail, third succeeds
        mock_connect.side_effect = [
            Exception("Connection failed"),
            Exception("Connection failed"),
            Mock(),
        ]

        service = TagGeneratorService()

        start_time = time.time()
        result = service.run_processing_cycle()
        recovery_time = time.time() - start_time

        # Should recover and complete within reasonable time (under 20 seconds with retries)
        assert recovery_time < 20.0, f"Recovery took {recovery_time:.2f}s, should be under 20s"
        assert result["success"] is True, "Should succeed after retries"

    def test_should_process_cycles_with_consistent_performance(self):
        """Multiple processing cycles should have consistent performance."""
        service = TagGeneratorService()
        cycle_times = []

        # Mock all dependencies
        with (
            patch.object(service, "_create_database_connection") as mock_conn,
            patch.object(service, "_process_article_batch") as mock_process,
        ):
            mock_conn.return_value = Mock()
            mock_process.return_value = {
                "total_processed": 10,
                "successful": 10,
                "failed": 0,
            }

            # Run multiple cycles and measure time
            for _ in range(5):
                start_time = time.time()
                service.run_processing_cycle()
                cycle_time = time.time() - start_time
                cycle_times.append(cycle_time)

        # Performance should be consistent (variation less than 50%)
        avg_time = sum(cycle_times) / len(cycle_times)
        max_deviation = max(abs(t - avg_time) for t in cycle_times)

        assert max_deviation < avg_time * 0.5, (
            f"Performance variation {max_deviation:.3f}s is too high for avg {avg_time:.3f}s"
        )


class TestMemoryManagement:
    """Test memory management and cleanup."""

    def test_should_cleanup_resources_after_processing(self):
        """Resources should be properly cleaned up after processing."""
        process = psutil.Process(os.getpid())
        initial_memory = process.memory_info().rss / 1024 / 1024  # MB

        # Process multiple articles
        extractor = TagExtractor()
        for i in range(100):
            extractor.extract_tags(f"Title {i}", f"Content for article number {i} with some text")

        # Force cleanup
        del extractor
        gc.collect()

        final_memory = process.memory_info().rss / 1024 / 1024  # MB
        memory_growth = final_memory - initial_memory

        # Memory growth should be minimal (under 200MB for 100 articles)
        assert memory_growth < 200, f"Memory leaked {memory_growth:.1f}MB after cleanup"

    def test_should_handle_memory_pressure_gracefully(self):
        """Should handle memory pressure without crashing."""
        extractor = TagExtractor()

        # Simulate processing under memory pressure
        large_text = "This is a very long article content. " * 1000  # ~35KB per article

        try:
            for i in range(50):  # Process 50 large articles
                tags = extractor.extract_tags(f"Large Article {i}", large_text)
                assert isinstance(tags, list), "Should continue processing under memory pressure"

                # Periodic cleanup
                if i % 10 == 0:
                    gc.collect()

        except MemoryError:
            pytest.fail("Should not raise MemoryError under normal processing load")


class TestConcurrencyPerformance:
    """Test concurrent processing performance."""

    def test_should_handle_concurrent_database_operations(self):
        """Should handle concurrent database operations safely."""
        mock_connections = [Mock() for _ in range(3)]

        for mock_conn in mock_connections:
            mock_cursor = Mock()
            mock_conn.cursor.return_value.__enter__.return_value = mock_cursor
            mock_cursor.fetchall.return_value = [(1, "tag1"), (2, "tag2")]

        inserter = TagInserter()
        results = []
        errors = []

        def concurrent_insert(conn, article_id):
            try:
                result = inserter.upsert_tags(conn, article_id, [f"tag-{article_id}"], "test-feed")
                results.append(result)
            except Exception as e:
                errors.append(e)

        # Run concurrent operations
        with ThreadPoolExecutor(max_workers=3) as executor:
            futures = []
            for i, conn in enumerate(mock_connections):
                future = executor.submit(concurrent_insert, conn, f"article-{i}")
                futures.append(future)

            # Wait for completion
            for future in futures:
                future.result(timeout=5)

        assert len(errors) == 0, f"Concurrent operations failed: {errors}"
        assert len(results) == 3, "All concurrent operations should complete"

    def test_should_scale_with_multiple_workers(self):
        """Performance should scale reasonably with multiple workers."""
        # This test would require actual threading implementation
        # For now, we'll test the concept with mocks

        extractor = TagExtractor()

        # Single-threaded baseline
        start_time = time.time()
        for i in range(10):
            extractor.extract_tags(f"Title {i}", f"Content {i}")
        single_thread_time = time.time() - start_time

        # Multi-threaded processing (simulated)
        def worker(articles):
            for title, content in articles:
                extractor.extract_tags(title, content)

        articles_batch1 = [(f"Title {i}", f"Content {i}") for i in range(5)]
        articles_batch2 = [(f"Title {i}", f"Content {i}") for i in range(5, 10)]

        start_time = time.time()
        with ThreadPoolExecutor(max_workers=2) as executor:
            future1 = executor.submit(worker, articles_batch1)
            future2 = executor.submit(worker, articles_batch2)

            future1.result()
            future2.result()
        multi_thread_time = time.time() - start_time

        # Multi-threading should provide some benefit (at least 10% faster)
        speedup_ratio = single_thread_time / multi_thread_time
        assert speedup_ratio > 1.1, f"Multi-threading speedup {speedup_ratio:.2f}x should be > 1.1x"


if __name__ == "__main__":
    # Run performance tests
    pytest.main([__file__, "-v", "--tb=short"])
