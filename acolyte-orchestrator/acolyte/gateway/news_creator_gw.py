"""News-creator gateway — LLMProviderPort implementation via HTTP."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.port.llm_provider import LLMResponse

if TYPE_CHECKING:
    import httpx

    from acolyte.config.settings import Settings

logger = structlog.get_logger(__name__)


class NewsCreatorGateway:
    """LLM text generation via news-creator's /api/v1/summarize endpoint."""

    def __init__(self, http_client: httpx.AsyncClient, settings: Settings) -> None:
        self._client = http_client
        self._base_url = settings.news_creator_url
        self._default_model = settings.default_model
        self._default_num_predict = settings.default_num_predict

    async def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        temperature: float | None = None,
    ) -> LLMResponse:
        """Generate text via news-creator summarization endpoint."""
        payload: dict = {
            "content": prompt,
            "article_id": "acolyte-gen",
            "priority": "low",
            "stream": False,
        }

        resp = await self._client.post(
            f"{self._base_url}/api/v1/summarize",
            json=payload,
        )
        resp.raise_for_status()
        data = resp.json()

        return LLMResponse(
            text=data.get("summary", ""),
            model=data.get("model") or model or self._default_model,
            prompt_tokens=data.get("prompt_tokens") or 0,
            completion_tokens=data.get("completion_tokens") or 0,
        )
