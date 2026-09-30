"""Unit tests for IrodoriGateway."""

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
