"""Tests for OllamaDriver non-streaming HTTP client timeout wiring.

HIGH finding (2026-07-06 review): initialize() built a ClientTimeout with
total=None, sock_read=None (connect timeout only), so config.llm_timeout_seconds
was logged but never actually applied. A hung Ollama backend would block the
calling coroutine (and its semaphore slot) forever.
"""

import asyncio
from unittest.mock import patch

import pytest
from aiohttp import web
from aiohttp.test_utils import TestServer

# `slow_handler` below awaits `asyncio.sleep(5)` using the *real* global
# asyncio.sleep -- so any mock of asyncio.sleep (even scoped to the
# ollama_driver module) would also fake out the test server's own delay,
# since both look up the same shared `asyncio` module object. Retries are
# therefore left un-mocked; the bound comes from `asyncio.wait_for` below.

from news_creator.config.config import NewsCreatorConfig
from news_creator.driver.ollama_driver import OllamaDriver


def _make_config(service_url: str, timeout_seconds: int) -> NewsCreatorConfig:
    with patch.dict(
        "os.environ",
        {
            "LLM_SERVICE_URL": service_url,
            "LLM_TIMEOUT_SECONDS": str(timeout_seconds),
        },
    ):
        return NewsCreatorConfig()


@pytest.mark.asyncio
async def test_initialize_applies_llm_timeout_seconds_to_read_timeout():
    """The session's ClientTimeout must actually carry llm_timeout_seconds.

    Previously both `total` and `sock_read` were hardcoded to None, so a hung
    Ollama response would never time out regardless of this config value.
    """
    config = _make_config("http://localhost:11435", timeout_seconds=45)
    driver = OllamaDriver(config)

    await driver.initialize()
    try:
        timeout = driver.session.timeout
        assert timeout.sock_read == 45 or timeout.total == 45, (
            f"llm_timeout_seconds not applied to ClientTimeout: {timeout}"
        )
    finally:
        await driver.cleanup()


@pytest.mark.asyncio
async def test_generate_errors_instead_of_hanging_when_ollama_stalls():
    """A backend that stops responding must cause generate() to fail bounded
    in time, not hang forever holding the caller's semaphore slot.
    """

    async def slow_handler(request: web.Request) -> web.Response:
        await asyncio.sleep(5)
        return web.json_response({"response": "too late"})

    app = web.Application()
    app.router.add_post("/api/generate", slow_handler)
    server = TestServer(app)
    await server.start_server()

    try:
        config = _make_config(f"http://{server.host}:{server.port}", timeout_seconds=1)
        driver = OllamaDriver(config)

        # With the fix, per-attempt sock_read timeout (1s) x 4 attempts plus
        # exponential backoff (~1.1 + 2.2 + 4.4s) totals ~11.7s -- comfortably
        # under this 15s outer bound -- so generate() itself raises
        # RuntimeError (retries exhausted) well before wait_for's own
        # deadline. If sock_read regressed back to None, the per-attempt
        # reads would never time out and wait_for would instead raise
        # asyncio.TimeoutError at the 15s mark; asserting RuntimeError
        # specifically (not just "raises something") is what catches that
        # regression instead of masking it.
        with pytest.raises(RuntimeError, match="Ollama API failed after"):
            await asyncio.wait_for(
                driver.generate({"model": "test", "prompt": "hi", "stream": False}),
                timeout=15,
            )
    finally:
        await driver.cleanup()
        await server.close()


@pytest.mark.asyncio
async def test_generate_does_not_reserialize_full_payload_for_logging():
    """generate() must not re-serialize the whole payload via json.dumps()
    just to size a diagnostic log field.

    For a large hierarchical job, generate() is called dozens of times with
    prompts that can be 100K+ chars. json.dumps(payload) on every call is
    wasted synchronous CPU work on the event loop for a value that is only
    ever used in a log line -- the log should reuse already-computed lengths
    (e.g. prompt_length) instead of re-serializing the payload.
    """

    async def ok_handler(request: web.Request) -> web.Response:
        return web.json_response({"response": "ok", "model": "test", "done": True})

    app = web.Application()
    app.router.add_post("/api/generate", ok_handler)
    server = TestServer(app)
    await server.start_server()

    try:
        config = _make_config(f"http://{server.host}:{server.port}", timeout_seconds=5)
        driver = OllamaDriver(config)

        with patch("news_creator.driver.ollama_driver.json.dumps") as mock_dumps:
            result = await driver.generate(
                {"model": "test-model", "prompt": "hello world", "stream": False}
            )

        assert result["response"] == "ok"
        assert mock_dumps.call_count == 0, (
            "generate() should not call json.dumps(payload) to compute a "
            "log-only size estimate"
        )
    finally:
        await driver.cleanup()
        await server.close()
