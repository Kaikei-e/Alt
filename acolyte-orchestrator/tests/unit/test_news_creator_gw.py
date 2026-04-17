"""Unit tests for news-creator gateway."""

from __future__ import annotations

import httpx
import pytest

from acolyte.config.settings import Settings
from acolyte.gateway.news_creator_gw import NewsCreatorGateway


@pytest.fixture
def settings() -> Settings:
    return Settings(news_creator_url="http://fake:11434")


@pytest.fixture
def mock_transport() -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        if request.url.path == "/api/v1/summarize":
            return httpx.Response(
                200,
                json={
                    "success": True,
                    "article_id": "acolyte-gen",
                    "summary": "Generated text output.",
                    "model": "gemma4-e4b-12k",
                    "prompt_tokens": 17,
                    "completion_tokens": 42,
                    "total_duration_ms": 1234.5,
                },
            )
        return httpx.Response(404)

    return httpx.MockTransport(handler)


@pytest.mark.asyncio
async def test_generate_returns_llm_response(settings: Settings, mock_transport: httpx.MockTransport) -> None:
    async with httpx.AsyncClient(transport=mock_transport, base_url="http://fake:11434") as client:
        gw = NewsCreatorGateway(client, settings)
        result = await gw.generate("Write an executive summary.")

    assert result.text == "Generated text output."
    assert result.model == "gemma4-e4b-12k"
    assert result.prompt_tokens == 17
    assert result.completion_tokens == 42


@pytest.mark.asyncio
async def test_generate_tolerates_missing_token_counts(settings: Settings) -> None:
    """news-creator returns prompt_tokens/completion_tokens as Optional[int]; treat None as 0."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "success": True,
                "article_id": "acolyte-gen",
                "summary": "Short output.",
                "model": "gemma4-e4b-12k",
                "prompt_tokens": None,
                "completion_tokens": None,
                "total_duration_ms": None,
            },
        )

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:11434") as client:
        gw = NewsCreatorGateway(client, settings)
        result = await gw.generate("test")

    assert result.prompt_tokens == 0
    assert result.completion_tokens == 0


@pytest.mark.asyncio
async def test_generate_raises_on_server_error(settings: Settings) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, json={"error": "internal"})

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:11434") as client:
        gw = NewsCreatorGateway(client, settings)
        with pytest.raises(httpx.HTTPStatusError):
            await gw.generate("test prompt")
