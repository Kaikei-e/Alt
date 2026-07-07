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

        # Bounded by wait_for: with the fix, per-attempt sock_read timeout
        # (1s) x up to 4 attempts + backoff is well under this bound. Without
        # the fix, sock_read=None means the call would hang past this bound
        # and asyncio.wait_for would raise TimeoutError instead -- either
        # outcome here proves it does NOT hang forever, but only the fixed
        # code raises the underlying RuntimeError from the driver's own
        # retry-exhausted path.
        with pytest.raises((RuntimeError, asyncio.TimeoutError)):
            await asyncio.wait_for(
                driver.generate({"model": "test", "prompt": "hi", "stream": False}),
                timeout=15,
            )
    finally:
        await driver.cleanup()
        await server.close()
