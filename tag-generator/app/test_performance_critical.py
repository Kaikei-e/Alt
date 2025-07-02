"""
Critical performance tests that drive TDD refactoring.
These tests will FAIL initially and drive the refactoring process.
"""

import pytest
import time
import threading
from unittest.mock import Mock, patch
from concurrent.futures import ThreadPoolExecutor, as_completed
import psutil
import os
import gc

from tag_extractor.extract import TagExtractor
from tag_inserter.upsert_tags import TagInserter
from main import TagGeneratorService


class TestCriticalPerformanceRequirements:
    """Critical performance tests that must pass after refactoring."""

    def test_model_singleton_performance(self):
        """TagExtractor should reuse model instance across threads - FAILING TEST."""
        # Test that the model manager loads models only once across multiple extractors

        from tag_extractor.model_manager import get_model_manager

        # Clear any previously loaded models
        model_manager = get_model_manager()
        model_manager.clear_models()

        results = []
        errors = []

        def create_extractor_and_extract():
            try:
                extractor = TagExtractor()
                tags = extractor.extract_tags(
                    "Test title", "Test content for tag extraction testing"
                )
                results.append(
                    {
                        "extractor_id": id(extractor),
                        "tags": tags,
                        "models_loaded": model_manager.is_loaded(),
                    }
                )
            except Exception as e:
                errors.append(e)

        # Track model loading at the manager level
        with patch.object(
            model_manager, "_load_models", wraps=model_manager._load_models
        ) as mock_load_models:
            # Create multiple extractors concurrently
            threads = []
            for _ in range(5):
                thread = threading.Thread(target=create_extractor_and_extract)
                threads.append(thread)
                thread.start()

            for thread in threads:
                thread.join()

            assert len(errors) == 0, f"Errors occurred: {errors}"
            assert len(results) == 5, "All extractions should complete"

            # CRITICAL: Models should be loaded only once by the singleton manager
            assert mock_load_models.call_count <= 1, (
                f"Models loaded {mock_load_models.call_count} times, should be ≤ 1 (shared singleton)"
            )

            # All extractors should have access to loaded models
            all_have_models = all(r["models_loaded"] for r in results)
            assert all_have_models, "All extractors should have access to loaded models"

    def test_connection_pool_performance(self):
        """Database connections should be pooled and reused - FAILING TEST."""
        # This test will fail initially because connections are created/closed per cycle

        service = TagGeneratorService()

        with patch("main.psycopg2.connect") as mock_connect:
            mock_conn = Mock()
            mock_connect.return_value = mock_conn

            # Mock successful processing
            with patch.object(service, "_process_article_batch") as mock_process:
                mock_process.return_value = {
                    "total_processed": 5,
                    "successful": 5,
                    "failed": 0,
                }

                # Run multiple cycles rapidly
                for _ in range(10):
                    service.run_processing_cycle()

        # CRITICAL: This will fail initially - should create fewer connections with pooling
        assert mock_connect.call_count <= 3, (
            f"Created {mock_connect.call_count} connections, should use connection pooling"
        )

    def test_batch_processing_efficiency(self):
        """Batch processing should be significantly faster than individual - FAILING TEST."""
        # This test will fail initially due to inefficient batching

        mock_conn = Mock()
        mock_cursor = Mock()
        mock_cursor.mogrify = Mock(return_value=b"INSERT...")
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.__enter__ = Mock(return_value=mock_cursor)
        mock_cursor.__exit__ = Mock(return_value=None)
        mock_cursor.fetchall.return_value = [(i, f"tag-{i}") for i in range(1, 101)]

        inserter = TagInserter()

        # Individual processing baseline
        start_time = time.time()
        for i in range(50):
            inserter.upsert_tags(
                mock_conn, f"article-{i}", [f"tag-{i}", f"tag-{i + 50}"]
            )
        individual_time = time.time() - start_time

        # Reset mock counters
        mock_conn.reset_mock()
        mock_cursor.reset_mock()

        # Batch processing
        batch_data = [
            {"article_id": f"article-{i}", "tags": [f"tag-{i}", f"tag-{i + 50}"]}
            for i in range(50)
        ]

        start_time = time.time()
        inserter.batch_upsert_tags(mock_conn, batch_data)
        batch_time = time.time() - start_time

        # CRITICAL: This may fail initially - batch should be at least 5x faster
        speedup = individual_time / batch_time
        assert speedup >= 5.0, (
            f"Batch speedup {speedup:.1f}x should be ≥ 5x (got individual: {individual_time:.3f}s, batch: {batch_time:.3f}s)"
        )

    def test_memory_growth_control(self):
        """Memory growth should be controlled during processing - FAILING TEST."""
        # This test may fail initially due to memory leaks

        process = psutil.Process(os.getpid())
        initial_memory = process.memory_info().rss / 1024 / 1024  # MB

        extractor = TagExtractor()

        # Process many articles to stress test memory
        for batch in range(10):  # 10 batches
            for i in range(50):  # 50 articles per batch
                content = (
                    f"This is article {i} in batch {batch}. " * 100
                )  # ~3KB per article
                extractor.extract_tags(f"Title {batch}-{i}", content)

            # Force garbage collection after each batch
            gc.collect()

        final_memory = process.memory_info().rss / 1024 / 1024  # MB
        memory_growth = final_memory - initial_memory

        # CRITICAL: Memory growth should be controlled with singleton models
        # With singleton models, memory growth should be significantly less than before
        assert memory_growth < 300, (
            f"Memory grew by {memory_growth:.1f}MB processing 500 articles, should be < 300MB (improved from previous 400MB+)"
        )

    def test_concurrent_extraction_scalability(self):
        """Concurrent extraction should scale with available cores - FAILING TEST."""
        # This test may fail initially without proper thread safety

        extractor = TagExtractor()
        num_workers = min(4, os.cpu_count() or 1)  # Use up to 4 workers
        articles_per_worker = 25

        # Sequential baseline
        start_time = time.time()
        for i in range(num_workers * articles_per_worker):
            extractor.extract_tags(f"Title {i}", f"Content for article {i}")
        sequential_time = time.time() - start_time

        # Concurrent processing
        def worker_task(worker_id):
            results = []
            for i in range(articles_per_worker):
                article_id = worker_id * articles_per_worker + i
                tags = extractor.extract_tags(
                    f"Title {article_id}", f"Content for article {article_id}"
                )
                results.append(tags)
            return results

        start_time = time.time()
        with ThreadPoolExecutor(max_workers=num_workers) as executor:
            futures = [executor.submit(worker_task, i) for i in range(num_workers)]
            all_results = []
            for future in as_completed(futures):
                all_results.extend(future.result())
        concurrent_time = time.time() - start_time

        # CRITICAL: This may fail initially - should get some speedup from concurrency
        speedup = sequential_time / concurrent_time
        expected_min_speedup = min(2.0, num_workers * 0.7)  # At least 70% efficiency
        assert speedup >= expected_min_speedup, (
            f"Concurrent speedup {speedup:.1f}x should be ≥ {expected_min_speedup:.1f}x"
        )
        assert len(all_results) == num_workers * articles_per_worker, (
            "All articles should be processed"
        )

    def test_database_transaction_efficiency(self):
        """Database operations should minimize transaction overhead - FAILING TEST."""
        # This test will fail initially due to inefficient transaction handling

        mock_conn = Mock()
        mock_cursor = Mock()
        mock_cursor.mogrify = Mock(return_value=b"INSERT...")
        mock_conn.cursor.return_value = mock_cursor
        mock_cursor.__enter__ = Mock(return_value=mock_cursor)
        mock_cursor.__exit__ = Mock(return_value=None)
        mock_cursor.fetchall.return_value = [(i, f"tag-{i}") for i in range(1, 501)]

        inserter = TagInserter()

        # Large batch that should use minimal transactions
        large_batch = []
        for i in range(200):  # 200 articles
            large_batch.append(
                {
                    "article_id": f"article-{i}",
                    "tags": [f"tag-{i}", f"tag-{i + 200}", f"tag-{i + 400}"],
                }
            )

        start_time = time.time()
        result = inserter.batch_upsert_tags(mock_conn, large_batch)
        processing_time = time.time() - start_time

        # CRITICAL: This may fail initially - large batch should be very fast
        assert processing_time < 0.5, (
            f"Large batch took {processing_time:.3f}s, should be < 0.5s"
        )
        assert result["success"] is True, "Large batch should succeed"

        # Should minimize database calls
        # Ideally: 1 call for tags, 1 for tag IDs, 1 for relationships, 1 commit
        total_db_calls = (
            mock_cursor.execute.call_count
            + mock_cursor.fetchall.call_count
            + mock_conn.commit.call_count
        )
        assert total_db_calls <= 10, (
            f"Used {total_db_calls} DB calls, should use ≤ 10 for efficient batching"
        )


