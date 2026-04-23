"""Readability — LLM-scored quick-catch readability (1-5 scale) via Ollama.

Used for 3days Recap daily briefing context; looser than G-Eval Fluency because
it evaluates "朝刊として読みやすいか" rather than pure linguistic fluency.
"""

from typing import Protocol

import structlog

logger = structlog.get_logger()


class ReadabilityLLM(Protocol):
    async def score_readability(self, summary: str) -> float: ...


class ReadabilityEvaluator:
    def __init__(self, ollama: ReadabilityLLM) -> None:
        self._ollama = ollama

    async def evaluate(self, summary: str) -> float:
        if not summary or not summary.strip():
            return 0.0
        try:
            return float(await self._ollama.score_readability(summary))
        except Exception as exc:
            logger.warning("readability evaluation failed", error=str(exc))
            return 0.0

    async def evaluate_batch(self, summaries: list[str]) -> float:
        scores: list[float] = []
        for s in summaries:
            score = await self.evaluate(s)
            if score > 0:
                scores.append(score)

        return sum(scores) / len(scores) if scores else 0.0
