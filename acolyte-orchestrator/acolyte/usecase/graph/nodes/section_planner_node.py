"""Section planner node — creates ClaimPlan per section from extracted facts.

Sits between extractor and writer. Each section gets a structured claim plan
that the writer must follow, ensuring fact-first generation.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.domain.claim import ClaimPlannerOutput
from acolyte.port.llm_provider import LLMMode
from acolyte.usecase.graph.xml_parse import generate_xml_validated, normalize_section_plan_output

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

SECTION_PLANNER_PROMPT = """You are a claim planner for report section "{title}".
Topic: {topic}

Available facts from evidence:
{facts_block}

Plan up to {max_claims} claims for this section. Each claim must be grounded in evidence.
{contract_instructions}
Wrap your response in <section_plan> tags:
<section_plan>
  <reasoning>your claim planning strategy</reasoning>
  <claim>
    <text>specific claim grounded in evidence</text>
    <claim_type>factual</claim_type>
    <evidence_id>src_1</evidence_id>
    <supporting_quote>verbatim quote from evidence</supporting_quote>
    <numeric_fact>42%</numeric_fact>
    <must_cite>true</must_cite>
  </claim>
</section_plan>

Output ONLY the <section_plan> block."""

CONCLUSION_PLANNER_PROMPT = """You are a synthesis planner for the conclusion section "{title}".
Topic: {topic}

Analysis claims to synthesize:
{claims_block}

Create 3-5 synthesis claims for the conclusion. Rules:
- claim_type MUST be "synthesis" for every claim
- Each claim must integrate 2+ analysis claims — do NOT restate any single analysis claim
- Focus on: implications, risks, priorities, and recommended actions

Wrap your response in <section_plan> tags:
<section_plan>
  <reasoning>your synthesis strategy</reasoning>
  <claim>
    <text>synthesized conclusion claim</text>
    <claim_type>synthesis</claim_type>
    <evidence_id>src_1</evidence_id>
    <must_cite>true</must_cite>
  </claim>
</section_plan>

Output ONLY the <section_plan> block."""

ES_PLANNER_PROMPT = """You are a summary planner for the executive summary "{title}".
Topic: {topic}

Key findings from all report sections:
{claims_block}

Select the 1-2 strongest claims from each section. Rules:
- claim_type MUST be "synthesis" for every claim
- Prefer claims with numeric_facts when available
- Must include at least one claim with numeric data if available

Wrap your response in <section_plan> tags:
<section_plan>
  <reasoning>your selection strategy</reasoning>
  <claim>
    <text>key finding summary</text>
    <claim_type>synthesis</claim_type>
    <evidence_id>src_1</evidence_id>
    <must_cite>true</must_cite>
  </claim>
</section_plan>

