"""Branch coverage tests for OllamaStreamDriver.generate_stream."""

from __future__ import annotations

import json
from http import HTTPStatus
from unittest.mock import AsyncMock, MagicMock

import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.driver.ollama_stream_driver import OllamaStreamDriver

EXPECTED_CHUNK_COUNT = 4


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
    cfg.llm = MagicMock()
    cfg.llm.inference_service_token = ""
    return cfg


@pytest.mark.asyncio
async def test_generate_stream_yields_past_initial_log_threshold() -> None:
    driver = OllamaStreamDriver(_make_config())
    lines = [
        (json.dumps({"response": f"tok{i}", "done": False}) + "\n").encode("utf-8")
        for i in range(1, EXPECTED_CHUNK_COUNT)
    ]
    lines.append((json.dumps({"response": "", "done": True}) + "\n").encode("utf-8"))
    lines.append(b"")

    resp = MagicMock()
    resp.status = HTTPStatus.OK
    resp.content = MagicMock()
    resp.content.readline = AsyncMock(side_effect=lines)

    session = MagicMock()
    session.closed = False
    session.post = MagicMock(return_value=_StubPostCtx(resp))
    driver.session = session

    chunks = [
        chunk
        async for chunk in driver.generate_stream(
            {"prompt": "hello", "stream": True, "model": "gemma4"}
        )
    ]
    assert len(chunks) == EXPECTED_CHUNK_COUNT
    assert chunks[-1]["done"] is True
