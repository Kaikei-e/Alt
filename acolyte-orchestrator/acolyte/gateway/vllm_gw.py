"""vLLM gateway — LLMProviderPort implementation via OpenAI-compatible API.

Design notes:
- vLLM serves Qwen3.5-27B via /v1/chat/completions (OpenAI format).
- Structured output: response_format with json_schema, thinking disabled.
- Free-text generation: no response_format, thinking optionally enabled.
- num_predict maps to max_tokens, options map to top-level params.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.port.llm_provider import LLMMode, LLMResponse

if TYPE_CHECKING:
    import httpx

    from acolyte.config.settings import Settings

logger = structlog.get_logger(__name__)


class VllmGateway:
    """LLM text generation via vLLM OpenAI-compatible API."""

    def __init__(self, http_client: httpx.AsyncClient, settings: Settings) -> None:
        self._client = http_client
        self._base_url = settings.news_creator_url.rstrip("/")
        self._default_model = settings.default_model
        self._default_num_predict = settings.default_num_predict
        self._api_key = settings.vllm_api_key
        self._mode_defaults = {
            LLMMode.STRUCTURED: {
                "temperature": settings.structured_temperature,
                "num_predict": settings.structured_num_predict,
            },
            LLMMode.LONGFORM: {
                "temperature": settings.longform_temperature,
                "num_predict": settings.longform_num_predict,
            },
        }

    async def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        temperature: float | None = None,
        format: dict | None = None,
        think: bool | None = None,
        mode: LLMMode | None = None,
    ) -> LLMResponse:
        """Generate text via vLLM OpenAI-compatible API."""
        if mode is not None:
            defaults = self._mode_defaults[mode]
            resolved_temp = temperature if temperature is not None else defaults["temperature"]
            resolved_predict = num_predict or defaults["num_predict"]
        else:
            resolved_temp = temperature if temperature is not None else 0.7
            resolved_predict = num_predict or self._default_num_predict

        resolved_model = model or self._default_model
        use_structured = mode == LLMMode.STRUCTURED or (mode is None and format is not None)

        payload: dict = {
            "model": resolved_model,
            "messages": [{"role": "user", "content": prompt}],
            "max_tokens": resolved_predict,
            "temperature": resolved_temp,
            "stream": False,
        }

        # Structured output: wrap format into response_format
        if use_structured and format is not None:
            payload["response_format"] = {
                "type": "json_schema",
                "json_schema": {"name": "output", "schema": format},
            }

        # Thinking control via chat_template_kwargs
        if use_structured:
            payload["chat_template_kwargs"] = {"enable_thinking": False}
        elif think is not None:
            payload["chat_template_kwargs"] = {"enable_thinking": think}

        # Authorization header
        headers: dict[str, str] = {"content-type": "application/json"}
        if self._api_key:
            headers["authorization"] = f"Bearer {self._api_key}"

        url = (
            f"{self._base_url}/chat/completions"
            if self._base_url.endswith("/v1")
            else f"{self._base_url}/v1/chat/completions"
        )

        logger.info(
            "vLLM chat",
            model=resolved_model,
            prompt_len=len(prompt),
            max_tokens=resolved_predict,
            temperature=resolved_temp,
            structured=use_structured,
        )

        resp = await self._client.post(url, json=payload, headers=headers)
        resp.raise_for_status()
        data = resp.json()

        text = data["choices"][0]["message"]["content"]
        usage = data.get("usage", {})

        logger.info(
            "vLLM response",
            model=data.get("model", ""),
            response_len=len(text),
            prompt_tokens=usage.get("prompt_tokens", 0),
            completion_tokens=usage.get("completion_tokens", 0),
        )

        return LLMResponse(
            text=text,
            model=data.get("model", resolved_model),
            prompt_tokens=usage.get("prompt_tokens", 0),
            completion_tokens=usage.get("completion_tokens", 0),
        )