Output ONLY the <section_plan> block."""


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
        quote = fact.get("quote", fact.get("verbatim_quote", ""))
        if quote:
            line += f'\n   Quote: "{quote}"'
        line += f"\n   Source: {fact.get('source_id', '')}"
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


def _rank_facts_for_synthesis(facts: list[dict]) -> list[dict]:
    """Rank facts by: numeric_facts presence → source diversity → confidence."""
    seen_sources: set[str] = set()
    scored: list[tuple[float, dict]] = []

    for fact in facts:
        score = 0.0
        # Prefer facts with numeric data
        if fact.get("numeric_facts") or fact.get("data_type") == "statistic":
            score += 10.0
        # Prefer diverse sources
        src = fact.get("source_id", "")
        if src and src not in seen_sources:
            score += 5.0
            seen_sources.add(src)
        # Confidence
        score += fact.get("confidence", 0.0)
        scored.append((score, fact))

    scored.sort(key=lambda x: x[0], reverse=True)
    return [f for _, f in scored]


def _topic_overview_claim(topic: str, *, prefix: str) -> dict:
    """Create a minimal topic-based claim as last-resort fallback."""
    return {
        "claim_id": f"{prefix}-topic-1",
        "claim": f"{topic} の概要",
        "claim_type": "synthesis",
        "evidence_ids": [],
        "supporting_quotes": [],
        "numeric_facts": [],
        "novelty_against": [],
        "must_cite": False,
    }


def _fact_to_synthesis_claim(fact: dict, *, claim_id: str = "") -> dict:
    """Convert an extracted fact into a synthesis claim dict."""
    quote = fact.get("quote", fact.get("verbatim_quote", ""))
    return {
        "claim_id": claim_id,
        "claim": fact.get("claim", quote),
        "claim_type": "synthesis",
        "evidence_ids": [fact["source_id"]] if fact.get("source_id") else [],
        "supporting_quotes": [quote] if quote else [],
        "numeric_facts": fact.get("numeric_facts", []),
        "novelty_against": [],
        "must_cite": True,
    }


def _claims_to_synthesis(claims: list[dict], *, max_claims: int = 5, prefix: str = "") -> list[dict]:
    """Convert existing claims into synthesis claims for conclusion/ES."""
    result: list[dict] = []
    seen_sources: set[str] = set()
    # Prefer claims with numeric_facts and diverse sources
    with_numeric = [c for c in claims if c.get("numeric_facts")]
    without_numeric = [c for c in claims if not c.get("numeric_facts")]

    for claim in with_numeric + without_numeric:
        if len(result) >= max_claims:
            break
        eids = set(claim.get("evidence_ids", []))
        # Prefer diverse sources
        is_diverse = bool(eids - seen_sources)
        if not is_diverse and len(result) >= 2:
            continue
        seen_sources.update(eids)
        result.append(
            {
                "claim_id": f"{prefix}-synth-{len(result) + 1}" if prefix else claim.get("claim_id", ""),
                "claim": claim.get("claim", ""),
                "claim_type": "synthesis",
                "evidence_ids": claim.get("evidence_ids", []),
                "supporting_quotes": claim.get("supporting_quotes", []),
                "numeric_facts": claim.get("numeric_facts", []),
                "novelty_against": claim.get("novelty_against", []),
                "must_cite": True,
            }
        )
    return result


def _deterministic_conclusion_claims(
    analysis_claims: list[dict],
    extracted_facts: list[dict],
    *,
    max_claims: int = 5,
    topic: str = "",
) -> list[dict]:
    """Deterministic fallback for conclusion: synthesis from analysis claims or facts.

    Priority: analysis_claims → extracted_facts → topic overview.
    Ranking: numeric_facts → source diversity → confidence.
    """
    if analysis_claims:
        return _claims_to_synthesis(analysis_claims, max_claims=max_claims, prefix="conclusion")

    if extracted_facts:
        ranked = _rank_facts_for_synthesis(extracted_facts)
        return [
            _fact_to_synthesis_claim(f, claim_id=f"conclusion-synth-{i + 1}") for i, f in enumerate(ranked[:max_claims])
        ]

    # Last-resort: topic-based claim to prevent empty body
    if topic:
        return [_topic_overview_claim(topic, prefix="conclusion")]
    return []


def _deterministic_es_claims(
    all_claims: dict[str, list[dict]],
    extracted_facts: list[dict],
    *,
    max_claims: int = 3,
    topic: str = "",
) -> list[dict]:
    """Deterministic fallback for ES: pick top claims from all sections or facts.

    Priority: existing claim_plans → extracted_facts → topic overview.
    Ranking: numeric_facts → source diversity.
    """
    # Flatten all claims from all sections
    flat_claims: list[dict] = []
    for claims in all_claims.values():
        flat_claims.extend(claims)

    if flat_claims:
        return _claims_to_synthesis(flat_claims, max_claims=max_claims, prefix="es")

    if extracted_facts:
        ranked = _rank_facts_for_synthesis(extracted_facts)
        return [_fact_to_synthesis_claim(f, claim_id=f"es-synth-{i + 1}") for i, f in enumerate(ranked[:max_claims])]

    # Last-resort: topic-based claim to prevent empty body
    if topic:
        return [_topic_overview_claim(topic, prefix="es")]
    return []


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
                    # Deterministic fallback: use claim_plans or extracted_facts
                    fallback_claims = _deterministic_es_claims(claim_plans, extracted_facts, topic=topic)
                    if fallback_claims:
                        logger.info("ES using deterministic fallback", claim_count=len(fallback_claims))
                        claim_plans[key] = fallback_claims
                    else:
                        logger.warning("No claims or facts for ES", section_key=key)
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
                    # Deterministic fallback: use extracted_facts
                    fallback_claims = _deterministic_conclusion_claims([], extracted_facts, topic=topic)
                    if fallback_claims:
                        logger.info("Conclusion using deterministic fallback", claim_count=len(fallback_claims))
                        claim_plans[key] = fallback_claims
                    else:
                        logger.warning("No analysis claims or facts for conclusion", section_key=key)
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
            result = await generate_xml_validated(
                self._llm,
                prompt,
                ClaimPlannerOutput,
                root_tag="section_plan",
                normalizer=normalize_section_plan_output,
                temperature=0,
                num_predict=2048,
                fallback=fallback,
                mode=LLMMode.STRUCTURED,
            )

            claim_dicts = [c.model_dump() for c in result.claims]
            for i, cd in enumerate(claim_dicts, 1):
                cd["claim_id"] = f"{key}-{i}"

            # Post-LLM fallback: if LLM returned empty claims for conclusion/ES
            if not claim_dicts and section_role in ("conclusion", "executive_summary"):
                if section_role == "conclusion":
                    analysis_claims = _collect_analysis_claims(claim_plans, outline)
                    claim_dicts = _deterministic_conclusion_claims(analysis_claims, extracted_facts, topic=topic)
                else:
                    claim_dicts = _deterministic_es_claims(claim_plans, extracted_facts, topic=topic)
                if claim_dicts:
                    logger.info("Post-LLM deterministic fallback", section_key=key, claim_count=len(claim_dicts))

            claim_plans[key] = claim_dicts

        logger.info(
            "Section planner completed",
            sections_planned=len(claim_plans),
            total_claims=sum(len(v) for v in claim_plans.values()),
        )
        return {"claim_plans": claim_plans}
