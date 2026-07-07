"""Readability — LLM-scored quick-catch readability (1-5 scale) via Ollama.

Used for 3days Recap daily briefing context; looser than G-Eval Fluency because
it evaluates "朝刊として読みやすいか" rather than pure linguistic fluency.
"""

from typing import Protocol

import httpx
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
        except (httpx.HTTPError, ValueError, KeyError) as exc:
            # Recoverable: the LLM call itself failed (network/timeout/HTTP
            # error) or returned an unparseable response. Anything else
            # (e.g. AttributeError from a missing/broken score_readability
            # implementation) must propagate — silently defaulting to 0.0
            # there would mask a wiring bug as "always low quality"
            # (CLAUDE.md rule 8).
            logger.warning("readability evaluation failed", error=str(exc))
            return 0.0

    async def evaluate_batch(self, summaries: list[str]) -> float:
        scores: list[float] = []
        for s in summaries:
            score = await self.evaluate(s)
            if score > 0:
                scores.append(score)

        return sum(scores) / len(scores) if scores else 0.0
