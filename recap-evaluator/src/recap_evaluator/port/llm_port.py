"""LLM port — abstract interface for LLM-based evaluation."""

from typing import Any, Protocol


class GEvalResult(Protocol):
    """Protocol for a single G-Eval result."""

    coherence: float
    consistency: float
    fluency: float
    relevance: float
    error: str | None


class LLMPort(Protocol):
    """Protocol for LLM-based evaluation (Ollama/G-Eval)."""

    async def evaluate_summary(
        self, source_articles: str, summary: str
    ) -> Any: ...

    async def evaluate_batch(
        self, items: list[tuple[str, str]]
    ) -> Any: ...

    async def health_check(self) -> bool: ...
