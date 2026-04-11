"""Ollama gateway — LLMProviderPort implementation via Ollama API directly.

Design notes (from ADR-579, ADR-632):
- All requests MUST use consistent base options (num_batch, num_keep, stop)
  to prevent Ollama model reload which causes 259s TTFT degradation.
- For structured output: temperature=0, reasoning-first field order in prompts
- For free-text generation: temperature=0.7
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.port.llm_provider import LLMMode, LLMResponse

if TYPE_CHECKING:
    import httpx

    from acolyte.config.settings import Settings

logger = structlog.get_logger(__name__)


def _build_base_options(settings: Settings) -> dict:
    """Build base options from settings. Consistent across all requests (ADR-579)."""
    opts: dict = {
        "num_batch": 1024,
        "num_keep": -1,
        "num_ctx": settings.llm_num_ctx,
    }
    if settings.llm_stop_tokens:
        opts["stop"] = [t.strip() for t in settings.llm_stop_tokens.split(",") if t.strip()]
    return opts


class OllamaGateway:
    """LLM text generation via Ollama API.

    Structured output (format != None): uses /api/chat with think=false.
    Free-text generation (format == None): uses /api/generate with optional thinking.
    """

    def __init__(self, http_client: httpx.AsyncClient, settings: Settings) -> None:
        self._client = http_client
        self._base_url = settings.news_creator_url
        self._default_model = settings.default_model
        self._default_num_predict = settings.default_num_predict
        self._base_options = _build_base_options(settings)
        self._longform_think = settings.longform_think
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
        """Generate text via Ollama.

        When mode is set, uses mode defaults for temperature/num_predict (explicit kwargs override).
        Routes: STRUCTURED → /api/chat, LONGFORM → /api/generate, None → format-based routing.
        """
        # Resolve defaults: mode defaults → explicit kwargs → gateway defaults
        if mode is not None:
            defaults = self._mode_defaults[mode]
            resolved_temp = temperature if temperature is not None else defaults["temperature"]
            resolved_predict = num_predict or defaults["num_predict"]
        else:
            resolved_temp = temperature if temperature is not None else 0.7
            resolved_predict = num_predict or self._default_num_predict

        options = {
            **self._base_options,
            "num_predict": resolved_predict,
            "temperature": resolved_temp,
        }

        resolved_model = model or self._default_model

        # Endpoint routing: mode-based when set, format-based otherwise
        if mode == LLMMode.STRUCTURED:
            if format:
                return await self._generate_structured(prompt, resolved_model, options, format)
            # XML DSL nodes: /api/chat without format, think=false (#14793)
            return await self._generate_chat_freetext(prompt, resolved_model, options, think=False)
        if mode == LLMMode.LONGFORM:
            # Writer: /api/chat without format, think controlled by setting (#14793)
            return await self._generate_chat_freetext(prompt, resolved_model, options, think=self._longform_think)
        # Fallback: format-based routing (backward compat)
        if format is not None:
            return await self._generate_structured(prompt, resolved_model, options, format)
        return await self._generate_freetext(prompt, resolved_model, options, think=think)

    async def _generate_structured(
        self,
        prompt: str,
        model: str,
        options: dict,
        format: dict,
    ) -> LLMResponse:
        """Structured output via /api/chat. No think parameter when format is set (Gemma4 #15260)."""
        payload: dict = {
            "model": model,
            "messages": [{"role": "user", "content": prompt}],
            "format": format,
            "stream": False,
            "options": options,
        }

        logger.info(
            "Ollama chat (structured)",
            model=model,
            prompt_len=len(prompt),
            num_predict=options["num_predict"],
            temperature=options["temperature"],
        )

        resp = await self._client.post(f"{self._base_url}/api/chat", json=payload)
        resp.raise_for_status()
        data = resp.json()

        text = data.get("message", {}).get("content", "")
        return self._build_response(data, text, model)

    async def _generate_chat_freetext(
        self,
        prompt: str,
        model: str,
        options: dict,
        *,
        think: bool = False,
    ) -> LLMResponse:
        """Free-text generation via /api/chat without format.

        Uses /api/chat instead of /api/generate because Ollama /api/generate
        ignores think=false for Qwen3.5 (#14793). /api/chat respects think
        as a top-level parameter for all models.
        """
        payload: dict = {
            "model": model,
            "messages": [{"role": "user", "content": prompt}],
            "stream": False,
            "options": options,
            "think": think,
        }

        logger.info(
            "Ollama chat (freetext)",
            model=model,
            prompt_len=len(prompt),
            num_predict=options["num_predict"],
            temperature=options["temperature"],
            think=think,
        )

        resp = await self._client.post(f"{self._base_url}/api/chat", json=payload)
        resp.raise_for_status()
        data = resp.json()

        text = data.get("message", {}).get("content", "")
        return self._build_response(data, text, model)

    async def _generate_freetext(
        self,
        prompt: str,
        model: str,
        options: dict,
        *,
        think: bool | None = None,
    ) -> LLMResponse:
        """Free-text generation via /api/generate. Thinking mode allowed."""
        payload: dict = {
            "model": model,
            "prompt": prompt,
            "stream": False,
            "options": options,
        }
        if think is not None:
            payload["think"] = think

        logger.info(
            "Ollama generate (freetext)",
            model=model,
            prompt_len=len(prompt),
            num_predict=options["num_predict"],
            temperature=options["temperature"],
        )

        resp = await self._client.post(f"{self._base_url}/api/generate", json=payload)
        resp.raise_for_status()
        data = resp.json()

        text = data.get("response", "")
        return self._build_response(data, text, model)

    def _build_response(self, data: dict, text: str, model: str) -> LLMResponse:
        """Build LLMResponse from Ollama API response."""
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
            model=data.get("model", model),
            prompt_tokens=data.get("prompt_eval_count", 0),
            completion_tokens=data.get("eval_count", 0),
        )