class TestPerformanceRegressionPrevention:
    """Tests to prevent performance regressions after refactoring."""

    def test_single_article_processing_speed(self):
        """Single article processing should complete quickly."""
        extractor = TagExtractor()

        start_time = time.time()
        tags = extractor.extract_tags(
            "Technology Advances in Machine Learning",
            "Machine learning algorithms are becoming more sophisticated with advances in neural networks and deep learning.",
        )
        processing_time = time.time() - start_time

        assert processing_time < 2.0, (
            f"Single article took {processing_time:.3f}s, should be < 2s"
        )
        assert isinstance(tags, list), "Should return list of tags"
        assert len(tags) > 0, "Should extract some tags"

    def test_service_initialization_speed(self):
        """Service initialization should be fast."""
        start_time = time.time()
        TagGeneratorService()
        init_time = time.time() - start_time

        assert init_time < 1.0, (
            f"Service initialization took {init_time:.3f}s, should be < 1s"
        )

    def test_database_connection_speed(self):
        """Database connection creation should be fast."""
        service = TagGeneratorService()

        with patch("main.psycopg2.connect") as mock_connect:
            mock_connect.return_value = Mock()

            with patch.object(service, "_get_database_dsn", return_value="test-dsn"):
                start_time = time.time()
                conn = service._create_direct_connection()
                connection_time = time.time() - start_time

                assert connection_time < 0.5, (
                    f"Connection took {connection_time:.3f}s, should be < 0.5s"
                )
                assert conn is not None, "Should return valid connection"


if __name__ == "__main__":
    # Run critical performance tests
    pytest.main([__file__, "-v", "--tb=short", "-x"])  # Stop on first failure
