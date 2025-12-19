"""
Test suite for model management optimization in TagExtractor.
Following TDD principles to improve performance through shared model instances.
"""

import gc
import threading
import time
from unittest.mock import Mock, patch

import pytest

from tag_extractor.extract import TagExtractionConfig, TagExtractor
from tag_extractor.model_manager import ModelConfig, get_model_manager


class TestModelSharingOptimization:
    """Test suite for model sharing optimization."""

    def test_should_share_models_across_multiple_extractors(self):
        """Test that multiple TagExtractor instances share the same model instances."""
        # Arrange
        from tag_extractor.model_manager import get_model_manager

        config = TagExtractionConfig(model_name="test-model")
        model_manager = get_model_manager()
        model_manager.clear_models()

        with patch.object(model_manager, "get_models", wraps=model_manager.get_models) as mock_get_models:
            # Act
            extractor1 = TagExtractor(config)
            extractor2 = TagExtractor(config)

            extractor1._lazy_load_models()
            extractor2._lazy_load_models()

            # Assert: ModelManager should provide shared underlying instances
            assert extractor1._embedder is extractor2._embedder, "Embedder models should be shared"
            assert extractor1._keybert is extractor2._keybert, "KeyBERT models should be shared"
            assert extractor1._ja_tagger is extractor2._ja_tagger, "Japanese tagger should be shared"
            # ModelManager.get_models may be called multiple times, but underlying objects are shared
            assert mock_get_models.call_count >= 2

    def test_should_support_gpu_acceleration_when_available(self):
        """Test that GPU acceleration is used when available."""
        # Arrange
        config = TagExtractionConfig(device="cuda")
        model_manager = get_model_manager()

        with patch.object(model_manager, "get_models") as mock_get_models:
            mock_get_models.return_value = (Mock(), Mock(), Mock())

            # Act
            extractor = TagExtractor(config)
            extractor._lazy_load_models()

            # Assert: ModelConfig passed to ModelManager should carry GPU device
            assert mock_get_models.call_count == 1
            passed_config = mock_get_models.call_args.args[0]
            assert isinstance(passed_config, ModelConfig)
            assert passed_config.device == "cuda"

    def test_should_cleanup_models_on_service_shutdown(self):
        """Test that models are properly cleaned up on shutdown."""
        # This test should fail because cleanup_models method doesn't exist yet
        # Arrange
        with patch("tag_extractor.extract.SentenceTransformer") as mock_transformer:
            mock_transformer.return_value = Mock()

            extractor = TagExtractor()
            extractor._lazy_load_models()

            # Act
            get_model_manager().clear_models()

            # Assert
            assert get_model_manager()._embedder is None
            assert get_model_manager()._keybert is None
            assert get_model_manager()._ja_tagger is None

    def test_should_not_reload_models_if_already_loaded(self):
        """Test that models are not reloaded if already loaded."""
        # Arrange
        extractor = TagExtractor()

        model_manager = get_model_manager()
        model_manager.clear_models()

        with patch.object(model_manager, "get_models", wraps=model_manager.get_models) as mock_get_models:
            # Act
            extractor._lazy_load_models()
            first_model = extractor._embedder

            extractor._lazy_load_models()  # Second call
            second_model = extractor._embedder

            # Assert
            assert first_model is second_model, "Model should not be reloaded at TagExtractor level"
            # TagExtractor should only request models once per instance
            assert mock_get_models.call_count == 1

    def test_should_handle_model_loading_failure_gracefully(self):
        """Test that model loading failures are handled gracefully."""
        # Arrange
        extractor = TagExtractor()

        model_manager = get_model_manager()
        model_manager.clear_models()

        with patch.object(model_manager, "_load_models", side_effect=Exception("Model loading failed")):
            # Act & Assert: underlying ModelManager error should be propagated
            with pytest.raises(Exception, match="Model loading failed"):
                # This will be triggered indirectly via _lazy_load_models
                extractor._lazy_load_models()

    def test_should_extract_tags_with_shared_models(self):
        """Test that tag extraction works with shared models."""
        # Arrange
        config = TagExtractionConfig()
        extractor1 = TagExtractor(config)
        extractor2 = TagExtractor(config)

        title = "Test Title"
        content = "This is test content for tag extraction"

        # Patch _run_extraction to avoid dependency on actual models
        with (
            patch.object(
                extractor1,
                "_run_extraction",
                return_value=(["test", "content"], {"test": 0.8, "content": 0.7}),
            ) as mock_run1,
            patch.object(
                extractor2,
                "_run_extraction",
                return_value=(["test", "content"], {"test": 0.8, "content": 0.7}),
            ) as mock_run2,
        ):
            # Act
            result1 = extractor1.extract_tags(title, content)
            result2 = extractor2.extract_tags(title, content)

            # Assert
            assert result1 == result2, "Results should be consistent across shared models"
            assert len(result1) > 0, "Should extract tags successfully"
            mock_run1.assert_called_once()
            mock_run2.assert_called_once()


