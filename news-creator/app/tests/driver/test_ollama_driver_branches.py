"""Branch coverage tests for OllamaDriver.generate edge cases."""

from __future__ import annotations

from http import HTTPStatus
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.driver.ollama_driver import OllamaDriver

ABNORMAL_PROMPT_CHARS = 100_050


class _StubPostCtx:
    def __init__(self, response: MagicMock) -> None:
        self._response = response

    async def __aenter__(self) -> MagicMock:
        return self._response

    async def __aexit__(self, _exc_type: object, _exc: object, _tb: object) -> bool:
        return False


def _make_config() -> NewsCreatorConfig:
    cfg = MagicMock(spec=NewsCreatorConfig)
    cfg.llm_service_url = "http://localhost:11434"
    cfg.llm_timeout_seconds = 30
    cfg.llm = MagicMock()
    cfg.llm.inference_service_token = ""
    return cfg


def _make_resp(status: int, body: str) -> MagicMock:
    resp = MagicMock()
    resp.status = status
    resp.text = AsyncMock(return_value=body)
    return resp


@pytest.mark.asyncio
async def test_generate_validates_prompt_and_stream_flag() -> None:
    driver = OllamaDriver(_make_config())
    driver.session = MagicMock()
    driver.session.closed = False

    with pytest.raises(ValueError, match="must contain 'prompt'"):
        await driver.generate({})

    with pytest.raises(ValueError, match="does not support streaming"):
        await driver.generate({"prompt": "hi", "stream": True})


@pytest.mark.asyncio
async def test_generate_retries_500_then_succeeds_with_abnormal_prompt() -> None:
    driver = OllamaDriver(_make_config())
    session = MagicMock()
    session.closed = False
    session.post = MagicMock(
        side_effect=[
            _StubPostCtx(_make_resp(HTTPStatus.INTERNAL_SERVER_ERROR, "oops")),
            _StubPostCtx(_make_resp(HTTPStatus.OK, '{"response": "ok", "done": true}')),
        ]
    )
    driver.session = session

    with patch("asyncio.sleep", new=AsyncMock()):
        result = await driver.generate(
            {"prompt": "x" * ABNORMAL_PROMPT_CHARS, "model": "gemma4"}
        )

    assert result["response"] == "ok"


@pytest.mark.asyncio
async def test_generate_non_retryable_400_and_invalid_json_raise_runtime_error() -> (
    None
):
    driver = OllamaDriver(_make_config())
    session = MagicMock()
    session.closed = False
    session.post = MagicMock(
        return_value=_StubPostCtx(_make_resp(HTTPStatus.BAD_REQUEST, "bad req"))
    )
    driver.session = session

    with pytest.raises(RuntimeError, match="HTTP 400"):
        await driver.generate({"prompt": "hello"})

    session.post = MagicMock(
        return_value=_StubPostCtx(_make_resp(HTTPStatus.OK, "not-json"))
    )
    with pytest.raises(RuntimeError, match="Failed to decode"):
        await driver.generate({"prompt": "hello"})
