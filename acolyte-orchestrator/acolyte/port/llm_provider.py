"""LLM provider port — interface for text generation via news-creator."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Any, Protocol


class LLMProviderError(Exception):
    """Base error for LLM provider operations."""

    def __init__(self, exc: Exception | str | None = None) -> None:
        detail = f": {exc}" if exc is not None else ""
        super().__init__(f"LLM provider request failed{detail}")


_MAX_RESPONSE_CHARS_IN_ERROR = 200


class LLMStatusError(LLMProviderError):
    """Raised when LLM provider returns an HTTP error status."""

    def __init__(self, status_code: int, response_text: str = "") -> None:
        self.status_code = status_code
        self.response_text = response_text
        truncated = response_text[:_MAX_RESPONSE_CHARS_IN_ERROR]
        suffix = "..." if len(response_text) > _MAX_RESPONSE_CHARS_IN_ERROR else ""
        detail = f": {truncated}{suffix}" if truncated else ""
        Exception.__init__(self, f"LLM provider request failed with status {status_code}{detail}")


class LLMTimeoutError(LLMProviderError, TimeoutError):
    """Raised when an LLM provider request times out."""

    def __init__(self, exc: Exception | str | None = None) -> None:
        detail = f": {exc}" if exc is not None else ""
        Exception.__init__(self, f"LLM provider request timed out{detail}")


class LLMMode(Enum):
    """LLM calling profile — determines default temperature, num_predict, and endpoint."""

    STRUCTURED = "structured"
    LONGFORM = "longform"


@dataclass(frozen=True)
class LLMResponse:
    text: str
    model: str
    prompt_tokens: int = 0
    completion_tokens: int = 0


class LLMProviderPort(Protocol):
    async def generate(  # noqa: PLR0913 — keyword-only LLM generation knobs, each independently optional
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        temperature: float | None = None,
        top_p: float | None = None,
        top_k: int | None = None,
        output_schema: dict[str, Any] | None = None,
        think: bool | None = None,
        mode: LLMMode | None = None,
        system_prompt: str | None = None,
    ) -> LLMResponse: ...
