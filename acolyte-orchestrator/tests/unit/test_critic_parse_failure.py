"""Unit tests for critic parse failure → revise fallback."""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.critic_node import CriticNode


class BadLLM:
    """Always returns unparseable output."""

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        return LLMResponse(text="not json at all {{{", model="fake")


@pytest.mark.asyncio
async def test_parse_failure_triggers_revise() -> None:
    """JSON parse failure should trigger revision, NOT silent acceptance."""
    node = CriticNode(BadLLM())
    state = {
        "sections": {"summary": "Some content here."},
        "brief": {"topic": "AI"},
    }
    result = await node(state)
    critique = result["critique"]
    assert critique["verdict"] == "revise"


class AcceptLLM:
    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        return LLMResponse(
            text=json.dumps({
                "reasoning": "Quality is good",
                "verdict": "accept",
                "failure_modes": [],
                "revise_sections": [],
                "feedback": {},
            }),
            model="fake",
        )


@pytest.mark.asyncio
async def test_valid_accept_passes_through() -> None:
    node = CriticNode(AcceptLLM())
    state = {
        "sections": {"summary": "Good content about AI semiconductor market."},
        "brief": {"topic": "AI semiconductor"},
    }
    result = await node(state)
    assert result["critique"]["verdict"] == "accept"
