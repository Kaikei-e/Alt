"""Unit tests for the shared bounded HTTP response body reader."""

from unittest.mock import AsyncMock, MagicMock

import pytest


def _make_streaming_response(
    chunks: list[bytes],
    *,
    headers: dict[str, str] | None = None,
    content_length: int | None = None,
):
    response = MagicMock()
    response.headers = dict(headers) if headers is not None else {}
    response.content_length = content_length
    response.content = MagicMock()
    response.content.read = AsyncMock(side_effect=list(chunks))
    return response


def test_bounded_body_error_is_value_error_free_exception():
    """L2: BoundedBodyError is an Exception subclass and NOT a ValueError subclass."""
    from news_creator.gateway.bounded_body import BoundedBodyError

    assert issubclass(BoundedBodyError, Exception)
    assert not issubclass(BoundedBodyError, ValueError)


@pytest.mark.asyncio
async def test_read_bounded_text_empty_body():
    """L2: Empty body (immediate EOF b'') returns empty string."""
    from news_creator.gateway.bounded_body import read_bounded_text

    response = _make_streaming_response([b""])
    text = await read_bounded_text(response, cap=64)

    assert text == ""
    response.content.read.assert_awaited_once_with(65)


@pytest.mark.asyncio
async def test_read_bounded_text_multi_chunk_body_loops_until_eof():
    """L2: Multi-chunk body loops on response.content.read(cap + 1 - total) until b''."""
    from news_creator.gateway.bounded_body import read_bounded_text

    response = _make_streaming_response([b"alpha-", b"beta-", b"gamma", b""])
    text = await read_bounded_text(response, cap=64)

    assert text == "alpha-beta-gamma"
    requested_sizes = [call.args[0] for call in response.content.read.await_args_list]
    assert requested_sizes == [65, 59, 54, 49]


@pytest.mark.asyncio
async def test_read_bounded_text_accepts_body_of_exact_cap_bytes():
    """L2: A body of exactly cap bytes is accepted and decoded."""
    from news_creator.gateway.bounded_body import read_bounded_text

    cap = 32
    response = _make_streaming_response([b"a" * 12, b"b" * 20, b""])
    text = await read_bounded_text(response, cap=cap)

    assert text == ("a" * 12) + ("b" * 20)
    assert len(text.encode("utf-8")) == cap


@pytest.mark.parametrize(
    "chunks",
    [
        [b"x" * 33, b""],
        [b"x" * 20, b"y" * 13, b""],
    ],
)
@pytest.mark.asyncio
async def test_read_bounded_text_rejects_body_of_cap_plus_one_bytes(
    chunks: list[bytes],
):
    """L2: A body of cap + 1 bytes raises BoundedBodyError (not ValueError)."""
    from news_creator.gateway.bounded_body import BoundedBodyError, read_bounded_text

    response = _make_streaming_response(chunks)
    with pytest.raises(BoundedBodyError) as exc_info:
        await read_bounded_text(response, cap=32)

    assert not isinstance(exc_info.value, ValueError)


@pytest.mark.asyncio
async def test_read_bounded_text_decodes_utf8_with_replacement():
    """L2: Invalid UTF-8 sequences are decoded with errors='replace'."""
    from news_creator.gateway.bounded_body import read_bounded_text

    response = _make_streaming_response([b"ok-\xff", b"\xfe-end", b""])
    text = await read_bounded_text(response, cap=64)

    assert "ok-" in text
    assert "-end" in text
    assert "\ufffd" in text
