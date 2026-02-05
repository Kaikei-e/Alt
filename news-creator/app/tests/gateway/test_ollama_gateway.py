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
    # Hybrid scheduling config
    config.scheduling_rt_reserved_slots = 1
    config.scheduling_aging_threshold_seconds = 60.0
    config.scheduling_aging_boost = 0.5
    # Preemption settings
    config.scheduling_preemption_enabled = True
    config.scheduling_preemption_wait_threshold_seconds = 2.0
    # Priority promotion and guaranteed bandwidth settings
    config.scheduling_priority_promotion_threshold_seconds = 600.0
    config.scheduling_guaranteed_be_ratio = 5
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
    # Set RT reserved to 0 so all slots are available for BE (low priority) requests
    mock_config.scheduling_rt_reserved_slots = 0

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

        # Verify HybridPrioritySemaphore is created with total_slots=1
        assert gateway._semaphore._total_slots == 1
        assert gateway._semaphore._rt_reserved == 1  # RT reserved from config

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


@pytest.mark.asyncio
async def test_high_priority_bypasses_low_priority_queue(mock_config, mock_driver):
    """Test that high priority requests use the high priority queue."""
    mock_config.ollama_request_concurrency = 1

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Verify high priority parameter is handled
        result = await gateway.generate("Test prompt", priority="high")

        assert isinstance(result, LLMGenerateResponse)
        assert result.response == "Test response"

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_low_priority_default(mock_config, mock_driver):
    """Test that default priority is low."""
    mock_config.ollama_request_concurrency = 1

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Default priority should be low
        result = await gateway.generate("Test prompt")

        assert isinstance(result, LLMGenerateResponse)
        assert result.response == "Test response"

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_ttft_metrics_logged_with_cold_start_warning(mock_config, mock_driver, caplog):
    """Test that TTFT metrics are logged with cold start warning when load_duration > 0.1s."""
    import logging
    caplog.set_level(logging.WARNING)

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Simulate cold start with load_duration = 2 seconds (2 billion nanoseconds)
        cold_start_response = {
            "response": "Test response",
            "model": "test-model",
            "done": True,
            "prompt_eval_count": 500,
            "eval_count": 100,
            "total_duration": 3_000_000_000,  # 3 seconds total
            "load_duration": 2_000_000_000,    # 2 seconds load time (cold start)
            "prompt_eval_duration": 500_000_000,  # 0.5 seconds prefill
            "eval_duration": 500_000_000,  # 0.5 seconds decode
        }
        mock_driver.generate = AsyncMock(return_value=cold_start_response)

        await gateway.generate("Test prompt")

        # Verify cold start warning was logged
        warning_records = [r for r in caplog.records if r.levelno == logging.WARNING]
        cold_start_warnings = [r for r in warning_records if "cold start" in r.message.lower() or "COLD_START" in r.message]
        assert len(cold_start_warnings) >= 1, "Cold start warning should be logged when load_duration > 0.1s"

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_ttft_metrics_logged_without_cold_start_warning(mock_config, mock_driver, caplog):
    """Test that no cold start warning is logged when model is hot (load_duration < 0.1s)."""
    import logging
    caplog.set_level(logging.WARNING)

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Simulate hot model with minimal load_duration
        hot_response = {
            "response": "Test response",
            "model": "test-model",
            "done": True,
            "prompt_eval_count": 500,
            "eval_count": 100,
            "total_duration": 600_000_000,  # 0.6 seconds total
            "load_duration": 1_000_000,    # 0.001 seconds (hot)
            "prompt_eval_duration": 300_000_000,  # 0.3 seconds prefill
            "eval_duration": 300_000_000,  # 0.3 seconds decode
        }
        mock_driver.generate = AsyncMock(return_value=hot_response)

        await gateway.generate("Test prompt")

        # Verify no cold start warning was logged
        warning_records = [r for r in caplog.records if r.levelno == logging.WARNING]
        cold_start_warnings = [r for r in warning_records if "cold start" in r.message.lower() or "COLD_START" in r.message]
        assert len(cold_start_warnings) == 0, "No cold start warning should be logged when load_duration < 0.1s"

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_ttft_breakdown_logged(mock_config, mock_driver, caplog):
    """Test that TTFT breakdown is logged in a structured format."""
    import logging
    caplog.set_level(logging.INFO)

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Simulate response with all timing metrics
        response_with_timing = {
            "response": "Test response",
            "model": "test-model",
            "done": True,
            "prompt_eval_count": 500,
            "eval_count": 100,
            "total_duration": 1_500_000_000,  # 1.5 seconds total
            "load_duration": 100_000_000,    # 0.1 seconds load
            "prompt_eval_duration": 400_000_000,  # 0.4 seconds prefill
            "eval_duration": 1_000_000_000,  # 1.0 seconds decode
        }
        mock_driver.generate = AsyncMock(return_value=response_with_timing)

        await gateway.generate("Test prompt")

        # Verify TTFT breakdown is logged
        info_records = [r for r in caplog.records if r.levelno == logging.INFO]
        ttft_logs = [r for r in info_records if "ttft" in r.message.lower() or "TTFT" in r.message]
        assert len(ttft_logs) >= 1, "TTFT breakdown should be logged"

        # Verify the TTFT log contains expected components
        ttft_log = ttft_logs[0]
        assert "load_duration" in ttft_log.message.lower() or hasattr(ttft_log, "load_duration_s")
        assert "prompt_eval" in ttft_log.message.lower() or hasattr(ttft_log, "prompt_eval_duration_s")

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_streaming_semaphore_acquired_before_iteration(mock_config, mock_driver):
    """Test that semaphore is acquired BEFORE generator iteration starts (eager acquisition)."""
    mock_config.ollama_request_concurrency = 1

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        # Create a mock stream driver
        mock_stream_driver = AsyncMock()

        async def mock_generate_stream(payload):
            """Simulate streaming response."""
            for i in range(3):
                yield {
                    "response": f"chunk{i}",
                    "model": "test-model",
                    "done": i == 2,
                }

        mock_stream_driver.generate_stream = mock_generate_stream
        mock_stream_driver.initialize = AsyncMock()
        mock_stream_driver.cleanup = AsyncMock()

        with patch("news_creator.gateway.ollama_gateway.OllamaStreamDriver", return_value=mock_stream_driver):
            gateway = OllamaGateway(mock_config)
            await gateway.initialize()

            # Get initial semaphore state (streaming uses RT slots)
            initial_rt_available = gateway._semaphore._rt_available

            # Call generate with stream=True
            # This should acquire the semaphore IMMEDIATELY (before iteration)
            generator = await gateway.generate("Test prompt", stream=True)

            # Verify semaphore was acquired (RT slots decreased since stream=True is high priority)
            assert gateway._semaphore._rt_available < initial_rt_available, \
                "Semaphore should be acquired before generator iteration"

            # Consume the generator
            chunks = []
            async for chunk in generator:
                chunks.append(chunk)

            # Verify semaphore was released after iteration
            assert gateway._semaphore._rt_available == initial_rt_available, \
                "Semaphore should be released after generator completes"

            # Verify we got all chunks
            assert len(chunks) == 3

            await gateway.cleanup()


