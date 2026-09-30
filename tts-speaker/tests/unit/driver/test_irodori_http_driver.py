"""Unit tests for IrodoriHttpDriver."""

import json

import httpx

from tts_speaker.driver.irodori_http_driver import IrodoriHttpDriver


async def test_post_speech_sends_correct_request() -> None:
    captured_request: httpx.Request | None = None

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal captured_request
        captured_request = request
        return httpx.Response(200, json={"status": "ok"})

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://irodori.test") as client:
        driver = IrodoriHttpDriver(client=client, api_key="secret-token-xyz")
        payload = {"model": "irodori-tts", "input": "テスト", "voice": "v1"}
        response = await driver.post_speech(payload)

        assert response.status_code == 200
        assert captured_request is not None
        assert captured_request.method == "POST"
        assert captured_request.url.path == "/v1/audio/speech"
        assert captured_request.headers.get("Authorization") == "Bearer secret-token-xyz"
        assert json.loads(captured_request.content) == payload


async def test_post_speech_returns_error_response_without_raising() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, json={"error": "internal"})

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://irodori.test") as client:
        driver = IrodoriHttpDriver(client=client, api_key="secret-token-xyz")
        response = await driver.post_speech({"model": "test"})
        assert response.status_code == 500
