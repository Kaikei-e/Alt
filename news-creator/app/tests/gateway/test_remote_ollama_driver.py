"""Tests for RemoteOllamaDriver."""

import json
import pytest
from unittest.mock import AsyncMock, MagicMock
import aiohttp

from news_creator.gateway.remote_ollama_driver import RemoteOllamaDriver
from news_creator.domain.models import LLMGenerateResponse


@pytest.fixture
def driver():
    return RemoteOllamaDriver(timeout_seconds=300)


def _make_mock_session():
    """Create a mock aiohttp session with closed=False."""
    session = MagicMock(spec=aiohttp.ClientSession)
    session.closed = False
    return session


def _make_mock_response(status=200, body=""):
    """Create a mock aiohttp response."""
    resp = MagicMock()
    resp.status = status
    resp.text = AsyncMock(return_value=body)
    resp.__aenter__ = AsyncMock(return_value=resp)
    resp.__aexit__ = AsyncMock(return_value=False)
    return resp


@pytest.mark.asyncio
async def test_generate_posts_to_given_url(driver):
    """RemoteOllamaDriver sends POST to {base_url}/api/generate."""
    body = json.dumps(
        {
            "response": "summary text",
            "model": "gemma4-e4b-q4km",
            "done": True,
            "done_reason": "stop",
            "prompt_eval_count": 100,
            "eval_count": 50,
            "total_duration": 1_000_000_000,
        }
    )
    mock_session = _make_mock_session()
    mock_session.post = MagicMock(return_value=_make_mock_response(200, body))
    driver._session = mock_session

    result = await driver.generate(
        base_url="http://remote-a:11434",
        payload={"model": "gemma4-e4b-q4km", "prompt": "hello", "stream": False},
    )

    mock_session.post.assert_called_once()
    call_args = mock_session.post.call_args
    assert call_args[0][0] == "http://remote-a:11434/api/generate"
    assert isinstance(result, LLMGenerateResponse)
    assert result.response == "summary text"
    assert result.model == "gemma4-e4b-q4km"


@pytest.mark.asyncio
async def test_generate_handles_timeout(driver):
    """Timeout during remote generation raises RuntimeError."""
    mock_session = _make_mock_session()
    mock_session.post = MagicMock(side_effect=aiohttp.ServerTimeoutError("timed out"))
    driver._session = mock_session

    with pytest.raises(RuntimeError, match="Remote Ollama.*timed out"):
        await driver.generate(
            base_url="http://remote-a:11434",
            payload={"model": "gemma4-e4b-q4km", "prompt": "hello", "stream": False},
        )


@pytest.mark.asyncio
async def test_generate_handles_asyncio_timeout(driver):
    """aiohttp total timeout surfaced as asyncio.TimeoutError should raise RuntimeError."""
    import asyncio

    mock_session = _make_mock_session()
    mock_session.post = MagicMock(side_effect=asyncio.TimeoutError())
    driver._session = mock_session

    with pytest.raises(RuntimeError, match="timed out"):
        await driver.generate(
            base_url="http://remote-a:11434",
            payload={"model": "gemma4-e4b-q4km", "prompt": "hello", "stream": False},
        )


@pytest.mark.asyncio
async def test_generate_handles_connection_error(driver):
    """Connection refused raises RuntimeError."""
    mock_session = _make_mock_session()
    mock_session.post = MagicMock(
        side_effect=aiohttp.ClientConnectorError(
            connection_key=MagicMock(), os_error=OSError("Connection refused")
        )
    )
    driver._session = mock_session

    with pytest.raises(RuntimeError, match="Remote Ollama.*failed"):
        await driver.generate(
            base_url="http://remote-a:11434",
            payload={"model": "gemma4-e4b-q4km", "prompt": "hello", "stream": False},
        )


@pytest.mark.asyncio
async def test_generate_parses_response_to_domain_model(driver):
    """Response JSON is parsed into LLMGenerateResponse with all metrics."""
    response_data = {
        "response": "Generated text here",
        "model": "gemma4-e4b-q4km",
        "done": True,
        "done_reason": "stop",
        "prompt_eval_count": 200,
        "eval_count": 80,
        "total_duration": 2_000_000_000,
        "load_duration": 100_000_000,
        "prompt_eval_duration": 500_000_000,
        "eval_duration": 1_400_000_000,
    }
    mock_session = _make_mock_session()
    mock_session.post = MagicMock(
        return_value=_make_mock_response(200, json.dumps(response_data))
    )
    driver._session = mock_session

    result = await driver.generate(
        base_url="http://host:11434",
        payload={"model": "gemma4-e4b-q4km", "prompt": "test", "stream": False},
    )

    assert result.response == "Generated text here"
    assert result.prompt_eval_count == 200
    assert result.eval_count == 80
    assert result.total_duration == 2_000_000_000
    assert result.load_duration == 100_000_000
    assert result.eval_duration == 1_400_000_000


@pytest.mark.asyncio
async def test_generate_handles_http_error_status(driver):
    """Non-200 HTTP status raises RuntimeError."""
    mock_session = _make_mock_session()
    mock_session.post = MagicMock(
        return_value=_make_mock_response(500, "Internal server error")
    )
    driver._session = mock_session

    with pytest.raises(RuntimeError, match="HTTP 500"):
        await driver.generate(
            base_url="http://host:11434",
            payload={"model": "test", "prompt": "hello", "stream": False},
        )


@pytest.mark.asyncio
async def test_generate_handles_malformed_json(driver):
    """Malformed JSON response raises RuntimeError."""
    mock_session = _make_mock_session()
    mock_session.post = MagicMock(return_value=_make_mock_response(200, "not json"))
    driver._session = mock_session

    with pytest.raises(RuntimeError, match="decode"):
        await driver.generate(
            base_url="http://host:11434",
            payload={"model": "test", "prompt": "hello", "stream": False},
        )


@pytest.mark.asyncio
async def test_initialize_and_cleanup(driver):
    """Session lifecycle: initialize creates, cleanup closes."""
    await driver.initialize()
    assert driver._session is not None

    await driver.cleanup()
