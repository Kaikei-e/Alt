"""Branch characterization tests for OllamaGateway.generate."""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from typing import Any
from unittest.mock import AsyncMock, Mock, patch

import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway.ollama_gateway import OllamaGateway

_DEFAULT_NUM_CTX = 4096
_DEFAULT_NUM_PREDICT = 500
_EXPLICIT_NUM_PREDICT = 256
_EXPLICIT_KEEP_ALIVE = "30m"
_MODEL_SPECIFIC_KEEP_ALIVE = "24h"
_ROUTED_BUCKET_SIZE = 16384
_LARGE_UNKNOWN_PROMPT_CHARS = 65_000
_SMALL_UNKNOWN_PROMPT_CHARS = 100
_ABNORMAL_PROMPT_CHARS = 100_050
_ZERO_CONTEXT_WINDOW = 0
_PROMPT_PREFIX = "<|turn>user\n"
_PROMPT_SUFFIX = "<turn|>\n<|turn>model\n"


def _make_config(*, routing_enabled: bool = False) -> Mock:
    config = Mock(spec=NewsCreatorConfig)
    config.llm_service_url = "http://localhost:11434"
    config.model_name = "test-model"
    config.llm_timeout_seconds = 60
    config.llm_keep_alive = -1
    config.llm = Mock(inference_service_token=None)
    config.ollama_request_concurrency = 1
    config.oom_detection_enabled = False
    config.model_routing_enabled = routing_enabled
    config.scheduling_rt_reserved_slots = 1
    config.scheduling_aging_threshold_seconds = 60.0
    config.scheduling_aging_boost = 0.5
    config.scheduling_preemption_enabled = True
    config.scheduling_preemption_wait_threshold_seconds = 2.0
    config.scheduling_priority_promotion_threshold_seconds = 600.0
    config.scheduling_guaranteed_be_ratio = 5
    config.max_queue_depth = 0
    config.scheduling_rt_mode = "fifo"
    config.scheduling_aging_sweep_interval_releases = 8
    config.llm_num_ctx = _DEFAULT_NUM_CTX
    config.is_base_model_name = Mock(return_value=False)
    config.is_bucket_model_name = Mock(return_value=False)
    config.get_keep_alive_for_model = Mock(return_value=_MODEL_SPECIFIC_KEEP_ALIVE)
    config.get_llm_options.return_value = {
        "num_ctx": _DEFAULT_NUM_CTX,
        "num_predict": _DEFAULT_NUM_PREDICT,
        "temperature": 0.2,
        "num_gpu": 99,
    }
    return config


def _make_driver(response: dict[str, Any] | None = None) -> AsyncMock:
    driver = AsyncMock()
    driver.initialize = AsyncMock()
    driver.cleanup = AsyncMock()
    driver.generate = AsyncMock(
        return_value=response
        or {
            "response": "ok",
            "model": "test-model",
            "done": True,
        }
    )
    driver.list_tags = AsyncMock(return_value={"models": []})
    return driver


def _wrap_prompt(body: str) -> str:
    return f"{_PROMPT_PREFIX}{body}{_PROMPT_SUFFIX}"


