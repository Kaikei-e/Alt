"""Planner node — generates report outline from scope.

Uses Ollama structured output (format parameter) with reasoning-first field order (ADR-632).
temperature=0, num_predict=512 for structured output accuracy.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

# JSON schema for Ollama structured output (GBNF grammar enforcement)
_PLANNER_FORMAT = {
    "type": "object",
    "properties": {
        "reasoning": {"type": "string"},
        "sections": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "key": {"type": "string"},
                    "title": {"type": "string"},
                },
                "required": ["key", "title"],
            },
        },
    },
    "required": ["reasoning", "sections"],
}

PLANNER_PROMPT = """You are a report planner. Given the topic, create a structured outline with 3-5 sections.

Topic: {scope}

Return JSON with "reasoning" (your thinking) and "sections" (array of key/title objects).

Example:
{{"reasoning": "A market analysis needs executive summary, trends, landscape, and outlook.", "sections": [{{"key": "executive_summary", "title": "Executive Summary"}}, {{"key": "market_trends", "title": "Market Trends"}}, {{"key": "competitive_landscape", "title": "Competitive Landscape"}}, {{"key": "outlook", "title": "Outlook & Recommendations"}}]}}

Now plan sections for the given topic."""


class PlannerNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def __call__(self, state: ReportGenerationState) -> dict:
        scope = state.get("scope", {})
        prompt = PLANNER_PROMPT.format(scope=json.dumps(scope))

        response = await self._llm.generate(
            prompt,
            num_predict=512,
            temperature=0,
            format=_PLANNER_FORMAT,
        )

        try:
            parsed = json.loads(response.text)
            outline = parsed.get("sections", [])
            reasoning = parsed.get("reasoning", "")
            if reasoning:
                logger.info("Planner reasoning", reasoning=reasoning[:200])
        except json.JSONDecodeError:
            logger.warning("Planner JSON parse failed, using fallback", raw_len=len(response.text))
            outline = [
                {"key": "executive_summary", "title": "Executive Summary"},
                {"key": "analysis", "title": "Analysis"},
                {"key": "conclusion", "title": "Conclusion"},
            ]

        if not outline:
            outline = [{"key": "executive_summary", "title": "Executive Summary"}]

        logger.info("Planner completed", section_count=len(outline))
        return {"outline": outline}
