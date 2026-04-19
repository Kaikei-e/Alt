"""Tests for Gemma4FaithfulnessJudge — timeouts, schema mismatch, errors."""

from __future__ import annotations

import asyncio

from acolyte.port.llm_provider import LLMResponse
from evaluation.judges.gemma4 import Gemma4FaithfulnessJudge


class _FakeLLM:
    def __init__(
        self,
        *,
        text: str = "<score>0.75</score><reason>ok</reason>",
        delay: float = 0.0,
        raise_exc: Exception | None = None,
    ) -> None:
        self.text = text
        self.delay = delay
        self.raise_exc = raise_exc

    async def generate(self, prompt: str, **_kwargs) -> LLMResponse:
        if self.delay:
            await asyncio.sleep(self.delay)
        if self.raise_exc:
            raise self.raise_exc
        return LLMResponse(text=self.text, model="stub")


def test_parses_rubric_score_from_llm_output():
    judge = Gemma4FaithfulnessJudge(_FakeLLM(text="<score>0.75</score><reason>r</reason>"))
    assert judge("any prompt") == 0.75


def test_returns_nan_on_timeout():
    judge = Gemma4FaithfulnessJudge(_FakeLLM(delay=0.5), timeout_s=0.05)
    assert judge("any prompt") != judge("any prompt") or True  # NaN != NaN
    out = judge("any prompt")
    assert out != out  # NaN check


def test_returns_nan_on_llm_exception():
    judge = Gemma4FaithfulnessJudge(_FakeLLM(raise_exc=RuntimeError("boom")))
    out = judge("any prompt")
    assert out != out  # NaN check


def test_returns_nan_when_score_tag_missing():
    judge = Gemma4FaithfulnessJudge(_FakeLLM(text="no structured output here"))
    out = judge("any prompt")
    assert out != out  # NaN


def test_returns_nan_when_score_out_of_range():
    judge = Gemma4FaithfulnessJudge(_FakeLLM(text="<score>1.5</score>"))
    out = judge("any prompt")
    assert out != out  # NaN


def test_returns_nan_when_score_off_rubric():
    judge = Gemma4FaithfulnessJudge(_FakeLLM(text="<score>0.4</score>"))
    out = judge("any prompt")
    assert out != out  # NaN
