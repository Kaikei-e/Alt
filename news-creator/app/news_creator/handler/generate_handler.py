"""Generate handler - REST endpoint for generic LLM generation."""

import asyncio
import logging
import aiohttp
from fastapi import APIRouter, HTTPException
from typing import Dict, Any

from news_creator.domain.models import GenerateRequest
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

router = APIRouter()


def create_generate_router(llm_provider: LLMProviderPort) -> APIRouter:
    """
    Create generate router with dependency injection.

    Args:
        llm_provider: LLM provider instance

    Returns:
        Configured APIRouter
    """
    @router.post("/api/generate")
    async def generate_endpoint(request: GenerateRequest) -> Dict[str, Any]:
        """
        Forward Ollama-compatible generate requests.

        Args:
            request: Generate request with prompt and options

        Returns:
            Dict with LLM response in Ollama format

        Raises:
            HTTPException: 400 for invalid request, 502 for LLM errors, 500 for unexpected errors
        """
        try:
            # Extract num_predict from options if present
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

            # Protection: Validate prompt length
            # 60k tokens ~ 240,000 chars. Reject anything larger to prevent queue clogging.
            MAX_PROMPT_LENGTH_CHARS = 240_000
            if request.prompt and len(request.prompt) > MAX_PROMPT_LENGTH_CHARS:
                 logger.warning(
                     "Rejecting request with excessive prompt length",
                     extra={
                         "prompt_length": len(request.prompt),
                         "limit": MAX_PROMPT_LENGTH_CHARS,
                         "model": request.model
                     }
                 )
                 raise ValueError(f"Prompt too long ({len(request.prompt)} chars). Max {MAX_PROMPT_LENGTH_CHARS} chars.")

            # Call LLM provider
            llm_response = await llm_provider.generate(
                prompt=request.prompt.strip(),
                model=request.model,
                num_predict=num_predict_override,
                stream=request.stream,
                keep_alive=request.keep_alive,
                options=options_override if options_override else None,
            )

            # Format response in Ollama format
            response_dict = {
                "model": llm_response.model,
                "response": llm_response.response,
                "done": llm_response.done if llm_response.done is not None else True,
                "done_reason": llm_response.done_reason or "stop",
            }

            # Add optional fields if present
            if llm_response.prompt_eval_count is not None:
                response_dict["prompt_eval_count"] = llm_response.prompt_eval_count
            if llm_response.eval_count is not None:
                response_dict["eval_count"] = llm_response.eval_count
            if llm_response.total_duration is not None:
                response_dict["total_duration"] = llm_response.total_duration

            return response_dict

        except ValueError as exc:
            logger.warning("Invalid /api/generate payload", extra={"error": str(exc)})
            raise HTTPException(status_code=400, detail=str(exc)) from exc

        except (aiohttp.ClientError, asyncio.TimeoutError) as exc:
            # Convert network/timeout errors to RuntimeError for consistent handling
            error_msg = f"Network error during LLM request: {type(exc).__name__} - {str(exc)}"
            logger.error(
                "Network error in /api/generate",
                extra={
                    "error": str(exc),
                    "error_type": type(exc).__name__,
                    "is_timeout": isinstance(exc, (asyncio.TimeoutError, aiohttp.ServerTimeoutError)),
                },
                exc_info=True,
            )
            # Convert to RuntimeError so it's handled as a 502 (Bad Gateway)
            raise RuntimeError(error_msg) from exc

        except RuntimeError as exc:
            error_detail = str(exc)
            logger.error(
                "LLM generate request failed",
                extra={
                    "error": error_detail,
                    "error_type": type(exc).__name__,
                    "model": request.model if hasattr(request, "model") else None,
                },
                exc_info=True,  # Include full traceback for debugging
            )
            raise HTTPException(status_code=502, detail=error_detail) from exc

        except Exception as exc:
            logger.exception(
                "Unhandled error in /api/generate",
                extra={
                    "error_type": type(exc).__name__,
                    "error": str(exc),
                },
            )
            raise HTTPException(status_code=500, detail="Internal server error") from exc

    return router
