"""Unit tests for synthesize handler."""

from unittest.mock import AsyncMock

from fastapi.testclient import TestClient

from tts_speaker.domain.errors import (
    AudioFormatError,
    EmptyTextError,
    TextTooLongError,
    UpstreamAuthError,
    UpstreamRejectedError,
    UpstreamUnavailableError,
)
from tts_speaker.main import create_app
from tts_speaker.usecase.synthesize_usecase import SynthesisResult, SynthesizeUsecase


def test_synthesize_success(sample_wav_bytes: bytes) -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.return_value = SynthesisResult(
        wav=sample_wav_bytes,
        chunk_count=2,
        duration_seconds=1.2345,
    )

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post(
        "/v1/synthesize",
        json={"text": "こんにちは世界", "speed": 1.1},
    )

    assert response.status_code == 200
    assert response.headers.get("content-type") == "audio/wav"
    assert response.headers.get("X-TTS-Chunk-Count") == "2"
    assert response.headers.get("X-TTS-Duration-Seconds") == "1.235"
    assert response.content == sample_wav_bytes
    mock_usecase.execute.assert_called_once_with("こんにちは世界", 1.1)


def test_synthesize_default_speed(sample_wav_bytes: bytes) -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.return_value = SynthesisResult(
        wav=sample_wav_bytes,
        chunk_count=1,
        duration_seconds=0.1,
    )

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "テスト"})
    assert response.status_code == 200
    mock_usecase.execute.assert_called_once_with("テスト", 1.0)


def test_extra_fields_forbidden() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post(
        "/v1/synthesize",
        json={"text": "テスト", "extra_field": "not_allowed"},
    )
    assert response.status_code == 422


def test_empty_or_missing_text_rejected() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    # Empty string
    res1 = client.post("/v1/synthesize", json={"text": ""})
    assert res1.status_code == 422

    # Missing text field
    res2 = client.post("/v1/synthesize", json={"speed": 1.0})
    assert res2.status_code == 422


def test_speed_out_of_bounds() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    # Below 0.5
    res1 = client.post("/v1/synthesize", json={"text": "テスト", "speed": 0.4})
    assert res1.status_code == 422

    # Above 2.0
    res2 = client.post("/v1/synthesize", json={"text": "テスト", "speed": 2.1})
    assert res2.status_code == 422


def test_empty_text_error_from_usecase_maps_to_422() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.side_effect = EmptyTextError("Empty text")

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "   "})
    assert response.status_code == 422
    assert "detail" in response.json()


def test_text_too_long_error_from_usecase_maps_to_422() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.side_effect = TextTooLongError("Text exceeds maximum character limit")

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "長い文章"})
    assert response.status_code == 422
    assert "detail" in response.json()


def test_upstream_unavailable_maps_to_503() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.side_effect = UpstreamUnavailableError("Upstream server is unreachable")

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "テスト"})
    assert response.status_code == 503
    assert "detail" in response.json()


def test_upstream_auth_error_maps_to_502() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.side_effect = UpstreamAuthError("Unauthorized upstream")

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "テスト"})
    assert response.status_code == 502
    assert "detail" in response.json()


def test_upstream_rejected_error_maps_to_502() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.side_effect = UpstreamRejectedError(status_code=400, detail="Voice error")

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "テスト"})
    assert response.status_code == 502
    assert "detail" in response.json()


def test_audio_format_error_maps_to_502() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.side_effect = AudioFormatError("Invalid WAV format")

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "テスト"})
    assert response.status_code == 502
    assert "detail" in response.json()


def test_error_body_does_not_leak_secrets_or_urls() -> None:
    secret_url = "http://internal-irodori.secret.corp:8000"
    secret_key = "sensitive-api-token-999"
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.execute.side_effect = UpstreamUnavailableError(
        f"Failed to connect to {secret_url} with key {secret_key}"
    )

    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)

    response = client.post("/v1/synthesize", json={"text": "テスト"})
    detail_str = str(response.json())
    assert secret_url not in detail_str
    assert secret_key not in detail_str