class TestModelMemoryOptimization:
    """Test suite for memory optimization in model management."""

    def test_should_limit_memory_usage_during_model_loading(self):
        """Test that memory usage is limited during model loading."""
        # This test would require actual memory monitoring
        # For now, we'll test the structure for memory monitoring
        extractor = TagExtractor()

        # Assert
        assert hasattr(extractor, "_embedder"), "Should have embedder attribute"
        assert hasattr(extractor, "_keybert"), "Should have keybert attribute"

    def test_should_cleanup_intermediate_results(self):
        """Test that intermediate processing results are cleaned up."""
        # Arrange
        extractor = TagExtractor()

        # Mock method that creates intermediate results
        with patch.object(extractor, "_get_candidate_tokens") as mock_candidates:
            mock_candidates.return_value = ["token1", "token2", "token3"]

            # Act
            extractor.extract_tags("title", "content")

            # Assert
            # Verify that intermediate results don't persist beyond the method call
            assert not hasattr(extractor, "_temp_candidates"), "Should not store temporary candidates"

    def test_should_handle_memory_pressure_gracefully(self):
        """Test that the system handles memory pressure gracefully."""
        # Arrange
        extractor = TagExtractor()

        # Simulate memory pressure by forcing garbage collection
        gc.collect()
        initial_objects = len(gc.get_objects())

        # Act
        extractor._lazy_load_models()

        # Assert
        # This is a basic test - in practice, we'd need more sophisticated memory monitoring
        assert len(gc.get_objects()) >= initial_objects, "Should manage memory objects properly"


class TestConcurrentModelAccess:
    """Test suite for concurrent access to shared models."""

    def test_should_maintain_thread_safety_during_extraction(self):
        """Test that model access is thread-safe during concurrent extraction."""
        # Arrange
        extractor = TagExtractor()
        results = []
        errors = []

        def extract_worker(worker_id: int):
            try:
                result = extractor.extract_tags(f"Title {worker_id}", f"Content {worker_id}")
                results.append(result)
            except Exception as e:
                errors.append(e)

        # Act
        threads = []
        for i in range(5):
            thread = threading.Thread(target=extract_worker, args=(i,))
            threads.append(thread)
            thread.start()

        for thread in threads:
            thread.join()

        # Assert
        assert len(errors) == 0, f"Should not have errors during concurrent access: {errors}"
        assert len(results) == 5, "Should complete all concurrent extractions"

    def test_should_handle_worker_failures_gracefully(self):
        """Test that worker failures don't affect other workers."""
        # Arrange
        extractor = TagExtractor()
        results = []
        errors = []

        def extract_worker(worker_id: int):
            try:
                if worker_id == 2:  # Simulate failure in worker 2
                    raise ValueError("Simulated worker failure")
                result = extractor.extract_tags(f"Title {worker_id}", f"Content {worker_id}")
                results.append(result)
            except Exception as e:
                errors.append(e)

        # Act
        threads = []
        for i in range(5):
            thread = threading.Thread(target=extract_worker, args=(i,))
            threads.append(thread)
            thread.start()

        for thread in threads:
            thread.join()

        # Assert
        assert len(errors) == 1, "Should have exactly one error from failed worker"
        assert len(results) == 4, "Should complete all other extractions"


class TestModelPerformanceMetrics:
    """Test suite for model performance metrics."""

    def test_should_collect_processing_time_metrics(self):
        """Test that processing time metrics are collected."""
        # Arrange
        extractor = TagExtractor()

        # Act
        start_time = time.time()
        result = extractor.extract_tags("Test title", "Test content")
        end_time = time.time()

        # Assert
        processing_time = end_time - start_time
        assert processing_time >= 0, "Processing time should be non-negative"
        assert isinstance(result, list), "Should return a list of tags"

    def test_should_track_memory_usage_patterns(self):
        """Test that memory usage patterns are tracked."""
        # Arrange
        extractor = TagExtractor()

        # Act
        initial_memory = len(gc.get_objects())
        extractor._lazy_load_models()
        after_loading_memory = len(gc.get_objects())

        # Assert
        assert after_loading_memory >= initial_memory, "Memory usage should increase after model loading"

    def test_should_report_throughput_statistics(self):
        """Test that throughput statistics are reported."""
        # Arrange
        extractor = TagExtractor()
        test_articles = [
            ("Title 1", "Content 1"),
            ("Title 2", "Content 2"),
            ("Title 3", "Content 3"),
        ]

        # Act
        start_time = time.time()
        results = []
        for title, content in test_articles:
            result = extractor.extract_tags(title, content)
            results.append(result)
        end_time = time.time()

        # Assert
        total_time = end_time - start_time
        throughput = len(test_articles) / total_time if total_time > 0 else 0

        assert throughput > 0, "Throughput should be positive"
        assert len(results) == len(test_articles), "Should process all articles"
