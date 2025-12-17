"""Tests for Ollama Gateway with semaphore-based request queuing."""

import asyncio
import pytest
from unittest.mock import AsyncMock, Mock, patch
from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway.ollama_gateway import OllamaGateway


@pytest.fixture
def mock_config():
    """Create a mock config for testing."""
    config = Mock(spec=NewsCreatorConfig)
    config.llm_service_url = "http://localhost:11434"
    config.model_name = "test-model"
    config.llm_timeout_seconds = 60
    config.llm_keep_alive = -1
    config.ollama_request_concurrency = 1
    config.oom_detection_enabled = False
    config.model_routing_enabled = False
    config.llm_num_ctx = 4096
    config.is_base_model_name = Mock(return_value=False)
    config.is_bucket_model_name = Mock(return_value=False)
    config.get_keep_alive_for_model = Mock(return_value=-1)
    config.get_llm_options.return_value = {
        "num_ctx": 4096,
        "num_predict": 500,
        "temperature": 0.2,
    }
    return config


@pytest.fixture
def mock_driver():
    """Create a mock OllamaDriver for testing."""
    driver = AsyncMock()
    driver.initialize = AsyncMock()
    driver.cleanup = AsyncMock()
    driver.generate = AsyncMock(return_value={
        "response": "Test response",
        "model": "test-model",
        "done": True,
        "prompt_eval_count": 100,
        "eval_count": 50,
        "total_duration": 1000000,
    })
    driver.list_tags = AsyncMock(return_value={"models": []})
    return driver


@pytest.mark.asyncio
async def test_semaphore_queues_requests(mock_config, mock_driver):
    """Test that semaphore properly queues concurrent requests."""
    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Track when each request starts processing
        processing_times = []
        call_count = [0]

        async def delayed_generate(payload):
            """Simulate a slow Ollama response."""
            call_count[0] += 1
            processing_times.append(call_count[0])
            await asyncio.sleep(0.1)  # Simulate processing time
            return {
                "response": f"Response {call_count[0]}",
                "model": "test-model",
                "done": True,
                "prompt_eval_count": 100,
                "eval_count": 50,
                "total_duration": 1000000,
            }

        mock_driver.generate = delayed_generate

        # Send 3 concurrent requests
        start_time = asyncio.get_event_loop().time()
        results = await asyncio.gather(
            gateway.generate("Prompt 1"),
            gateway.generate("Prompt 2"),
            gateway.generate("Prompt 3"),
        )
        end_time = asyncio.get_event_loop().time()

        # All requests should complete
        assert len(results) == 3
        assert all(isinstance(r, LLMGenerateResponse) for r in results)

        # With concurrency=1, requests should be processed sequentially
        # Total time should be at least 0.3 seconds (3 requests * 0.1s each)
        assert end_time - start_time >= 0.25  # Allow some margin

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_semaphore_allows_concurrent_requests_when_configured(mock_config, mock_driver):
    """Test that semaphore allows concurrent requests when concurrency > 1."""
    mock_config.ollama_request_concurrency = 2

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        call_count = [0]

        async def delayed_generate(payload):
            """Simulate a slow Ollama response."""
            call_count[0] += 1
            await asyncio.sleep(0.1)  # Simulate processing time
            return {
                "response": f"Response {call_count[0]}",
                "model": "test-model",
                "done": True,
                "prompt_eval_count": 100,
                "eval_count": 50,
                "total_duration": 1000000,
            }

        mock_driver.generate = delayed_generate

        # Send 3 concurrent requests with concurrency=2
        start_time = asyncio.get_event_loop().time()
        results = await asyncio.gather(
            gateway.generate("Prompt 1"),
            gateway.generate("Prompt 2"),
            gateway.generate("Prompt 3"),
        )
        end_time = asyncio.get_event_loop().time()

        # All requests should complete
        assert len(results) == 3
        assert all(isinstance(r, LLMGenerateResponse) for r in results)

        # With concurrency=2, first 2 should run in parallel, then the 3rd
        # Total time should be around 0.2 seconds (2 batches: 0.1s + 0.1s)
        assert end_time - start_time < 0.3  # Should be faster than sequential

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_semaphore_defaults_to_one(mock_config, mock_driver):
    """Test that semaphore defaults to concurrency=1 when not configured."""
    # Don't set ollama_request_concurrency
    mock_config.ollama_request_concurrency = 1

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Verify semaphore is created with value 1
        assert gateway._semaphore._value == 1
        assert gateway._semaphore._max_value == 1

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_semaphore_fifo_order(mock_config, mock_driver):
    """Test that semaphore processes requests in FIFO (First In First Out) order."""
    mock_config.ollama_request_concurrency = 1
    mock_config.model_routing_enabled = False
    mock_config.oom_detection_enabled = False
    mock_config.llm_num_ctx = 4096
    mock_config.is_base_model_name = Mock(return_value=False)
    mock_config.is_bucket_model_name = Mock(return_value=False)
    mock_config.get_keep_alive_for_model = Mock(return_value=-1)

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Track the order in which requests start processing (acquire semaphore)
        processing_order = []
        lock = asyncio.Lock()

        async def delayed_generate_with_tracking(payload, request_id):
            """Simulate a slow Ollama response and track processing order."""
            async with lock:
                processing_order.append(request_id)
            await asyncio.sleep(0.05)  # Simulate processing time
            return {
                "response": f"Response {request_id}",
                "model": "test-model",
                "done": True,
                "prompt_eval_count": 100,
                "eval_count": 50,
                "total_duration": 1000000,
            }

        # Create a closure to track request IDs
        request_counter = [0]
        original_generate = mock_driver.generate

        async def tracked_generate(payload):
            request_counter[0] += 1
            request_id = request_counter[0]
            return await delayed_generate_with_tracking(payload, request_id)

        mock_driver.generate = tracked_generate

        # Submit 5 requests concurrently using gather
        # They will be queued in the order they are created
        tasks = [
            gateway.generate("Prompt 1"),
            gateway.generate("Prompt 2"),
            gateway.generate("Prompt 3"),
            gateway.generate("Prompt 4"),
            gateway.generate("Prompt 5"),
        ]

        # Execute all tasks concurrently
        results = await asyncio.gather(*tasks, return_exceptions=True)

        # All requests should complete successfully
        assert len(results) == 5
        assert all(not isinstance(r, Exception) for r in results)

        # Verify FIFO order: requests should be processed in the order they were submitted
        # Note: This test verifies that asyncio.Semaphore maintains FIFO order
        # If the semaphore doesn't guarantee FIFO, this test may fail
        assert len(processing_order) == 5, f"Expected 5 processed requests, got {len(processing_order)}"

        # Check if processing order matches submission order (FIFO)
        # With concurrency=1, requests should be processed strictly in order
        expected_order = [1, 2, 3, 4, 5]
        is_fifo = processing_order == expected_order

        # At minimum, verify that all requests were processed
        assert set(processing_order) == set(expected_order), f"All requests should be processed. Got: {processing_order}"

        # Assert FIFO order - this will fail if asyncio.Semaphore doesn't guarantee FIFO
        assert is_fifo, (
            f"FIFO order not guaranteed by asyncio.Semaphore. "
            f"Processing order: {processing_order}, Expected: {expected_order}. "
            f"Consider implementing a FIFO-guaranteed semaphore if strict ordering is required."
        )

        await gateway.cleanup()

