"""Planner node — generates report outline from scope.

Uses Ollama structured output (format parameter) with reasoning-first field order (ADR-632).
temperature=0, num_predict=2048 for sufficient Japanese JSON output budget.

Design (Issue 5 + resolve): LLM generates only key/title/search_queries/section_role.
Contract fields (synthesis_only, novelty_against, min_citations, etc.) come from
ROLE_CONTRACT_DEFAULTS templates, merged in post-processing.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.domain.query_facet import decompose_queries
from acolyte.domain.section_contract import ROLE_CONTRACT_DEFAULTS, PlannerOutput
from acolyte.usecase.graph.llm_parse import generate_validated

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

PLANNER_PROMPT = """You are a report planner. Given the topic, create a structured outline with 3-5 sections.
Each section must include search_queries — a list of 1-3 specific search strings to find relevant articles.
Each section must include section_role — one of "analysis", "conclusion", "executive_summary", or "general".

Role guidelines:
- "executive_summary": high-level overview of key findings
- "analysis": detailed examination of evidence and data
- "conclusion": synthesis of analysis findings — implications, priorities, recommendations (NO new facts)
- "general": any other section type

Topic: {scope}

Return JSON with "reasoning" (one sentence about your planning approach) and "sections" (array of objects with key, title, search_queries, section_role).
Keep reasoning to one sentence to save tokens for sections.

Example:
{{"reasoning": "A market analysis needs executive summary, analysis, and conclusion.", "sections": [{{"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary", "search_queries": ["AI market overview 2026"]}}, {{"key": "market_analysis", "title": "Market Analysis", "section_role": "analysis", "search_queries": ["AI market size forecast", "AI adoption enterprise"]}}, {{"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "search_queries": ["AI industry predictions"]}}]}}

Now plan sections for the given topic."""


def _infer_section_role(key: str, title: str) -> str:
    """Infer section_role from key/title when LLM doesn't provide one."""
    combined = f"{key} {title}".lower()
    if "conclusion" in combined:
        return "conclusion"
    if "executive" in combined or "summary" in combined:
        return "executive_summary"
    if "analysis" in combined:
        return "analysis"
    return "general"


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
            "section_role": "executive_summary",
            "search_queries": [f"{topic} executive summary"],
        },
        {
            "key": "analysis",
            "title": "Analysis",
            "section_role": "analysis",
            "search_queries": [f"{topic} analysis"],
        },
        {
            "key": "conclusion",
            "title": "Conclusion",
            "section_role": "conclusion",
            "search_queries": [f"{topic} conclusion"],
        },
    ]


def _enrich_with_contract(sections: list[dict], brief: dict | None = None) -> list[dict]:
    """Merge ROLE_CONTRACT_DEFAULTS into sections, resolve novelty_against, and decompose queries."""
    analysis_keys = [s["key"] for s in sections if s.get("section_role") == "analysis"]
    non_es_keys = [s["key"] for s in sections if s.get("section_role") != "executive_summary"]

    for section in sections:
        role = section.get("section_role", "general")
        defaults = ROLE_CONTRACT_DEFAULTS.get(role, ROLE_CONTRACT_DEFAULTS["general"])
        for field, default_val in defaults.items():
            if field not in section:
                # Copy lists to avoid mutating template
                section[field] = list(default_val) if isinstance(default_val, list) else default_val

        # Dynamic novelty_against resolution
        if role == "conclusion":
            section["novelty_against"] = list(analysis_keys)
        elif role == "executive_summary":
            section["novelty_against"] = list(non_es_keys)

    # Issue 6: Populate query_facets via deterministic decomposition
    brief = brief or {}
    for section in sections:
        if not section.get("synthesis_only", False):
            section["query_facets"] = [
                f.model_dump()
                for f in decompose_queries(
                    section.get("search_queries", []),
                    brief,
                    section,
                )
            ]
        else:
            section["query_facets"] = []

    return sections


class PlannerNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def __call__(self, state: ReportGenerationState) -> dict:
        brief = state.get("brief") or state.get("scope") or {}
        topic = brief.get("topic", "")
        prompt = PLANNER_PROMPT.format(scope=brief)

        fallback = PlannerOutput(
            reasoning="fallback",
            sections=[],
        )
        result = await generate_validated(
            self._llm,
            prompt,
            PlannerOutput,
            temperature=0,
            num_predict=2048,
            fallback=fallback,
        )

        if result.reasoning and result.reasoning != "fallback":
            logger.info("Planner reasoning", reasoning=result.reasoning[:200])

        outline = [s.model_dump() for s in result.sections]

        if not outline:
            outline = _default_fallback_sections(topic)

        # Ensure all sections have search_queries and section_role
        outline = _ensure_search_queries(outline, topic)
        for section in outline:
            if not section.get("section_role"):
                section["section_role"] = _infer_section_role(section.get("key", ""), section.get("title", ""))

        # Enrich with contract template defaults + query facets
        outline = _enrich_with_contract(outline, brief=brief)

        logger.info("Planner completed", section_count=len(outline))
        return {"outline": outline}
