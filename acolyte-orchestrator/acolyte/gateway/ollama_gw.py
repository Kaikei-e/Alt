"""Ollama gateway — LLMProviderPort implementation via Ollama API directly.

Design notes (from ADR-579, ADR-632):
- All requests MUST use consistent base options (num_batch, num_keep, stop)
  to prevent Ollama model reload which causes 259s TTFT degradation.
- For structured output: temperature=0, reasoning-first field order in prompts
- For free-text generation: temperature=0.7
- num_predict=2048 for 26B model (larger than 8B's 1200)
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.port.llm_provider import LLMResponse

if TYPE_CHECKING:
    import httpx

    from acolyte.config.settings import Settings

logger = structlog.get_logger(__name__)

# Base options — consistent across ALL requests to prevent Ollama model reload (ADR-579)
_BASE_OPTIONS = {
    "num_batch": 1024,
    "num_keep": -1,
    "num_ctx": 12288,  # 32GB VRAM target — 12K context for Gemma4 26B
    "stop": ["<end_of_turn>"],
}


class OllamaGateway:
    """LLM text generation via Ollama /api/generate endpoint."""

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
        format: dict | None = None,
    ) -> LLMResponse:
        """Generate text via Ollama /api/generate.

        Options are merged on top of _BASE_OPTIONS to ensure model reload prevention.
        Pass `format` (JSON schema dict) to enable Ollama structured output (GBNF grammar).
        NOTE: Do NOT set think=false with format — Gemma4 bug (ollama/ollama#15260).
        """
        options = {
            **_BASE_OPTIONS,
            "num_predict": num_predict or self._default_num_predict,
            "temperature": temperature if temperature is not None else 0.7,
        }

        payload: dict = {
            "model": model or self._default_model,
            "prompt": prompt,
            "stream": False,
            "options": options,
        }
        if format is not None:
            payload["format"] = format

        logger.info(
            "Ollama generate",
            model=payload["model"],
            prompt_len=len(prompt),
            num_predict=options["num_predict"],
            temperature=options["temperature"],
        )

        resp = await self._client.post(
            f"{self._base_url}/api/generate",
            json=payload,
        )
        resp.raise_for_status()
        data = resp.json()

        text = data.get("response", "")
        eval_dur = data.get("eval_duration", 0)
        logger.info(
            "Ollama response",
            model=data.get("model", ""),
            response_len=len(text),
            eval_count=data.get("eval_count", 0),
            eval_duration_ms=eval_dur // 1_000_000 if eval_dur else 0,
        )

        return LLMResponse(
            text=text,
            model=data.get("model", model or self._default_model),
            prompt_tokens=data.get("prompt_eval_count", 0),
            completion_tokens=data.get("eval_count", 0),
        )