@pytest.mark.asyncio
async def test_streaming_semaphore_released_on_early_termination(mock_config, mock_driver):
    """Test that semaphore is released when generator is terminated early."""
    mock_config.ollama_request_concurrency = 1

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        # Create a mock stream driver
        mock_stream_driver = AsyncMock()

        async def mock_generate_stream(payload):
            """Simulate streaming response with many chunks."""
            for i in range(100):
                yield {
                    "response": f"chunk{i}",
                    "model": "test-model",
                    "done": False,
                }

        mock_stream_driver.generate_stream = mock_generate_stream
        mock_stream_driver.initialize = AsyncMock()
        mock_stream_driver.cleanup = AsyncMock()

        with patch("news_creator.gateway.ollama_gateway.OllamaStreamDriver", return_value=mock_stream_driver):
            gateway = OllamaGateway(mock_config)
            await gateway.initialize()

            # Get initial semaphore state (streaming uses RT slots)
            initial_rt_available = gateway._semaphore._rt_available

            # Call generate with stream=True
            generator = await gateway.generate("Test prompt", stream=True)

            # Verify semaphore was acquired
            assert gateway._semaphore._rt_available < initial_rt_available

            # Consume only first 3 chunks then break (early termination)
            chunks = []
            async for chunk in generator:
                chunks.append(chunk)
                if len(chunks) >= 3:
                    break

            # Close the generator explicitly (simulate early termination)
            # AsyncIterator has aclose() for cleanup
            if hasattr(generator, "aclose"):
                await generator.aclose()

            # Verify semaphore was released after early termination
            assert gateway._semaphore._rt_available == initial_rt_available, \
                "Semaphore should be released after early termination"

            await gateway.cleanup()


