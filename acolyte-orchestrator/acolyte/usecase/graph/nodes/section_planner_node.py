"""Section planner node — creates ClaimPlan per section from extracted facts.

Sits between extractor and writer. Each section gets a structured claim plan
that the writer must follow, ensuring fact-first generation.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.domain.claim import ClaimPlannerOutput
from acolyte.usecase.graph.llm_parse import generate_validated

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

SECTION_PLANNER_PROMPT = """You are a claim planner for report section "{title}".
Topic: {topic}

Available facts from evidence:
{facts_block}

Plan up to {max_claims} claims for this section. Each claim must:
- Be grounded in at least one evidence fact listed above
- Include the evidence_ids and supporting_quotes from the facts
- Specify claim_type: "factual", "statistical", "comparative", or "synthesis"
- Include numeric_facts if the evidence contains numbers or statistics
- Set novelty_against to list section keys whose claims this claim must NOT repeat
{contract_instructions}
Return JSON with "reasoning" (your thinking) and "claims" array."""

CONCLUSION_PLANNER_PROMPT = """You are a synthesis planner for the conclusion section "{title}".
Topic: {topic}

Analysis claims to synthesize:
{claims_block}

Create 3-5 synthesis claims for the conclusion. Rules:
- claim_type MUST be "synthesis" for every claim
- Each claim must integrate 2 or more analysis claims — do NOT restate any single analysis claim
- Do NOT introduce new facts not present in the analysis claims
- Focus on: implications, risks, priorities, and recommended actions
- Set evidence_ids to the evidence from the analysis claims you synthesize
- Set novelty_against to ["analysis"] for all claims

Return JSON with "reasoning" (your thinking) and "claims" array."""

ES_PLANNER_PROMPT = """You are a summary planner for the executive summary "{title}".
Topic: {topic}

Key findings from all report sections:
{claims_block}

Select the 1-2 strongest claims from each section for the executive summary. Rules:
- claim_type MUST be "synthesis" for every claim
- Do NOT introduce new facts — only summarize existing section findings
- Prefer claims with numeric_facts when available
- Must include at least one claim with numeric data if any source claim has numeric data
- Set evidence_ids to the evidence from the source claims you summarize
- Set novelty_against to all source section keys

Return JSON with "reasoning" (your thinking) and "claims" array."""


def _build_contract_instructions(section: dict) -> str:
    """Build contract-driven instruction lines for section planner prompt."""
    lines: list[str] = []

    novelty_against = section.get("novelty_against", [])
    if novelty_against:
        keys_str = ", ".join(novelty_against)
        lines.append(f"- Do NOT repeat claims from sections: {keys_str}")

    must_include = section.get("must_include_data_types", [])
    if must_include:
        types_str = ", ".join(must_include)
        lines.append(f"- Include claims covering these data types: {types_str}")

    if section.get("synthesis_only"):
        lines.append("- claim_type MUST be synthesis for every claim")

    return "\n".join(lines)


def _format_facts_block(facts: list[dict]) -> str:
    """Format filtered facts into a readable block for the planner prompt."""
    if not facts:
        return "No facts available."
    lines = []
    for i, fact in enumerate(facts, 1):
        line = f"{i}. [{fact.get('data_type', 'quote')}] {fact.get('claim', '')}"
        quote = fact.get("verbatim_quote", "")
        if quote:
            line += f'\n   Quote: "{quote}"'
        line += f"\n   Source: {fact.get('source_id', '')} — {fact.get('source_title', '')}"
        conf = fact.get("confidence", 0)
        line += f"\n   Confidence: {conf}"
        lines.append(line)
    return "\n".join(lines)


def _filter_facts_for_section(
    all_facts: list[dict],
    section_evidence_ids: set[str],
) -> list[dict]:
    """Keep only facts whose source_id is in the section's curated evidence."""
    return [f for f in all_facts if f.get("source_id") in section_evidence_ids]


def _format_analysis_claims(claims: list[dict]) -> str:
    """Format analysis claims as input for the conclusion planner."""
    if not claims:
        return "No analysis claims available."
    lines = []
    for i, claim in enumerate(claims, 1):
        line = f"{i}. [{claim.get('claim_type', 'factual')}] {claim.get('claim', '')}"
        for q in claim.get("supporting_quotes", []):
            line += f'\n   Quote: "{q}"'
        eids = claim.get("evidence_ids", [])
        if eids:
            line += f"\n   Evidence: {', '.join(eids)}"
        lines.append(line)
    return "\n".join(lines)


