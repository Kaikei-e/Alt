"""Tests for NewsCreatorHyDEGenerator — timeouts, failure modes, sanitisation."""

from __future__ import annotations

import asyncio

import pytest

from acolyte.gateway.news_creator_hyde_gw import NewsCreatorHyDEGenerator
from acolyte.port.llm_provider import LLMResponse


class _FakeLLM:
    def __init__(self, *, text: str = "", delay: float = 0.0, raise_exc: Exception | None = None) -> None:
        self.text = text
        self.delay = delay
        self.raise_exc = raise_exc
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs) -> LLMResponse:  # noqa: ANN003
        self.calls.append({"prompt": prompt, **kwargs})
        if self.delay:
            await asyncio.sleep(self.delay)
        if self.raise_exc:
            raise self.raise_exc
        return LLMResponse(text=self.text, model="stub")


@pytest.mark.asyncio
async def test_generator_returns_sanitised_output_for_en():
    passage = (
        "The 2026 AI chip market continues to expand with several new "
        "entrants pushing aggressive pricing across GPU and NPU segments. "
        "Analysts observe margin pressure in the consumer tier."
    )
    gen = NewsCreatorHyDEGenerator(_FakeLLM(text=passage))
    out = await gen.generate_hypothetical_doc("AIチップ市場 2026", "en")
    assert out is not None
    assert "2026" in out


@pytest.mark.asyncio
async def test_empty_topic_returns_none_without_llm_call():
    llm = _FakeLLM(text="ignored")
    gen = NewsCreatorHyDEGenerator(llm)
    assert await gen.generate_hypothetical_doc("   ", "en") is None
    assert llm.calls == []


@pytest.mark.asyncio
async def test_unsupported_target_lang_returns_none_without_llm_call():
    llm = _FakeLLM(text="ignored")
    gen = NewsCreatorHyDEGenerator(llm)
    assert await gen.generate_hypothetical_doc("topic", "fr") is None
    assert llm.calls == []


@pytest.mark.asyncio
async def test_timeout_returns_none():
    gen = NewsCreatorHyDEGenerator(_FakeLLM(delay=0.5), timeout_s=0.05)
    out = await gen.generate_hypothetical_doc("AIチップ市場 2026", "en")
    assert out is None


@pytest.mark.asyncio
async def test_llm_exception_returns_none():
    gen = NewsCreatorHyDEGenerator(_FakeLLM(raise_exc=RuntimeError("boom")))
    out = await gen.generate_hypothetical_doc("AIチップ市場 2026", "en")
    assert out is None


@pytest.mark.asyncio
async def test_sanitiser_reject_returns_none():
    # Japanese-heavy output for en target -> sanitiser rejects.
    gen = NewsCreatorHyDEGenerator(_FakeLLM(text="本文は日本語で書かれています。これは拒否されます。"))
    out = await gen.generate_hypothetical_doc("AIチップ市場 2026", "en")
    assert out is None