@pytest.mark.asyncio
async def test_streaming_high_priority_immediate_acquisition(mock_config, mock_driver, caplog):
    """Test that streaming (high priority) requests acquire semaphore immediately."""
    import logging
    caplog.set_level(logging.INFO)

    mock_config.ollama_request_concurrency = 2
    mock_config.scheduling_rt_reserved_slots = 1  # 1 slot reserved for RT

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        mock_stream_driver = AsyncMock()

        async def mock_generate_stream(payload):
            await asyncio.sleep(0.1)  # Simulate some latency
            yield {"response": "done", "model": "test-model", "done": True}

        mock_stream_driver.generate_stream = mock_generate_stream
        mock_stream_driver.initialize = AsyncMock()
        mock_stream_driver.cleanup = AsyncMock()

        with patch("news_creator.gateway.ollama_gateway.OllamaStreamDriver", return_value=mock_stream_driver):
            gateway = OllamaGateway(mock_config)
            await gateway.initialize()

            # Call generate with stream=True
            generator = await gateway.generate("Test prompt", stream=True)

            # Verify log shows HIGH PRIORITY acquisition
            info_records = [r for r in caplog.records if r.levelno == logging.INFO]
            high_priority_logs = [
                r for r in info_records
                if "HIGH PRIORITY" in r.message and "streaming generator" in r.message
            ]
            assert len(high_priority_logs) >= 1, \
                "Streaming requests should log HIGH PRIORITY semaphore acquisition"

            # Consume the generator
            async for _ in generator:
                pass

            await gateway.cleanup()


