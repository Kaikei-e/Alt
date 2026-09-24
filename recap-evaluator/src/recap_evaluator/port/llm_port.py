"""LLM port — abstract interface for LLM-based evaluation."""

from typing import Protocol


class GEvalResult(Protocol):
    """Protocol for a single G-Eval result."""

    @property
    def coherence(self) -> float: ...

    @property
    def consistency(self) -> float: ...

    @property
    def fluency(self) -> float: ...

    @property
    def relevance(self) -> float: ...

    @property
    def error(self) -> str | None: ...


class GEvalBatchResult(Protocol):
    """Protocol for aggregated G-Eval batch results."""

    @property
    def avg_coherence(self) -> float: ...

    @property
    def avg_consistency(self) -> float: ...

    @property
    def avg_fluency(self) -> float: ...

    @property
    def avg_relevance(self) -> float: ...

    @property
    def avg_overall(self) -> float: ...

    @property
    def success_count(self) -> int: ...


class LLMPort(Protocol):
    """Protocol for LLM-based evaluation (Ollama/G-Eval)."""

    async def evaluate_summary(self, source_articles: str, summary: str) -> GEvalResult: ...

    async def evaluate_batch(self, items: list[tuple[str, str]]) -> GEvalBatchResult: ...

    async def score_readability(self, summary: str) -> float: ...

    async def health_check(self) -> bool: ...
