"""Unit tests for TTSConnectService handler."""

from collections.abc import AsyncIterator
from unittest.mock import AsyncMock

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError
from connectrpc.method import IdempotencyLevel, MethodInfo
from connectrpc.request import Headers, RequestContext

import tts_speaker.gen  # noqa: F401
from tts_speaker.domain.errors import (
    AudioFormatError,
    EmptyTextError,
    SynthesisBusyError,
    TextTooLongError,
    UpstreamAuthError,
    UpstreamRejectedError,
    UpstreamUnavailableError,
)
from tts_speaker.gen.proto.alt.tts.v1.tts_pb2 import (
    SynthesizeStreamRequest,
    SynthesizeStreamResponse,
)
from tts_speaker.handler.tts_connect_service import TTSConnectService
from tts_speaker.usecase.synthesize_usecase import ChunkAudio, SynthesizeUsecase


def _make_context() -> RequestContext:
    method_info = MethodInfo(
        name="SynthesizeStream",
        service_name="alt.tts.v1.TTSService",
        input=SynthesizeStreamRequest,
        output=SynthesizeStreamResponse,
        idempotency_level=IdempotencyLevel.UNKNOWN,
    )
    return RequestContext(
        method=method_info,
        http_method="POST",
        request_headers=Headers(),
    )


async def _async_iter(items: list[ChunkAudio]) -> AsyncIterator[ChunkAudio]:
    for item in items:
        yield item


async def _async_error(exc: Exception) -> AsyncIterator[ChunkAudio]:
    raise exc
    if False:
        yield


async def test_synthesize_stream_single_chunk(sample_wav_bytes: bytes) -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.stream.return_value = _async_iter(
        [ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.1)]
    )

    service = TTSConnectService(usecase=mock_usecase)
    request = SynthesizeStreamRequest(text="hello", speed=1.2)
    ctx = _make_context()

    responses = [resp async for resp in service.synthesize_stream(request, ctx)]
    assert len(responses) == 1
    resp = responses[0]
    assert isinstance(resp, SynthesizeStreamResponse)
    assert resp.audio_wav == sample_wav_bytes
    assert resp.sample_rate == 48000
    assert pytest.approx(resp.duration_seconds, abs=0.01) == 0.1
    mock_usecase.stream.assert_called_once_with("hello", speed=1.2)


async def test_synthesize_stream_multiple_chunks(sample_wav_bytes: bytes) -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.stream.return_value = _async_iter(
        [
            ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.1),
            ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.2),
            ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.1),
        ]
    )

    service = TTSConnectService(usecase=mock_usecase)
    request = SynthesizeStreamRequest(text="長文テスト")
    ctx = _make_context()

    responses = [resp async for resp in service.synthesize_stream(request, ctx)]
    assert len(responses) == 3
    assert all(isinstance(r, SynthesizeStreamResponse) for r in responses)
    assert responses[1].duration_seconds == pytest.approx(0.2, abs=0.01)
    mock_usecase.stream.assert_called_once_with("長文テスト", speed=None)


async def test_synthesize_stream_default_speed_when_unset() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.stream.return_value = _async_iter([])

    service = TTSConnectService(usecase=mock_usecase)
    request = SynthesizeStreamRequest(text="テスト")
    ctx = _make_context()

    _ = [resp async for resp in service.synthesize_stream(request, ctx)]
    mock_usecase.stream.assert_called_once_with("テスト", speed=None)


async def test_synthesize_stream_speed_below_min_rejected() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    service = TTSConnectService(usecase=mock_usecase)
    request = SynthesizeStreamRequest(text="テスト", speed=0.4)
    ctx = _make_context()

    with pytest.raises(ConnectError) as exc_info:
        async for _ in service.synthesize_stream(request, ctx):
            pass

    assert exc_info.value.code == Code.INVALID_ARGUMENT
    mock_usecase.stream.assert_not_called()


async def test_synthesize_stream_speed_above_max_rejected() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    service = TTSConnectService(usecase=mock_usecase)
    request = SynthesizeStreamRequest(text="テスト", speed=1.6)
    ctx = _make_context()

    with pytest.raises(ConnectError) as exc_info:
        async for _ in service.synthesize_stream(request, ctx):
            pass

    assert exc_info.value.code == Code.INVALID_ARGUMENT
    mock_usecase.stream.assert_not_called()


@pytest.mark.parametrize(
    ("domain_err", "expected_code"),
    [
        (EmptyTextError("Text is empty"), Code.INVALID_ARGUMENT),
        (TextTooLongError("Text exceeds maximum"), Code.INVALID_ARGUMENT),
        (UpstreamUnavailableError("Upstream service down"), Code.UNAVAILABLE),
        (SynthesisBusyError("Queue timeout exceeded"), Code.UNAVAILABLE),
        (UpstreamAuthError("Unauthorized key"), Code.INTERNAL),
        (UpstreamRejectedError(400, "Bad request from upstream"), Code.INTERNAL),
        (AudioFormatError("Corrupt WAV header"), Code.INTERNAL),
    ],
)
async def test_error_mapping_to_connect_codes(domain_err: Exception, expected_code: Code) -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.stream.return_value = _async_error(domain_err)

    service = TTSConnectService(usecase=mock_usecase)
    request = SynthesizeStreamRequest(text="error-trigger")
    ctx = _make_context()

    with pytest.raises(ConnectError) as exc_info:
        async for _ in service.synthesize_stream(request, ctx):
            pass

    assert exc_info.value.code == expected_code


async def test_error_message_never_leaks_upstream_url_or_key() -> None:
    leaky_err = UpstreamAuthError("Failed request to http://irodori-tts:8088/v1/audio/speech with secret_api_key_12345")
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.stream.return_value = _async_error(leaky_err)

    service = TTSConnectService(usecase=mock_usecase)
    request = SynthesizeStreamRequest(text="sensitive")
    ctx = _make_context()

    with pytest.raises(ConnectError) as exc_info:
        async for _ in service.synthesize_stream(request, ctx):
            pass

    msg = exc_info.value.message
    assert "irodori-tts" not in msg
    assert "8088" not in msg
    assert "secret_api_key" not in msg
    assert "http://" not in msg
