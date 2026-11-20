"""Branch coverage tests for RemoteChatDriver.chat malformed payloads and session checks."""

from __future__ import annotations

import json
from http import HTTPStatus
from unittest.mock import AsyncMock, MagicMock

import pytest

from news_creator.config.distributed_be_config import RemoteTarget
from news_creator.gateway.remote_chat_driver import RemoteChatDriver


class _StubPostContext:
    def __init__(self, response: MagicMock) -> None:
        self._response = response

    async def __aenter__(self) -> MagicMock:
        return self._response

    async def __aexit__(self, _exc_type: object, _exc: object, _tb: object) -> bool:
        return False


def _make_target() -> RemoteTarget:
    return RemoteTarget(
        base_url="http://router.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=8192,
    )


def _make_response(body: str) -> MagicMock:
    raw = body.encode("utf-8")
    resp = MagicMock()
    resp.status = HTTPStatus.OK
    resp.headers = {"Content-Length": str(len(raw))}
    resp.content_length = len(raw)
    resp.content = MagicMock()
    resp.content.read = AsyncMock(side_effect=[raw, b""])
    return resp


@pytest.mark.asyncio
async def test_chat_raises_when_session_not_initialized() -> None:
    driver = RemoteChatDriver(timeout_seconds=30)
    with pytest.raises(RuntimeError, match="session not initialized"):
        await driver.chat(
            _make_target(),
            [{"role": "user", "content": "hello"}],
            max_tokens=100,
            temperature=0.0,
            top_p=1.0,
        )


@pytest.mark.parametrize("raw_json", ["[]", '{"choices": ["not-a-dict"]}'])
@pytest.mark.asyncio
async def test_chat_rejects_non_dict_payload_or_choice(raw_json: str) -> None:
    driver = RemoteChatDriver(timeout_seconds=30)
    session = MagicMock()
    session.closed = False
    session.post = MagicMock(return_value=_StubPostContext(_make_response(raw_json)))
    driver._session = session

    with pytest.raises(RuntimeError, match="Invalid"):
        await driver.chat(
            _make_target(),
            [{"role": "user", "content": "hello"}],
            max_tokens=100,
            temperature=0.0,
            top_p=1.0,
        )
    assert json.loads(raw_json) is not None
