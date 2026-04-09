"""LLM provider port — interface for text generation via news-creator."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol


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
        format: dict | None = None,
    ) -> LLMResponse: ...
