"""Tests for RemoteChatDriver (OpenAI-compatible /v1/chat/completions client)."""

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock

import aiohttp
import pytest

from news_creator.domain.models import LLMGenerateResponse


def _make_target():
    from news_creator.config.distributed_be_config import RemoteTarget

    return RemoteTarget(
        base_url="http://router.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )


def _make_driver(reasoning_effort: str | None = None):
    from news_creator.gateway.remote_chat_driver import RemoteChatDriver

    return RemoteChatDriver(timeout_seconds=120, reasoning_effort=reasoning_effort)


class _StubPostContext:
    def __init__(self, response):
        self._response = response

    async def __aenter__(self):
        reset_stream = getattr(self._response, "_reset_stream", None)
        if callable(reset_stream):
            reset_stream()
        return self._response

    async def __aexit__(self, exc_type, exc, tb):
        return False


def _make_mock_session(response=None, side_effect=None):
    session = MagicMock()
    session.closed = False
    session.headers = {}
    if side_effect is not None:
        session.post = MagicMock(side_effect=side_effect)
    else:
        session.post = MagicMock(return_value=_StubPostContext(response))
    return session


def _make_mock_response(
    status: int = 200,
    body: str | bytes = "",
    *,
    headers: dict[str, str] | None = None,
):
    raw_bytes = body.encode("utf-8") if isinstance(body, str) else bytes(body)
    resp = MagicMock()
    resp.status = status
    effective_headers = (
        dict(headers)
        if headers is not None
        else {"Content-Length": str(len(raw_bytes))}
    )
    resp.headers = effective_headers
    cl = effective_headers.get("Content-Length")
    resp.content_length = int(cl) if cl is not None else None

    async def _text(encoding: str | None = "utf-8", errors: str = "strict") -> str:
        return raw_bytes.decode(encoding or "utf-8", errors=errors)

    offset = 0

    def _reset_stream() -> None:
        nonlocal offset
        offset = 0

    async def _content_read(n: int = -1) -> bytes:
        nonlocal offset
        if n is None or n < 0:
            chunk = raw_bytes[offset:]
            offset = len(raw_bytes)
            return chunk
        chunk = raw_bytes[offset : offset + n]
        offset += len(chunk)
        return chunk

    async def _iter_chunked(n: int):
        for i in range(0, len(raw_bytes), n):
            yield raw_bytes[i : i + n]

    resp._reset_stream = _reset_stream
    resp.text = AsyncMock(side_effect=_text)
    resp.read = AsyncMock(return_value=raw_bytes)
    resp.content = MagicMock()
    resp.content.read = AsyncMock(side_effect=_content_read)
    resp.content.iter_chunked = _iter_chunked
    return resp


@pytest.mark.asyncio
async def test_initialize_and_cleanup_omit_authorization_header():
    """RemoteChatDriver never attaches an Authorization header to its session."""
    drv = _make_driver(reasoning_effort=None)
    await drv.initialize()
    try:
        assert drv._session is not None
        assert "Authorization" not in drv._session.headers
    finally:
        await drv.cleanup()


@pytest.mark.asyncio
async def test_chat_posts_exact_payload_without_reasoning_effort_when_none():
    """When reasoning_effort is None, 'reasoning_effort' is omitted from the request JSON."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    response_body = json.dumps(
        {
            "id": "chatcmpl-123",
            "object": "chat.completion",
            "model": "model-a:latest",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": "  要約テキストです。  ",
                    },
                    "finish_reason": "stop",
                }
            ],
            "usage": {
                "prompt_tokens": 210,
                "completion_tokens": 75,
                "total_tokens": 285,
            },
        }
    )
    mock_session = _make_mock_session(_make_mock_response(200, response_body))
    driver._session = mock_session

    messages = [{"role": "user", "content": "Summarize this article."}]
    result = await driver.chat(
        target,
        messages,
        max_tokens=1000,
        temperature=0.5,
        top_p=0.85,
    )

    mock_session.post.assert_called_once()
    call_args, call_kwargs = mock_session.post.call_args
    assert call_args[0] == "http://router.example:8080/v1/chat/completions"
    assert call_kwargs["allow_redirects"] is False
    assert "headers" not in call_kwargs or "Authorization" not in (
        call_kwargs.get("headers") or {}
    )
    assert call_kwargs["json"] == {
        "model": "provider-a/model-a:latest",
        "messages": messages,
        "stream": False,
        "max_tokens": 1000,
        "temperature": 0.5,
        "top_p": 0.85,
    }

    assert isinstance(result, LLMGenerateResponse)
    assert result.response == "要約テキストです。"
    assert result.model == "model-a:latest"
    assert result.done is True
    assert result.done_reason == "stop"
    assert result.prompt_eval_count == 210
    assert result.eval_count == 75


@pytest.mark.asyncio
async def test_chat_posts_exact_payload_with_reasoning_effort_when_configured():
    """When reasoning_effort is set, it is included verbatim as top-level 'reasoning_effort'."""
    driver = _make_driver(reasoning_effort="none")
    target = _make_target()
    response_body = json.dumps(
        {
            "id": "chatcmpl-456",
            "object": "chat.completion",
            "model": "model-a:latest",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": "要約テキストです。",
                    },
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 100, "completion_tokens": 50},
        }
    )
    mock_session = _make_mock_session(_make_mock_response(200, response_body))
    driver._session = mock_session

    messages = [{"role": "user", "content": "Summarize this article."}]
    await driver.chat(
        target,
        messages,
        max_tokens=1000,
        temperature=0.5,
        top_p=0.85,
    )

    _, call_kwargs = mock_session.post.call_args
    assert call_kwargs["json"] == {
        "model": "provider-a/model-a:latest",
        "messages": messages,
        "stream": False,
        "max_tokens": 1000,
        "temperature": 0.5,
        "top_p": 0.85,
        "reasoning_effort": "none",
    }


@pytest.mark.asyncio
async def test_chat_uses_content_as_returned_and_ignores_reasoning_fields():
    """Surrounding whitespace is stripped from content; reasoning/reasoning_content fields are ignored."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    response_body = json.dumps(
        {
            "model": "model-a:latest",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": "  \n最終的な記事要約である。\n ",
                        "reasoning": "should be ignored",
                        "reasoning_content": "should also be ignored",
                    },
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 100, "completion_tokens": 40},
        }
    )
    driver._session = _make_mock_session(_make_mock_response(200, response_body))

    result = await driver.chat(
        target,
        [{"role": "user", "content": "Article"}],
        max_tokens=500,
        temperature=0.3,
        top_p=0.85,
    )

    assert result.response == "最終的な記事要約である。"
    assert "ignored" not in result.response