def _collect_analysis_claims(
    claim_plans: dict[str, list[dict]],
    outline: list[dict],
) -> list[dict]:
    """Collect all claims from sections with section_role='analysis'."""
    analysis_keys = {s.get("key", "") for s in outline if s.get("section_role") == "analysis"}
    claims: list[dict] = []
    for key in analysis_keys:
        claims.extend(claim_plans.get(key, []))
    return claims


def _collect_all_accepted_claims(
    claim_plans: dict[str, list[dict]],
    outline: list[dict],
) -> list[dict]:
    """Collect claims from ALL non-ES sections for executive summary input."""
    non_es_keys = {s.get("key", "") for s in outline if s.get("section_role") != "executive_summary"}
    claims: list[dict] = []
    for key in non_es_keys:
        claims.extend(claim_plans.get(key, []))
    return claims


class SectionPlannerNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def __call__(self, state: ReportGenerationState) -> dict:
        outline = state.get("outline", [])
        curated_by_section = state.get("curated_by_section", {})
        extracted_facts = state.get("extracted_facts", [])
        brief = state.get("brief") or state.get("scope") or {}
        topic = brief.get("topic", "")

        claim_plans: dict[str, list[dict]] = {}

        # Process ES last so it can use accepted claims from all other sections
        non_es = [s for s in outline if s.get("section_role") != "executive_summary"]
        es_sections = [s for s in outline if s.get("section_role") == "executive_summary"]

        for section in non_es + es_sections:
            key = section.get("key", "")
            title = section.get("title", key)
            section_role = section.get("section_role", "general")

            if section_role == "executive_summary":
                # ES uses accepted claims from ALL other sections
                all_claims = _collect_all_accepted_claims(claim_plans, outline)
                if not all_claims:
                    logger.warning("No accepted claims for ES, using empty plan", section_key=key)
                    claim_plans[key] = []
                    continue

                claims_block = _format_analysis_claims(all_claims)
                prompt = ES_PLANNER_PROMPT.format(
                    title=title,
                    topic=topic,
                    claims_block=claims_block,
                )
            elif section_role == "conclusion":
                # Conclusion uses analysis claims as input, not raw facts
                analysis_claims = _collect_analysis_claims(claim_plans, outline)
                if not analysis_claims:
                    logger.warning("No analysis claims for conclusion, using empty plan", section_key=key)
                    claim_plans[key] = []
                    continue

                claims_block = _format_analysis_claims(analysis_claims)
                prompt = CONCLUSION_PLANNER_PROMPT.format(
                    title=title,
                    topic=topic,
                    claims_block=claims_block,
                )
            else:
                # Standard path: use extracted facts filtered by section evidence
                section_evidence = curated_by_section.get(key, [])
                evidence_ids = {item.get("id", "") for item in section_evidence}
                section_facts = _filter_facts_for_section(extracted_facts, evidence_ids)

                if not section_facts:
                    logger.warning("No facts for section, using empty claim plan", section_key=key)
                    claim_plans[key] = []
                    continue

                max_claims = section.get("max_claims", 7)
                contract_instructions = _build_contract_instructions(section)

                facts_block = _format_facts_block(section_facts)
                prompt = SECTION_PLANNER_PROMPT.format(
                    title=title,
                    topic=topic,
                    facts_block=facts_block,
                    max_claims=max_claims,
                    contract_instructions=contract_instructions,
                )

            fallback = ClaimPlannerOutput(reasoning="fallback", claims=[])
            result = await generate_validated(
                self._llm,
                prompt,
                ClaimPlannerOutput,
                temperature=0,
                num_predict=2048,
                fallback=fallback,
            )

            claim_dicts = [c.model_dump() for c in result.claims]
            for i, cd in enumerate(claim_dicts, 1):
                cd["claim_id"] = f"{key}-{i}"
            claim_plans[key] = claim_dicts

        logger.info(
            "Section planner completed",
            sections_planned=len(claim_plans),
            total_claims=sum(len(v) for v in claim_plans.values()),
        )
        return {"claim_plans": claim_plans}
