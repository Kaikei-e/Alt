"""Planner node — semi-deterministic report outline from skeleton + query expansion.

Design (Issue 5 + resolve + quality hotfix):
  - Section structure is fixed per report_type (REPORT_TYPE_SKELETONS)
  - LLM generates only search_queries per skeleton section (query expansion)
  - Contract fields come from ROLE_CONTRACT_DEFAULTS templates
  - num_predict=1024 (skeleton only needs short query output)
  - On LLM failure, skeleton + topic-based queries is the intended fallback
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.domain.query_facet import decompose_queries
from acolyte.domain.section_contract import ROLE_CONTRACT_DEFAULTS, QueryExpansionOutput
from acolyte.port.llm_provider import LLMMode
from acolyte.usecase.graph.xml_parse import generate_xml_validated, normalize_plan_output

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

PLANNER_PROMPT = """You are a report query planner. Given the topic and fixed section structure, generate specific search queries for each section.

Topic: {scope}

Sections (fixed structure):
{skeleton_block}

For each section, generate 1-3 specific search queries to find relevant articles.
Wrap your response in <plan> tags:

<plan>
  <reasoning>one sentence explaining your query strategy</reasoning>
  <section>
    <key>executive_summary</key>
    <query>AI chip market overview 2026</query>
  </section>
  <section>
    <key>analysis</key>
    <query>NVIDIA Blackwell GPU</query>
    <query>AMD MI400 series</query>
  </section>
</plan>

Output ONLY the <plan> block. Focus on diverse, specific search queries."""

# Default skeleton for unknown report_types (no pre-baked queries).
DEFAULT_SKELETON: list[dict] = [
    {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
    {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
    {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
]

# Fixed section skeletons per report_type. LLM only expands queries.
REPORT_TYPE_SKELETONS: dict[str, list[dict]] = {
    "market_analysis": [
        {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
        {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
        {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
    ],
    "weekly_briefing": [
        {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
        {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
        {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
    ],
    "trend_report": [
        {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
        {"key": "analysis", "title": "Trend Analysis", "section_role": "analysis"},
        {"key": "conclusion", "title": "Outlook", "section_role": "conclusion"},
    ],
}


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


def _get_skeleton(brief: dict) -> list[dict]:
    """Get fixed section skeleton for report_type, or default."""
    report_type = brief.get("report_type", "")
    skeleton = REPORT_TYPE_SKELETONS.get(report_type)
    if skeleton:
        return [dict(s) for s in skeleton]  # copy to avoid mutation
    return [dict(s) for s in DEFAULT_SKELETON]


def _format_skeleton_block(skeleton: list[dict]) -> str:
    """Format skeleton sections for the prompt."""
    lines = []
    for s in skeleton:
        lines.append(f"- {s['key']} ({s.get('section_role', 'general')}): {s.get('title', s['key'])}")
    return "\n".join(lines)


class PlannerNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def __call__(self, state: ReportGenerationState) -> dict:
        brief = state.get("brief") or state.get("scope") or {}
        topic = brief.get("topic", "")
        skeleton = _get_skeleton(brief)

        prompt = PLANNER_PROMPT.format(
            scope=topic,
            skeleton_block=_format_skeleton_block(skeleton),
        )

        fallback = QueryExpansionOutput(reasoning="fallback", queries={})
        result = await generate_xml_validated(
            self._llm,
            prompt,
            QueryExpansionOutput,
            root_tag="plan",
            normalizer=normalize_plan_output,
            temperature=0,
            num_predict=1024,
            fallback=fallback,
            mode=LLMMode.STRUCTURED,
        )

        if result.reasoning and result.reasoning != "fallback":
            logger.info("Planner reasoning", reasoning=result.reasoning[:200])
        else:
            logger.info(
                "Planner skeleton fallback activated",
                report_type=brief.get("report_type", ""),
            )

        # Merge LLM-generated queries into skeleton
        queries_by_key = result.queries or {}
        outline: list[dict] = []
        for skel in skeleton:
            section = dict(skel)
            section["search_queries"] = queries_by_key.get(skel["key"], [])
            outline.append(section)

        # Ensure all sections have search_queries and section_role
        outline = _ensure_search_queries(outline, topic)
        for section in outline:
            if not section.get("section_role"):
                section["section_role"] = _infer_section_role(section.get("key", ""), section.get("title", ""))

        # Enrich with contract template defaults + query facets
        outline = _enrich_with_contract(outline, brief=brief)

        logger.info("Planner completed", section_count=len(outline))
        return {"outline": outline}
