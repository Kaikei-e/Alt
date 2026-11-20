"""Generate handler - REST endpoint for generic LLM generation."""

import asyncio
import logging
from typing import Any

import aiohttp
from fastapi import APIRouter, HTTPException
from fastapi.responses import JSONResponse

from news_creator.domain.errors import QueueFullError
from news_creator.domain.models import GenerateRequest, LLMGenerateResponse
from news_creator.domain.prompts import neutralize_control_tokens, wrap_gemma_prompt
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)


_MAX_PROMPT_LENGTH_CHARS = 240_000


def _extract_generation_overrides(
    request: GenerateRequest,
) -> tuple[int | None, dict[str, Any] | None]:
    num_predict_override = None
    options_override = dict(request.options or {})
    # Remove num_ctx from options to ensure consistent context length for runner reuse
    if "num_ctx" in options_override:
        options_override.pop("num_ctx")
        logger.debug("Removed num_ctx from options to enforce config value")
    if "num_predict" in options_override:
        raw_num_predict = options_override.pop("num_predict")
        try:
            num_predict_override = int(raw_num_predict)
        except (TypeError, ValueError):
            logger.warning(
                "Invalid num_predict override provided; falling back to heuristic",
                extra={"value": raw_num_predict},
            )
    return num_predict_override, options_override or None


def _validate_prompt_length(request: GenerateRequest) -> None:
    # 60k tokens ~ 240,000 chars. Reject anything larger to prevent queue clogging.
    if request.prompt and len(request.prompt) > _MAX_PROMPT_LENGTH_CHARS:
        logger.warning(
            "Rejecting request with excessive prompt length",
            extra={
                "prompt_length": len(request.prompt),
                "limit": _MAX_PROMPT_LENGTH_CHARS,
                "model": request.model,
            },
        )
        # Static validation feedback, safe to surface: it carries only
        # the client's own prompt length and the cap (the E2E contract
        # asserts on it). Raised as HTTPException so the generic
        # except-ValueError sanitizer below cannot swallow it.
        raise HTTPException(
            status_code=400,
            detail=(
                f"Prompt too long ({len(request.prompt)} chars). "
                f"Max {_MAX_PROMPT_LENGTH_CHARS} chars."
            ),
        )


def _prepare_generate_prompt(raw_prompt: str) -> str:
    stripped_prompt = raw_prompt.strip()
    if stripped_prompt.startswith("<|turn>"):
        prompt_to_send = raw_prompt
        prompt_wrapped = False
        control_tokens_removed = 0
    else:
        neutralized_prompt, control_tokens_removed = neutralize_control_tokens(
            stripped_prompt
        )
        prompt_to_send = wrap_gemma_prompt(neutralized_prompt)
        prompt_wrapped = True
    logger.info(
        "generate_prompt_wrapped=%s control_tokens_removed=%d",
        "true" if prompt_wrapped else "false",
        control_tokens_removed,
        extra={
            "generate_prompt_wrapped": prompt_wrapped,
            "control_tokens_removed": control_tokens_removed,
        },
    )
    return prompt_to_send


def _ensure_generate_response(result: object) -> LLMGenerateResponse:
    if not isinstance(result, LLMGenerateResponse):
        msg = "Expected non-streaming LLMGenerateResponse"
        raise TypeError(msg)
    return result


def _format_generate_response(llm_response: LLMGenerateResponse) -> dict[str, Any]:
    response_dict: dict[str, Any] = {
        "model": llm_response.model,
        "response": llm_response.response,
        "done": llm_response.done if llm_response.done is not None else True,
        "done_reason": llm_response.done_reason or "stop",
    }
    if llm_response.prompt_eval_count is not None:
        response_dict["prompt_eval_count"] = llm_response.prompt_eval_count
    if llm_response.eval_count is not None:
        response_dict["eval_count"] = llm_response.eval_count
    if llm_response.total_duration is not None:
        response_dict["total_duration"] = llm_response.total_duration
    return response_dict


async def _handle_generate_request(
    llm_provider: LLMProviderPort,
    request: GenerateRequest,
) -> dict[str, Any] | JSONResponse:
    try:
        num_predict_override, options_override = _extract_generation_overrides(request)
        _validate_prompt_length(request)
        prompt_to_send = _prepare_generate_prompt(request.prompt)

        result = await llm_provider.generate(
            prompt=prompt_to_send,
            model=request.model,
            num_predict=num_predict_override,
            stream=request.stream,
            keep_alive=request.keep_alive,
            options=options_override,
        )
        llm_response = _ensure_generate_response(result)

    except QueueFullError as exc:
        logger.warning(
            "Queue full, returning 429",
            extra={"error": str(exc)},
        )
        return JSONResponse(
            status_code=429,
            content={"error": "queue full"},
            headers={"Retry-After": "30"},
        )

    except ValueError as exc:
        logger.warning("Invalid /api/generate payload", extra={"error": str(exc)})
        raise HTTPException(status_code=400, detail="Invalid request") from exc

    except (TimeoutError, aiohttp.ClientError) as exc:
        # Network/timeout errors are surfaced as 502 here -- a
        # `raise RuntimeError(...)` in this block would NOT be caught by
        # the `except RuntimeError` below (a raise inside an except
        # clause of the same try is not re-matched against sibling
        # except clauses), so it would escape as an unhandled 500.
        logger.exception(
            "Network error in /api/generate",
            extra={
                "error": str(exc),
                "error_type": type(exc).__name__,
                "is_timeout": isinstance(
                    exc, (asyncio.TimeoutError, aiohttp.ServerTimeoutError)
                ),
            },
        )
        raise HTTPException(status_code=502, detail="Upstream service error") from exc

    except RuntimeError as exc:
        logger.exception(
            "LLM generate request failed",
            extra={
                "error": str(exc),
                "error_type": type(exc).__name__,
                "model": request.model if hasattr(request, "model") else None,
            },
        )
        raise HTTPException(status_code=502, detail="Upstream service error") from exc

    except HTTPException:
        # Deliberate client-facing errors (the prompt-length 400 above)
        # pass through; without this, the catch-all below would rewrite
        # them into a 500.
        raise

    except Exception as exc:
        logger.exception(
            "Unhandled error in /api/generate",
            extra={
                "error_type": type(exc).__name__,
                "error": str(exc),
            },
        )
        raise HTTPException(status_code=500, detail="Internal server error") from exc
    else:
        return _format_generate_response(llm_response)


def create_generate_router(llm_provider: LLMProviderPort) -> APIRouter:
    """
    Create generate router with dependency injection.

    Args:
        llm_provider: LLM provider instance

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.post("/api/generate", response_model=None)
    async def generate_endpoint(
        request: GenerateRequest,
    ) -> dict[str, Any] | JSONResponse:
        """
        Forward Ollama-compatible generate requests.

        Args:
            request: Generate request with prompt and options

        Returns:
            Dict with LLM response in Ollama format

        Raises:
            HTTPException: 400 for invalid request, 502 for LLM errors, 500 for unexpected errors
        """
        return await _handle_generate_request(llm_provider, request)

    return router
