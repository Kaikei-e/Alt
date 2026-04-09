"""ClaimPlan — structured claim planning for fact-first report generation.

Each section gets a ClaimPlan before writing. The writer generates text
only from planned claims, not from raw evidence.
"""

from __future__ import annotations

from pydantic import BaseModel


class PlannedClaim(BaseModel):
    """Single claim to be made in a section."""

    claim_id: str = ""  # assigned by SectionPlannerNode as "{section_key}-{N}"
    claim: str
    claim_type: str  # factual | statistical | comparative | synthesis
    evidence_ids: list[str]
    supporting_quotes: list[str]
    numeric_facts: list[str] = []
    novelty_against: list[str] = []
    must_cite: bool = True


class SectionCitation(BaseModel):
    """Structured citation linking a claim to its evidence source."""

    claim_id: str
    source_id: str
    source_type: str = "article"  # "article" | "recap"
    quote: str = ""
    offset_start: int = -1  # position in section body, -1 = not mapped
    offset_end: int = -1


class SectionClaimPlan(BaseModel):
    """Claim plan for one section."""

    section_key: str
    claims: list[PlannedClaim]


class ClaimPlannerOutput(BaseModel):
    """Full LLM structured output for section claim planning."""

    reasoning: str
    claims: list[PlannedClaim]