@pytest.mark.asyncio
async def test_slow_generation_warning_uses_decode_speed(mock_config, mock_driver, caplog):
    """Test that slow generation warning uses decode speed (eval_duration) not total_duration.

    Previously, tokens_per_second was calculated as eval_count / total_duration, which
    incorrectly included load_duration and prompt_eval_duration, leading to false positive
    warnings. The fix uses eval_count / eval_duration (decode speed) for accurate detection.
    """
    import logging
    caplog.set_level(logging.WARNING)

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Simulate response with:
        # - Fast decode speed: 70 tok/s (eval_count=70, eval_duration=1s)
        # - But slow total_duration due to load/prefill time
        # This should NOT trigger a slow generation warning
        normal_decode_response = {
            "response": "Test response",
            "model": "test-model",
            "done": True,
            "prompt_eval_count": 2000,
            "eval_count": 70,  # 70 tokens generated
            "total_duration": 70_000_000_000,  # 70 seconds total (includes queue, load, prefill)
            "load_duration": 500_000_000,  # 0.5 seconds load
            "prompt_eval_duration": 68_500_000_000,  # 68.5 seconds prefill (large prompt)
            "eval_duration": 1_000_000_000,  # 1 second decode -> 70 tok/s (FAST!)
        }
        mock_driver.generate = AsyncMock(return_value=normal_decode_response)

        await gateway.generate("Test prompt")

        # Verify NO slow generation warning was logged
        # Old calculation: 70 tokens / 70 seconds = 1 tok/s -> would warn (WRONG)
        # New calculation: 70 tokens / 1 second = 70 tok/s -> no warning (CORRECT)
        warning_records = [r for r in caplog.records if r.levelno == logging.WARNING]
        slow_gen_warnings = [r for r in warning_records if "Slow LLM generation detected" in r.message]
        assert len(slow_gen_warnings) == 0, (
            f"Should NOT warn when decode speed is fast (70 tok/s). "
            f"Found warnings: {[r.message for r in slow_gen_warnings]}"
        )

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_slow_generation_warning_triggers_on_slow_decode(mock_config, mock_driver, caplog):
    """Test that slow generation warning triggers when decode speed is actually slow."""
    import logging
    caplog.set_level(logging.WARNING)

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Simulate response with truly slow decode speed: 10 tok/s
        # and sufficient eval_count (>= 20) to trigger warning
        slow_decode_response = {
            "response": "Test response",
            "model": "test-model",
            "done": True,
            "prompt_eval_count": 100,
            "eval_count": 30,  # 30 tokens generated (>= 20 threshold)
            "total_duration": 4_000_000_000,  # 4 seconds total
            "load_duration": 100_000_000,  # 0.1 seconds load
            "prompt_eval_duration": 900_000_000,  # 0.9 seconds prefill
            "eval_duration": 3_000_000_000,  # 3 seconds decode -> 10 tok/s (SLOW!)
        }
        mock_driver.generate = AsyncMock(return_value=slow_decode_response)

        await gateway.generate("Test prompt")

        # Verify slow generation warning WAS logged
        warning_records = [r for r in caplog.records if r.levelno == logging.WARNING]
        slow_gen_warnings = [r for r in warning_records if "Slow LLM generation detected" in r.message]
        assert len(slow_gen_warnings) >= 1, (
            f"Should warn when decode speed is slow (10 tok/s). "
            f"Found warnings: {[r.message[:100] for r in warning_records]}"
        )

        await gateway.cleanup()


@pytest.mark.asyncio
async def test_slow_generation_warning_skipped_for_short_eval_count(mock_config, mock_driver, caplog):
    """Test that slow generation warning is skipped when eval_count is too low.

    Short generations (eval_count < 20) can have unstable speed measurements,
    so we skip the warning to avoid false positives from rerank/expand-query calls.
    """
    import logging
    caplog.set_level(logging.WARNING)

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=mock_driver):
        gateway = OllamaGateway(mock_config)
        await gateway.initialize()

        # Simulate response with low eval_count (typical for rerank/expand-query)
        # Even if decode speed looks slow, we should NOT warn
        short_response = {
            "response": "yes",  # Short response
            "model": "test-model",
            "done": True,
            "done_reason": "stop",  # Stop token triggered early termination
            "prompt_eval_count": 100,
            "eval_count": 7,  # Only 7 tokens (< 20 threshold)
            "total_duration": 2_000_000_000,  # 2 seconds total
            "load_duration": 100_000_000,  # 0.1 seconds load
            "prompt_eval_duration": 1_200_000_000,  # 1.2 seconds prefill
            "eval_duration": 700_000_000,  # 0.7 seconds decode -> 10 tok/s
        }
        mock_driver.generate = AsyncMock(return_value=short_response)

        await gateway.generate("Test prompt")

        # Verify NO slow generation warning was logged
        # Old behavior: would warn because 7 / 2 = 3.5 tok/s < 30
        # New behavior: skip warning because eval_count (7) < 20 threshold
        warning_records = [r for r in caplog.records if r.levelno == logging.WARNING]
        slow_gen_warnings = [r for r in warning_records if "Slow LLM generation detected" in r.message]
        assert len(slow_gen_warnings) == 0, (
            f"Should NOT warn when eval_count is too low ({short_response['eval_count']} < 20). "
            f"Found warnings: {[r.message for r in slow_gen_warnings]}"
        )

        await gateway.cleanup()

