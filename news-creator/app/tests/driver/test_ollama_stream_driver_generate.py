"""Unit tests for OllamaStreamDriver.generate_stream() and option merging."""

from unittest.mock import AsyncMock, MagicMock, patch

import aiohttp
import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.driver.ollama_stream_driver import (
    _RUNNER_STARTUP_PARAMS,
    OllamaStreamDriver,
)

pytestmark = pytest.mark.usefixtures("dummy_redis_password_file")


@pytest.fixture
def config() -> NewsCreatorConfig:
    """Create a NewsCreatorConfig with known test values."""
    with patch.dict(
        "os.environ",
        {
            "LLM_SERVICE_URL": "http://127.0.0.1:11435",
            "LLM_MODEL": "gemma4-e4b-12k",
        },
    ):
        return NewsCreatorConfig()


@pytest.fixture
def driver(config: NewsCreatorConfig) -> OllamaStreamDriver:
    return OllamaStreamDriver(config)


class _ReadlineContent:
    """Mock response.content supporting await readline() for generate_stream()."""

    def __init__(self, items: list[bytes | Exception]) -> None:
        self._items = list(items)
        self._index = 0

    async def readline(self) -> bytes:
        if self._index >= len(self._items):
            return b""
        item = self._items[self._index]
        self._index += 1
        if isinstance(item, Exception):
            raise item
        return item


def _make_generate_session(
    readline_items: list[bytes | Exception] | None = None,
    status: int = 200,
    text_body: str = "",
) -> tuple[MagicMock, dict]:
    """Create a mock aiohttp.ClientSession for generate_stream()."""
    captured: dict = {}

    mock_resp = AsyncMock()
    mock_resp.status = status
    mock_resp.text = AsyncMock(return_value=text_body)
    mock_resp.content = _ReadlineContent(readline_items or [])

    ctx = AsyncMock()
    ctx.__aenter__ = AsyncMock(return_value=mock_resp)
    ctx.__aexit__ = AsyncMock(return_value=False)

    session = MagicMock()
    session.closed = False

    def _post(url: str, json: dict | None = None, **kwargs):
        captured["url"] = url
        captured["json"] = json
        captured["kwargs"] = kwargs
        return ctx

    session.post = _post
    return session, captured


class TestGenerateStreamHappyPath:
    """Happy path tests for OllamaStreamDriver.generate_stream()."""

    @pytest.mark.asyncio
    async def test_yields_ndjson_chunks_and_done_final_chunk(self, driver):
        """generate_stream() yields parsed NDJSON dicts up to done=True."""
        lines: list[bytes | Exception] = [
            b'{"model":"gemma4-e4b-12k","response":"Hello","done":false}\n',
            b"\n",  # empty line skipped
            b'{"model":"gemma4-e4b-12k","response":" world","done":false}\n',
            b'{"model":"gemma4-e4b-12k","response":"","done":true,"eval_count":2}\n',
        ]
        session, captured = _make_generate_session(readline_items=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "prompt": "Summarize this article.",
            "stream": True,
        }

        chunks = [chunk async for chunk in driver.generate_stream(payload)]

        assert len(chunks) == 3
        assert chunks[0]["response"] == "Hello"
        assert chunks[0]["done"] is False
        assert chunks[1]["response"] == " world"
        assert chunks[1]["done"] is False
        assert chunks[2]["done"] is True
        assert chunks[2]["eval_count"] == 2
        assert captured["url"] == "http://127.0.0.1:11435/api/generate"
        assert captured["kwargs"].get("allow_redirects") is False

    @pytest.mark.asyncio
    async def test_skips_malformed_json_line_and_continues_to_done(self, driver):
        """Malformed NDJSON lines are logged and skipped if stream still completes with done=true."""
        lines: list[bytes | Exception] = [
            b'{"model":"gemma4-e4b-12k","response":"First","done":false}\n',
            b"{malformed-json\n",
            b'{"model":"gemma4-e4b-12k","response":"","done":true}\n',
        ]
        session, _ = _make_generate_session(readline_items=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "prompt": "Test prompt",
            "stream": True,
        }

        chunks = [chunk async for chunk in driver.generate_stream(payload)]
        assert len(chunks) == 2
        assert chunks[0]["response"] == "First"
        assert chunks[1]["done"] is True


class TestThinkFalseAndMergeOptions:
    """Tests for think=False enforcement and _RUNNER_STARTUP_PARAMS stripping."""

    def test_merge_options_strips_all_runner_startup_params(self, driver):
        """_merge_options() strips num_ctx, num_batch, num_keep, and num_gpu."""
        assert _RUNNER_STARTUP_PARAMS == frozenset(
            {"num_ctx", "num_batch", "num_keep", "num_gpu"}
        )
        base = driver.config.get_llm_options()
        caller_options = {
            "num_ctx": 99999,
            "num_batch": 9999,
            "num_keep": 999,
            "num_gpu": 0,
            "temperature": 0.42,
            "num_predict": 777,
        }

        merged = driver._merge_options(caller_options)

        assert merged["num_ctx"] == base["num_ctx"]
        assert merged["num_batch"] == base["num_batch"]
        assert merged["num_keep"] == base["num_keep"]
        assert merged["num_gpu"] == base["num_gpu"]
        assert merged["temperature"] == 0.42
        assert merged["num_predict"] == 777

    @pytest.mark.asyncio
    async def test_chat_stream_forces_think_false_and_merges_options(self, driver):
        """chat_stream() forces think=False and strips runner startup params."""
        mock_resp = AsyncMock()
        mock_resp.status = 200

        async def _iter_lines():
            yield b'{"message":{"role":"assistant","content":"ok"},"done":true}\n'

        mock_resp.content = _iter_lines()
        ctx = AsyncMock()
        ctx.__aenter__ = AsyncMock(return_value=mock_resp)
        ctx.__aexit__ = AsyncMock(return_value=False)

        captured: dict = {}
        session = MagicMock()
        session.closed = False

        def _post(url: str, json: dict | None = None, **kwargs):
            captured["url"] = url
            captured["json"] = json
            return ctx

        session.post = _post
        driver.session = session

        base = driver.config.get_llm_options()
        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "hi"}],
            "options": {"num_ctx": 65536, "num_gpu": 0, "temperature": 0.15},
        }

        chunks = [c async for c in driver.chat_stream(payload)]
        assert len(chunks) == 1
        assert captured["json"]["think"] is False
        assert captured["json"]["options"]["num_ctx"] == base["num_ctx"]
        assert captured["json"]["options"]["num_gpu"] == base["num_gpu"]
        assert captured["json"]["options"]["temperature"] == 0.15


