"""Critic node — evaluates generated sections and decides if revision is needed.

Uses Ollama structured output with temperature=0 for stable verdict (ADR-632).
"""

import json
from typing import TYPE_CHECKING

import structlog

from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)

MAX_REVISIONS = 2

_CRITIC_FORMAT = {
    "type": "object",
    "properties": {
        "reasoning": {"type": "string"},
        "verdict": {"type": "string", "enum": ["accept", "revise"]},
        "revise_sections": {"type": "array", "items": {"type": "string"}},
        "feedback": {"type": "object"},
    },
    "required": ["reasoning", "verdict", "revise_sections", "feedback"],
}

CRITIC_PROMPT = """You are a report quality critic. Evaluate these report sections:

{sections}

Return JSON with:
- "reasoning": your evaluation thinking
- "verdict": "accept" if quality is good enough, "revise" if sections need improvement
- "revise_sections": list of section keys needing revision (empty if accept)
- "feedback": object mapping section_key to specific feedback

If the sections are reasonably informative and well-structured, verdict should be "accept"."""


def should_revise(state: ReportGenerationState) -> str:
    """Conditional edge: should the writer revise or should we finalize?"""
    critique = state.get("critique")
    revision_count = state.get("revision_count", 0)

    if critique is None:
        return "accept"
    if revision_count >= MAX_REVISIONS:
        logger.info("Max revisions reached, accepting", revision_count=revision_count)
        return "accept"
    if critique.get("verdict") == "revise":
        return "revise"
    return "accept"


class CriticNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def __call__(self, state: ReportGenerationState) -> dict:
        sections = state.get("sections", {})
        sections_text = "\n\n".join(f"## {k}\n{v[:500]}" for k, v in sections.items())

        prompt = CRITIC_PROMPT.format(sections=sections_text)
        response = await self._llm.generate(
            prompt,
            num_predict=512,
            temperature=0,
            format=_CRITIC_FORMAT,
        )

        try:
            critique = json.loads(response.text)
        except json.JSONDecodeError:
            critique = {"verdict": "accept", "revise_sections": [], "feedback": {}}

        logger.info("Critic completed", verdict=critique.get("verdict"))
        return {"critique": critique}
