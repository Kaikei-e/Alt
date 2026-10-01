import logging
from unittest.mock import AsyncMock

import httpx
import pytest

from tts_speaker.domain.errors import (
    AudioFormatError,
    UpstreamAuthError,
    UpstreamRejectedError,
    UpstreamUnavailableError,
)
from tts_speaker.driver.irodori_http_driver import IrodoriHttpDriver
from tts_speaker.gateway.irodori_gateway import IrodoriGateway


async def test_exact_payload_structure(fake_sleep_recorder) -> None:
    fake_sleep, _ = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(
        200,
        content=b"RIFF...",
        headers={"content-type": "audio/wav"},
    )

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="speaker_test",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    result = await gateway.synthesize_chunk("テスト文章", speed=1.2)
    assert result == b"RIFF..."

    mock_driver.post_speech.assert_called_once_with(
        {
            "model": "irodori-tts",
            "input": "テスト文章",
            "voice": "speaker_test",
            "response_format": "wav",
            "speed": 1.2,
            "irodori": {"chunking_enabled": False},
        }
    )


async def test_audio_format_error_on_non_wav_content_type(fake_sleep_recorder) -> None:
    fake_sleep, _ = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(
        200,
        content=b'{"error": "not wav"}',
        headers={"content-type": "application/json"},
    )

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(AudioFormatError):
        await gateway.synthesize_chunk("テスト", speed=1.0)


async def test_retry_schedule_5xx_exhausts_attempts(fake_sleep_recorder) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(
        500,
        content=b"Server error",
        headers={"content-type": "text/plain"},
    )

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamUnavailableError):
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert mock_driver.post_speech.call_count == 3
    # Retry schedule: attempt 1 fails -> sleep 1.0 * 2^0 = 1.0; attempt 2 fails -> sleep 1.0 * 2^1 = 2.0
    assert delays == [1.0, 2.0]


async def test_retry_schedule_transport_error(fake_sleep_recorder) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.side_effect = httpx.ConnectError("Connection refused")

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamUnavailableError):
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert mock_driver.post_speech.call_count == 3
    assert delays == [1.0, 2.0]


async def test_retry_succeeds_on_second_attempt(fake_sleep_recorder) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.side_effect = [
        httpx.Response(503, content=b"Unavailable"),
        httpx.Response(200, content=b"RIFF-WAV", headers={"content-type": "audio/wav"}),
    ]

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    result = await gateway.synthesize_chunk("テスト", speed=1.0)
    assert result == b"RIFF-WAV"
    assert mock_driver.post_speech.call_count == 2
    assert delays == [1.0]


async def test_401_raises_upstream_auth_error_no_retry(fake_sleep_recorder) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(401, content=b"Unauthorized")

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamAuthError):
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert mock_driver.post_speech.call_count == 1
    assert delays == []


@pytest.mark.parametrize("status_code", [400, 404, 422])
async def test_other_4xx_raises_upstream_rejected_error_no_retry(fake_sleep_recorder, status_code: int) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(status_code, content=b"Client error detail")

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamRejectedError) as exc_info:
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert exc_info.value.status_code == status_code
    assert "Client error detail" in exc_info.value.detail
    assert mock_driver.post_speech.call_count == 1
    assert delays == []


async def test_upstream_rejected_error_detail_truncated_to_200_chars(fake_sleep_recorder) -> None:
    fake_sleep, delays = fake_sleep_recorder
    long_body = "x" * 500
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(400, text=long_body)

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamRejectedError) as exc_info:
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert exc_info.value.status_code == 400
    assert len(exc_info.value.detail) == 200
    assert exc_info.value.detail == "x" * 200
    assert mock_driver.post_speech.call_count == 1
    assert delays == []


async def test_error_message_never_contains_api_key(fake_sleep_recorder) -> None:
    api_key_secret = "super-confidential-api-key-999"
    fake_sleep, _ = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.side_effect = httpx.ConnectError(f"Failed connecting with {api_key_secret}")

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=1,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamUnavailableError) as exc_info:
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert api_key_secret not in str(exc_info.value)


@pytest.mark.parametrize("status_code", [408, 429])
async def test_retry_schedule_408_and_429(fake_sleep_recorder, status_code: int) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(
        status_code,
        content=b"Rate limited or timeout",
        headers={"content-type": "text/plain"},
    )

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamUnavailableError):
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert mock_driver.post_speech.call_count == 3
    assert delays == [1.0, 2.0]


async def test_read_timeout_raises_immediately_no_retry(fake_sleep_recorder) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    read_timeout = httpx.ReadTimeout("Read timeout past deadline")
    mock_driver.post_speech.side_effect = read_timeout

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamUnavailableError) as exc_info:
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert exc_info.value.__cause__ is read_timeout
    assert mock_driver.post_speech.call_count == 1
    assert delays == []


async def test_transport_error_chains_from_err(fake_sleep_recorder) -> None:
    fake_sleep, _ = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    connect_error = httpx.ConnectError("Connection refused")
    mock_driver.post_speech.side_effect = connect_error

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=1,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamUnavailableError) as exc_info:
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert exc_info.value.__cause__ is connect_error


@pytest.mark.parametrize("status_code", [301, 302, 307])
async def test_3xx_raises_upstream_rejected_error_no_retry(fake_sleep_recorder, status_code: int) -> None:
    fake_sleep, delays = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(status_code, content=b"Redirected")

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=3,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with pytest.raises(UpstreamRejectedError) as exc_info:
        await gateway.synthesize_chunk("テスト", speed=1.0)

    assert exc_info.value.status_code == status_code
    assert mock_driver.post_speech.call_count == 1
    assert delays == []


@pytest.mark.parametrize("content_type", ["audio/wav", "audio/x-wav", "audio/wave", "audio/wav; charset=utf-8"])
async def test_accepted_content_types(fake_sleep_recorder, content_type: str) -> None:
    fake_sleep, _ = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(
        200,
        content=b"RIFF...",
        headers={"content-type": content_type},
    )

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=1,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    result = await gateway.synthesize_chunk("テスト", speed=1.0)
    assert result == b"RIFF..."


async def test_upstream_failure_logs_warning(fake_sleep_recorder, caplog: pytest.LogCaptureFixture) -> None:
    fake_sleep, _ = fake_sleep_recorder
    mock_driver = AsyncMock(spec=IrodoriHttpDriver)
    mock_driver.post_speech.return_value = httpx.Response(500, content=b"Internal server error")

    gateway = IrodoriGateway(
        driver=mock_driver,
        model_name="irodori-tts",
        voice_id="v1",
        max_attempts=1,
        backoff_seconds=1.0,
        sleep=fake_sleep,
    )

    with caplog.at_level(logging.WARNING):
        with pytest.raises(UpstreamUnavailableError):
            await gateway.synthesize_chunk("テスト", speed=1.0)

    warnings = [r for r in caplog.records if r.levelno == logging.WARNING]
    assert len(warnings) >= 1
    msg = warnings[0].getMessage()
    assert "attempt 1/1" in msg
    assert "status=500" in msg
    assert "Internal server error" in msg