@pytest.mark.asyncio
async def test_generate_routing_enabled_none_model_uses_router() -> None:
    """When routing is enabled and model is None, ModelRouter selects the model."""
    config = _make_config(routing_enabled=True)
    driver = _make_driver({"response": "ok", "model": "gemma4-e4b-16k", "done": True})

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        gateway.model_router.select_model = Mock(
            return_value=("gemma4-e4b-16k", _ROUTED_BUCKET_SIZE)
        )
        gateway.model_router._current_bucket = _ROUTED_BUCKET_SIZE
        await gateway.initialize()
        try:
            result = await gateway.generate(
                _wrap_prompt("hello"),
                model=None,
                num_predict=_EXPLICIT_NUM_PREDICT,
                keep_alive=_EXPLICIT_KEEP_ALIVE,
                format="json",
            )
            assert isinstance(result, LLMGenerateResponse)
            assert result.model == "gemma4-e4b-16k"
            sent_payload = driver.generate.call_args.args[0]
            assert sent_payload["model"] == "gemma4-e4b-16k"
            assert sent_payload["keep_alive"] == _EXPLICIT_KEEP_ALIVE
            assert sent_payload["format"] == "json"
            assert sent_payload["options"]["num_predict"] == _EXPLICIT_NUM_PREDICT
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_generate_routing_enabled_base_model_auto_maps(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """When routing is enabled and a base model name is passed, it auto-maps via router."""
    caplog.set_level(logging.INFO)
    config = _make_config(routing_enabled=True)
    config.is_base_model_name = Mock(return_value=True)
    driver = _make_driver({"response": "ok", "model": "gemma4-e4b-16k", "done": True})

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        gateway.model_router.select_model = Mock(
            return_value=("gemma4-e4b-16k", _ROUTED_BUCKET_SIZE)
        )
        await gateway.initialize()
        try:
            result = await gateway.generate(
                _wrap_prompt("hello"), model="gemma4-e4b-q4km"
            )
            assert isinstance(result, LLMGenerateResponse)
            sent_payload = driver.generate.call_args.args[0]
            assert sent_payload["model"] == "gemma4-e4b-16k"
            assert any(
                "auto-mapped to bucket model" in rec.message for rec in caplog.records
            )
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_generate_routing_enabled_bucket_model_used_as_is(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """When routing is enabled and a bucket model name is passed, it is used as-is."""
    caplog.set_level(logging.DEBUG)
    config = _make_config(routing_enabled=True)
    config.is_bucket_model_name = Mock(return_value=True)
    driver = _make_driver({"response": "ok", "model": "gemma4-e4b-12k", "done": True})

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        gateway.model_router.select_model = Mock()
        await gateway.initialize()
        try:
            result = await gateway.generate(
                _wrap_prompt("hello"), model="gemma4-e4b-12k"
            )
            assert isinstance(result, LLMGenerateResponse)
            gateway.model_router.select_model.assert_not_called()
            sent_payload = driver.generate.call_args.args[0]
            assert sent_payload["model"] == "gemma4-e4b-12k"
            assert any(
                "Bucket model explicitly specified" in rec.message
                for rec in caplog.records
            )
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_generate_routing_enabled_unknown_model_small_vs_large_prompt(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Unknown model warns only when estimated tokens exceed 16k."""
    config = _make_config(routing_enabled=True)
    driver = _make_driver({"response": "ok", "model": "custom-model", "done": True})

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        await gateway.initialize()
        try:
            caplog.clear()
            caplog.set_level(logging.WARNING)
            await gateway.generate(
                _wrap_prompt("x" * _SMALL_UNKNOWN_PROMPT_CHARS), model="custom-model"
            )
            assert not any(
                "Large prompt detected with unknown model" in rec.message
                for rec in caplog.records
            )

            caplog.clear()
            await gateway.generate(
                _wrap_prompt("x" * _LARGE_UNKNOWN_PROMPT_CHARS), model="custom-model"
            )
            assert any(
                "Large prompt detected with unknown model" in rec.message
                for rec in caplog.records
            )
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_generate_abnormal_prompt_size_and_zero_context_window(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Prompts over 100k chars emit an ERROR log and zero context_window does not divide by zero."""
    caplog.set_level(logging.ERROR)
    config = _make_config(routing_enabled=False)
    config.llm_num_ctx = _ZERO_CONTEXT_WINDOW
    driver = _make_driver({"response": "ok", "model": "test-model", "done": True})

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        await gateway.initialize()
        try:
            result = await gateway.generate(
                _wrap_prompt("a" * _ABNORMAL_PROMPT_CHARS),
            )
            assert isinstance(result, LLMGenerateResponse)
            assert any(
                "ABNORMAL PROMPT SIZE DETECTED" in rec.message for rec in caplog.records
            )
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_generate_streaming_exception_logs_and_releases_slot(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Streaming errors log an exception, re-raise, and still release the semaphore slot."""
    caplog.set_level(logging.ERROR)
    config = _make_config(routing_enabled=False)
    driver = _make_driver()

    async def failing_stream(_payload: dict[str, Any]) -> AsyncIterator[dict[str, Any]]:
        yield {"response": "partial", "model": "test-model", "done": False}
        msg = "stream boom"
        raise RuntimeError(msg)

    mock_stream_driver = AsyncMock()
    mock_stream_driver.initialize = AsyncMock()
    mock_stream_driver.cleanup = AsyncMock()
    mock_stream_driver.generate_stream = failing_stream

    with (
        patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver),
        patch(
            "news_creator.gateway.ollama_gateway.OllamaStreamDriver",
            return_value=mock_stream_driver,
        ),
    ):
        gateway = OllamaGateway(config)
        await gateway.initialize()
        initial_rt = gateway._semaphore._rt_available
        try:
            result = await gateway.generate(_wrap_prompt("hello"), stream=True)
            assert not isinstance(result, LLMGenerateResponse)
            with pytest.raises(RuntimeError, match="stream boom"):
                async for _ in result:
                    pass
            assert gateway._semaphore._rt_available == initial_rt
            assert any(
                "Stream generator error" in rec.message for rec in caplog.records
            )
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_generate_missing_response_field_raises_runtime_error() -> None:
    """Non-streaming responses without a 'response' key raise RuntimeError."""
    config = _make_config(routing_enabled=False)
    driver = _make_driver({"model": "test-model", "done": True})

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        await gateway.initialize()
        initial_rt = gateway._semaphore._rt_available
        try:
            with pytest.raises(RuntimeError, match="Invalid Ollama response format"):
                await gateway.generate(_wrap_prompt("hello"))
            assert gateway._semaphore._rt_available == initial_rt
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_generate_model_mismatch_logs_warning_and_returns_actual_model(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """When Ollama responds with a different model, warn and return actual_model."""
    caplog.set_level(logging.WARNING)
    config = _make_config(routing_enabled=False)
    driver = _make_driver(
        {
            "response": "ok",
            "model": "different-actual-model",
            "done": True,
        }
    )

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        await gateway.initialize()
        try:
            result = await gateway.generate(_wrap_prompt("hello"))
            assert isinstance(result, LLMGenerateResponse)
            assert result.model == "different-actual-model"
            assert any(
                "Model mismatch detected" in rec.message for rec in caplog.records
            )
        finally:
            await gateway.cleanup()