@pytest.mark.asyncio
async def test_chat_falls_back_to_target_model_when_response_model_missing():
    """When response omits 'model' or sets it empty, result.model defaults to target.model."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    response_body = json.dumps(
        {
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "Summary output."},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 50, "completion_tokens": 20},
        }
    )
    driver._session = _make_mock_session(_make_mock_response(200, response_body))

    result = await driver.chat(
        target,
        [{"role": "user", "content": "Article"}],
        max_tokens=500,
        temperature=0.3,
        top_p=0.85,
    )

    assert result.model == "provider-a/model-a:latest"


@pytest.mark.asyncio
async def test_chat_non_200_raises_runtime_error_with_status_and_body_slice():
    """Non-200 HTTP response raises RuntimeError including status and first 200 chars of body."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    long_body = "Upstream failure detail: " + ("X" * 250)
    driver._session = _make_mock_session(_make_mock_response(502, long_body))

    with pytest.raises(RuntimeError) as exc_info:
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )

    err_text = str(exc_info.value)
    assert "502" in err_text
    assert long_body[:200] in err_text
    assert long_body[:220] not in err_text


@pytest.mark.asyncio
async def test_chat_non_json_body_raises_runtime_error():
    """200 response with non-JSON body raises RuntimeError."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    driver._session = _make_mock_session(
        _make_mock_response(200, "<html>Bad Gateway</html>")
    )

    with pytest.raises(RuntimeError):
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )


@pytest.mark.parametrize(
    "bad_payload",
    [
        {},
        {"choices": []},
        {"choices": [{}]},
        {"choices": [{"message": None}]},
        {"choices": [{"message": {}}]},
        {"choices": [{"message": {"role": "assistant", "content": ""}}]},
        {"choices": [{"message": {"role": "assistant", "content": "   \n  "}}]},
    ],
)
@pytest.mark.asyncio
async def test_chat_missing_choices_or_empty_content_raises_runtime_error(
    bad_payload: dict,
):
    """Missing choices/message or empty/whitespace content raises RuntimeError."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    driver._session = _make_mock_session(
        _make_mock_response(200, json.dumps(bad_payload))
    )

    with pytest.raises(RuntimeError):
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )


@pytest.mark.parametrize(
    "exc",
    [
        aiohttp.ClientConnectorError(
            connection_key=MagicMock(), os_error=OSError("Connection refused")
        ),
        aiohttp.ServerTimeoutError("Request timed out"),
        asyncio.TimeoutError(),
    ],
)
@pytest.mark.asyncio
async def test_chat_client_error_and_timeout_raise_runtime_error(exc: BaseException):
    """aiohttp ClientError and timeout exceptions are wrapped in RuntimeError."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    driver._session = _make_mock_session(side_effect=exc)

    with pytest.raises(RuntimeError):
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )


@pytest.mark.parametrize("status", [400, 404, 422])
@pytest.mark.asyncio
async def test_chat_4xx_rejection_raises_remote_request_rejected_error(status: int):
    """HTTP 4xx other than 408/429 raises RemoteRequestRejectedError(RuntimeError)."""
    from news_creator.gateway.remote_chat_driver import RemoteRequestRejectedError

    assert issubclass(RemoteRequestRejectedError, RuntimeError)
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    driver._session = _make_mock_session(
        _make_mock_response(status, '{"error": "bad request"}')
    )

    with pytest.raises(RemoteRequestRejectedError):
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )


@pytest.mark.parametrize("empty_content", ["", "   \n\t  "])
@pytest.mark.asyncio
async def test_chat_empty_content_raises_remote_request_rejected_error(
    empty_content: str,
):
    """Empty or whitespace-only assistant content raises RemoteRequestRejectedError."""
    from news_creator.gateway.remote_chat_driver import RemoteRequestRejectedError

    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    body = json.dumps(
        {
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": empty_content},
                    "finish_reason": "length",
                }
            ]
        }
    )
    driver._session = _make_mock_session(_make_mock_response(200, body))

    with pytest.raises(RemoteRequestRejectedError):
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )


@pytest.mark.parametrize("status", [408, 429, 500])
@pytest.mark.asyncio
async def test_chat_408_429_500_raise_plain_runtime_error_not_rejected_subclass(
    status: int,
):
    """HTTP 408, 429, and 500 raise plain RuntimeError, not RemoteRequestRejectedError."""
    from news_creator.gateway.remote_chat_driver import RemoteRequestRejectedError

    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    driver._session = _make_mock_session(
        _make_mock_response(status, '{"error": "transient or server failure"}')
    )

    with pytest.raises(RuntimeError) as exc_info:
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )

    assert not isinstance(exc_info.value, RemoteRequestRejectedError)


@pytest.mark.asyncio
async def test_chat_non_utf8_200_body_never_leaks_unicode_decode_error():
    """A 200 body with invalid UTF-8 bytes is decoded with replacement or converted to RuntimeError."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    driver._session = _make_mock_session(
        _make_mock_response(200, b"\xff\xfe\xfdnot-json")
    )

    with pytest.raises(RuntimeError) as exc_info:
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )

    assert not isinstance(exc_info.value, UnicodeDecodeError)


@pytest.mark.parametrize("with_content_length", [True, False])
@pytest.mark.asyncio
async def test_chat_response_body_over_4_mib_raises_runtime_error(
    with_content_length: bool,
):
    """A 200 chat response body larger than 4 MiB raises RuntimeError."""
    driver = _make_driver(reasoning_effort=None)
    target = _make_target()
    four_mib = 4 * 1024 * 1024
    oversized_content = "x" * (four_mib + 64)
    body = json.dumps(
        {
            "id": "chatcmpl-oversized",
            "model": target.model,
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": oversized_content},
                    "finish_reason": "stop",
                }
            ],
        }
    ).encode("utf-8")
    assert len(body) > four_mib
    headers = {"Content-Length": str(len(body))} if with_content_length else {}
    driver._session = _make_mock_session(
        _make_mock_response(200, body, headers=headers)
    )

    with pytest.raises(RuntimeError):
        await driver.chat(
            target,
            [{"role": "user", "content": "Article"}],
            max_tokens=500,
            temperature=0.3,
            top_p=0.85,
        )
