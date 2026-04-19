"""LLM provider port — interface for text generation via news-creator."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Protocol


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
    async def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        temperature: float | None = None,
        top_p: float | None = None,
        top_k: int | None = None,
        format: dict | None = None,
        think: bool | None = None,
        mode: LLMMode | None = None,
        system_prompt: str | None = None,
    ) -> LLMResponse: ...
