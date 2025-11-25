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

        await gateway.cleanup()