class TestTruncatedStreamInvariant:
    """Truncated stream invariant: stream ending after data without done=true must raise RuntimeError."""

    @pytest.mark.asyncio
    async def test_raises_runtime_error_when_eof_reached_without_done_true(
        self, driver
    ):
        """Stream yielding data chunks then hitting EOF without done=true raises RuntimeError."""
        lines: list[bytes | Exception] = [
            b'{"model":"gemma4-e4b-12k","response":"partial 1","done":false}\n',
            b'{"model":"gemma4-e4b-12k","response":"partial 2","done":false}\n',
        ]
        session, _ = _make_generate_session(readline_items=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "prompt": "Summarize this article.",
            "stream": True,
        }

        yielded = []
        with pytest.raises(RuntimeError, match="response is truncated"):
            async for chunk in driver.generate_stream(payload):
                yielded.append(chunk)

        assert len(yielded) == 2
        assert yielded[0]["response"] == "partial 1"
        assert yielded[1]["response"] == "partial 2"

    @pytest.mark.asyncio
    async def test_raises_runtime_error_when_connection_drops_mid_stream_without_done(
        self, driver
    ):
        """Mid-stream ClientConnectionError after yielding partial chunks without done=true raises RuntimeError."""
        lines: list[bytes | Exception] = [
            b'{"model":"gemma4-e4b-12k","response":"partial","done":false}\n',
            aiohttp.ClientConnectionError("peer reset connection"),
        ]
        session, _ = _make_generate_session(readline_items=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "prompt": "Summarize this article.",
            "stream": True,
        }

        yielded = []
        with pytest.raises(RuntimeError, match="response is truncated"):
            async for chunk in driver.generate_stream(payload):
                yielded.append(chunk)

        assert len(yielded) == 1


class TestGenerateStreamErrorHandling:
    """Tests for error conditions in OllamaStreamDriver.generate_stream()."""

    @pytest.mark.asyncio
    async def test_raises_on_empty_prompt(self, driver):
        """Empty or missing prompt raises ValueError."""
        with pytest.raises(ValueError, match="payload must contain 'prompt'"):
            async for _ in driver.generate_stream({"prompt": "", "stream": True}):
                pass

    @pytest.mark.asyncio
    async def test_raises_when_stream_flag_not_true(self, driver):
        """stream=False raises ValueError."""
        with pytest.raises(ValueError, match="requires stream=True"):
            async for _ in driver.generate_stream({"prompt": "hello", "stream": False}):
                pass

    @pytest.mark.asyncio
    async def test_raises_runtime_error_on_http_non_200(self, driver):
        """HTTP status != 200 raises RuntimeError with status code."""
        session, _ = _make_generate_session(
            status=500, text_body="internal ollama error"
        )
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "prompt": "Summarize this article.",
            "stream": True,
        }

        with pytest.raises(
            RuntimeError, match="Ollama API error: HTTP 500 - internal ollama error"
        ):
            async for _ in driver.generate_stream(payload):
                pass

    @pytest.mark.asyncio
    async def test_raises_when_session_remains_uninitialized(self, driver):
        """If initialize() leaves session as None, generate_stream raises AssertionError."""
        driver.session = None
        driver.initialize = AsyncMock()  # no-op so driver.session stays None

        payload = {
            "model": "gemma4-e4b-12k",
            "prompt": "Summarize this article.",
            "stream": True,
        }

        with pytest.raises(AssertionError, match="Session not initialized"):
            async for _ in driver.generate_stream(payload):
                pass

    @pytest.mark.asyncio
    async def test_propagates_client_connection_error_when_no_data_received(
        self, driver
    ):
        """aiohttp.ClientConnectionError before any data chunk is propagated directly."""
        lines: list[bytes | Exception] = [
            aiohttp.ClientConnectionError("connection refused before data"),
        ]
        session, _ = _make_generate_session(readline_items=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "prompt": "Summarize this article.",
            "stream": True,
        }

        with pytest.raises(
            aiohttp.ClientConnectionError, match="connection refused before data"
        ):
            async for _ in driver.generate_stream(payload):
                pass

    @pytest.mark.asyncio
    async def test_initialize_and_cleanup_lifecycle(self, config):
        """initialize() creates an aiohttp.ClientSession and cleanup() closes it."""
        drv = OllamaStreamDriver(config)
        await drv.initialize()
        assert drv.session is not None
        assert not drv.session.closed
        await drv.cleanup()
        assert drv.session.closed
