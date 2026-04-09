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
                    "search_queries": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                },
                "required": ["key", "title", "search_queries"],
            },
        },
    },
    "required": ["reasoning", "sections"],
}

PLANNER_PROMPT = """You are a report planner. Given the topic, create a structured outline with 3-5 sections.
Each section must include search_queries — a list of 1-3 specific search strings to find relevant articles.

Topic: {scope}

Return JSON with "reasoning" (your thinking) and "sections" (array of objects with key, title, search_queries).

Example:
{{"reasoning": "A market analysis needs executive summary, trends, landscape, and outlook.", "sections": [{{"key": "executive_summary", "title": "Executive Summary", "search_queries": ["AI market overview 2026"]}}, {{"key": "market_trends", "title": "Market Trends", "search_queries": ["AI market size forecast", "AI adoption enterprise"]}}, {{"key": "competitive_landscape", "title": "Competitive Landscape", "search_queries": ["AI company comparison", "AI startup funding"]}}, {{"key": "outlook", "title": "Outlook & Recommendations", "search_queries": ["AI industry predictions"]}}]}}

Now plan sections for the given topic."""


def _ensure_search_queries(sections: list[dict], topic: str) -> list[dict]:
    """Ensure every section has at least one search_query. Add default if missing."""
    for section in sections:
        queries = section.get("search_queries")
        if not queries:
            title = section.get("title", section.get("key", ""))
            section["search_queries"] = [f"{topic} {title}"]
    return sections


def _default_fallback_sections(topic: str) -> list[dict]:
    """Generate fallback sections with topic-based search queries."""
    return [
        {
            "key": "executive_summary",
            "title": "Executive Summary",
            "search_queries": [f"{topic} executive summary"],
        },
        {
            "key": "analysis",
            "title": "Analysis",
            "search_queries": [f"{topic} analysis"],
        },
        {
            "key": "conclusion",
            "title": "Conclusion",
            "search_queries": [f"{topic} conclusion"],
        },
    ]


class PlannerNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def __call__(self, state: ReportGenerationState) -> dict:
        brief = state.get("brief") or state.get("scope") or {}
        topic = brief.get("topic", "")
        prompt = PLANNER_PROMPT.format(scope=json.dumps(brief))

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
            outline = _default_fallback_sections(topic)

        if not outline:
            outline = [
                {
                    "key": "executive_summary",
                    "title": "Executive Summary",
                    "search_queries": [f"{topic} executive summary"],
                },
            ]

        # Ensure all sections have search_queries
        outline = _ensure_search_queries(outline, topic)

        logger.info("Planner completed", section_count=len(outline))
        return {"outline": outline}
