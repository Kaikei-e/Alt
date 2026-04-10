"""Pydantic-validated LLM structured output utility.

Usage:
    result = await generate_validated(llm, prompt, MyModel, temperature=0)

Flow: LLM generate → json.loads → Pydantic validate → retry on failure → fallback.

Truncation detection: if completion_tokens >= 95% of num_predict and JSON parsing
fails, the response was likely cut off by thinking token budget exhaustion.
In this case, num_predict is increased by 25% (once only) for the retry.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING, Any

import structlog
from pydantic import TypeAdapter

if TYPE_CHECKING:
    from pydantic import BaseModel

    from acolyte.port.llm_provider import LLMProviderPort, LLMResponse

logger = structlog.get_logger(__name__)

# Truncation detection threshold: completion_tokens / num_predict
_TRUNCATION_RATIO = 0.95
# Budget increase factor (applied once only)
_BUDGET_INCREASE = 1.25


async def generate_validated[T: "BaseModel"](
    llm: LLMProviderPort,
    prompt: str,
    model_cls: type[T],
    *,
    retries: int = 1,
    fallback: T | None = None,
    **llm_kwargs: Any,
) -> T:
    """Generate LLM output and validate with Pydantic.

    Args:
        llm: LLM provider port.
        prompt: The prompt to send.
        model_cls: Pydantic model class for validation.
        retries: Number of retries on validation failure (default 1).
        fallback: Value to return if all retries are exhausted. Raises ValueError if None.
        **llm_kwargs: Extra kwargs passed to llm.generate (temperature, num_predict, etc.).
    """
    adapter = TypeAdapter(model_cls)
    # Inject JSON schema as Ollama format parameter for GBNF grammar enforcement
    llm_kwargs.setdefault("format", model_cls.model_json_schema())

    last_error: Exception | None = None
    budget_increased = False
    response: LLMResponse | None = None

    for attempt in range(1 + retries):
        try:
            response = await llm.generate(prompt, **llm_kwargs)
            parsed = json.loads(response.text)
            return adapter.validate_python(parsed)
        except json.JSONDecodeError as exc:
            last_error = exc
            # Detect truncation: thinking tokens exhausted num_predict budget
            num_predict = llm_kwargs.get("num_predict")
            if (
                not budget_increased
                and isinstance(num_predict, int)
                and num_predict > 0
                and response is not None
                and response.completion_tokens >= num_predict * _TRUNCATION_RATIO
            ):
                increased = int(num_predict * _BUDGET_INCREASE)
                logger.warning(
                    "JSON truncated at num_predict limit, increasing budget",
                    attempt=attempt + 1,
                    old_budget=num_predict,
                    new_budget=increased,
                    completion_tokens=response.completion_tokens,
                )
                llm_kwargs["num_predict"] = increased
                budget_increased = True
            else:
                logger.warning(
                    "LLM output validation failed",
                    attempt=attempt + 1,
                    max_attempts=1 + retries,
                    error=str(exc),
                )
        except Exception as exc:
            last_error = exc
            logger.warning(
                "LLM output validation failed",
                attempt=attempt + 1,
                max_attempts=1 + retries,
                error=str(exc),
            )

    if fallback is not None:
        logger.info("Using fallback after validation failures", model=model_cls.__name__)
        return fallback

    raise ValueError(f"LLM output validation failed after {1 + retries} attempts: {last_error}")
