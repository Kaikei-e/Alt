"""Section contract — template-driven behavioral constraints for report sections.

Design: resolve doc mandates semi-deterministic planner. Contract fields
come from ROLE_CONTRACT_DEFAULTS templates, NOT from LLM generation.
LLM only produces key/title/search_queries/section_role (PlannerSection).
PlannerNode merges template defaults after LLM output.
"""

from __future__ import annotations

from pydantic import BaseModel


class SectionContract(BaseModel):
    """Full behavioral contract for a report section."""

    key: str
    title: str
    search_queries: list[str]
    section_role: str  # "analysis" | "conclusion" | "executive_summary" | "general"
    must_include_data_types: list[str] = []
    min_citations: int = 0
    novelty_against: list[str] = []
    max_claims: int = 7
    query_facets: list[dict] = []  # Structured facets from decompose_queries()
    synthesis_only: bool = False


class PlannerSection(BaseModel):
    """LLM-generated section output — lightweight, no contract fields."""

    key: str
    title: str
    search_queries: list[str] = []
    section_role: str = "general"


class PlannerOutput(BaseModel):
    """Full LLM structured output for the planner node.

    reasoning-first field order per ADR-632.
    """

    reasoning: str
    sections: list[PlannerSection]


# Template defaults per section_role.
# novelty_against is resolved dynamically in PlannerNode post-processing.
ROLE_CONTRACT_DEFAULTS: dict[str, dict] = {
    "analysis": {
        "must_include_data_types": ["statistic", "quote"],
        "min_citations": 2,
        "novelty_against": [],
        "max_claims": 7,
        "synthesis_only": False,
    },
    "conclusion": {
        "must_include_data_types": [],
        "min_citations": 1,
        "novelty_against": [],  # Dynamically resolved to analysis keys
        "max_claims": 5,
        "synthesis_only": True,
    },
    "executive_summary": {
        "must_include_data_types": ["statistic"],
        "min_citations": 2,
        "novelty_against": [],  # Dynamically resolved to all non-ES keys
        "max_claims": 3,
        "synthesis_only": True,
    },
    "general": {
        "must_include_data_types": [],
        "min_citations": 1,
        "novelty_against": [],
        "max_claims": 7,
        "synthesis_only": False,
    },
}
