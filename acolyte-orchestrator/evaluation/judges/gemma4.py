"""Gemma4-backed faithfulness judge.

Wraps an ``LLMProviderPort`` and enforces the rubric-aligned output
schema. Returns ``None`` on timeout, schema mismatch, or out-of-range
scores — those failures are propagated up the metrics pipeline so a
degraded judge never produces a falsely optimistic ``faithfulness_mean``.
"""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING

import structlog

from evaluation.judges.prompt import parse_judge_output

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)


class Gemma4FaithfulnessJudge:
    """Synchronous callable wrapping an async LLM generate call.

    ``__call__`` runs the coroutine on an event loop via ``asyncio.run`` —
    the evaluation harness is sync by design, so the judge is the only
    place that has to bridge. Timeouts surface as ``None`` rather than
    exceptions because ``evaluation.metrics.faithfulness`` treats a
    non-numeric return as "missing score" rather than pipeline failure.
    """

    def __init__(
        self,
        llm: LLMProviderPort,
        *,
        timeout_s: float = 30.0,
        num_predict: int = 96,
        temperature: float = 0.0,
    ) -> None:
        self._llm = llm
        self._timeout_s = timeout_s
        self._num_predict = num_predict
        self._temperature = temperature

    def __call__(self, prompt: str) -> float:
        try:
            score = asyncio.run(self._ask(prompt))
        except Exception as exc:  # noqa: BLE001 - never let judge break run_eval
            logger.warning("judge: llm call failed", error=str(exc))
            return float("nan")
        if score is None:
            return float("nan")
        return score

    async def _ask(self, prompt: str) -> float | None:
        response = await asyncio.wait_for(
            self._llm.generate(
                prompt,
                num_predict=self._num_predict,
                temperature=self._temperature,
            ),
            timeout=self._timeout_s,
        )
        return parse_judge_output(response.text)
