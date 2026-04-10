"""Critic node — GroUSE-based failure mode detection.

Multi-check evaluator:
- FM2 (meta-statements): heuristic, no LLM
- FM3 (incomplete info): heuristic, no LLM
- FM1 (relevancy) + FM7 (unsupported claims): LLM structured output

Parse failure → revise (never silent accept).
"""

import json
import re
from typing import TYPE_CHECKING

import structlog

from acolyte.domain.critic_taxonomy import FailureMode, FailureModeDetection
from acolyte.port.llm_provider import LLMMode
from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)

MAX_REVISIONS = 3

META_PATTERNS = [
    "情報が不足",
    "トピックが明示されて",
    "一般的な知識",
    "データを提供してください",
    "具体的な情報がありません",
    "I don't have",
    "As an AI",
    "I cannot provide",
    "As a language model",
]

MIN_SECTION_LENGTH = 100  # legacy default

_MIN_SECTION_LENGTH_BY_ROLE = {
    "analysis": 200,
    "conclusion": 100,
    "executive_summary": 80,
    "general": 100,
}

_CRITIC_FORMAT = {
    "type": "object",
    "properties": {
        "reasoning": {"type": "string"},
        "verdict": {"type": "string", "enum": ["accept", "revise"]},
        "failure_modes": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "mode": {"type": "string"},
                    "section": {"type": "string"},
                    "description": {"type": "string"},
                },
            },
        },
        "revise_sections": {"type": "array", "items": {"type": "string"}},
        "feedback": {"type": "object"},
    },
    "required": ["reasoning", "verdict", "revise_sections", "feedback"],
}

CRITIC_PROMPT = """You are a strict report quality critic. Evaluate these report sections for the topic: {topic}

{sections}

Check for:
1. Title/scope relevancy — does each section's content match its title and the overall topic?
2. Unsupported claims — are there claims not backed by the referenced evidence?
3. Overall quality — is the report informative, well-structured, and non-repetitive?

Return JSON with:
- "reasoning": your evaluation
- "verdict": "accept" if quality passes ALL checks, "revise" if ANY section needs improvement
- "failure_modes": array of detected issues [{{"mode": "FM1|FM7", "section": "key", "description": "..."}}]
- "revise_sections": list of section keys needing revision
- "feedback": object mapping section_key to specific revision instructions

Be STRICT. Reject reports with off-topic content, meta-commentary, or unsupported claims."""


def detect_meta_statements(sections: dict[str, str]) -> list[FailureModeDetection]:
    """FM2: Detect meta-statements using regex heuristics (no LLM)."""
    detections: list[FailureModeDetection] = []
    for key, body in sections.items():
        for pattern in META_PATTERNS:
            if re.search(re.escape(pattern), body, re.IGNORECASE):
                detections.append(
                    FailureModeDetection(
                        mode=FailureMode.FM2_FAILURE_TO_REFRAIN,
                        severity="blocking",
                        section_key=key,
                        description=f"Meta-statement detected: '{pattern}'",
                        suggested_fix=f"Remove meta-commentary from section '{key}' and replace with substantive content.",
                    )
                )
                break  # one detection per section is enough
    return detections


def detect_incomplete_sections(
    sections: dict[str, str],
    outline: list[dict],
) -> list[FailureModeDetection]:
    """FM3: Detect sections that are too short or missing. Per-role thresholds."""
    detections: list[FailureModeDetection] = []
    for section in outline:
        key = section.get("key", "")
        body = sections.get(key, "")
        role = section.get("section_role", "general")
        min_len = _MIN_SECTION_LENGTH_BY_ROLE.get(role, MIN_SECTION_LENGTH)
        if not body:
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM3_INCOMPLETE_INFORMATION,
                    severity="warning",
                    section_key=key,
                    description=f"Section '{key}' is missing",
                )
            )
        elif len(body) < min_len:
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM3_INCOMPLETE_INFORMATION,
                    severity="warning",
                    section_key=key,
                    description=f"Section '{key}' is too short ({len(body)} chars, min {min_len})",
                )
            )
    return detections


def detect_empty_body(
    sections: dict[str, str],
    outline: list[dict],
) -> list[FailureModeDetection]:
    """FM4: Detect sections with empty body → blocking."""
    detections: list[FailureModeDetection] = []
    for section in outline:
        key = section.get("key", "")
        body = sections.get(key, "")
        if not body.strip():
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM4_EMPTY_BODY,
                    severity="blocking",
                    section_key=key,
                    description=f"Section '{key}' has empty body",
                    suggested_fix=f"Regenerate section '{key}' — body is empty (thinking exhaustion or no claims).",
                )
            )
    return detections


def detect_zero_claims(
    claim_plans: dict[str, list[dict]],
    outline: list[dict],
    extracted_facts: list[dict] | None = None,
) -> list[FailureModeDetection]:
    """FM5: Detect sections with zero claims when evidence exists → blocking."""
    detections: list[FailureModeDetection] = []
    has_evidence = bool(extracted_facts)

    for section in outline:
        key = section.get("key", "")
        claims = claim_plans.get(key, [])
        if not claims and has_evidence:
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM5_ZERO_CLAIMS,
                    severity="blocking",
                    section_key=key,
                    description=f"Section '{key}' has zero claims despite available evidence",
                    suggested_fix=f"Re-plan claims for section '{key}' using deterministic fallback.",
                )
            )
    return detections


def detect_es_numeric_absence(
    claim_plans: dict[str, list[dict]],
    outline: list[dict],
) -> list[FailureModeDetection]:
    """FM11: Detect ES without numeric facts → warning."""
    detections: list[FailureModeDetection] = []
    for section in outline:
        if section.get("section_role") != "executive_summary":
            continue
        key = section.get("key", "")
        claims = claim_plans.get(key, [])
        has_numeric = any(claim.get("numeric_facts") for claim in claims)
        if claims and not has_numeric:
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM11_ES_NUMERIC_ABSENCE,
                    severity="warning",
                    section_key=key,
                    description=f"Executive summary '{key}' has no numeric data in claims",
                    suggested_fix=f"Include at least one claim with numeric facts in '{key}'.",
                )
            )
    return detections


def _build_claim_feedbacks(
    detections: list[FailureModeDetection],
    section_paragraphs: dict[str, list[dict]],
) -> dict[str, list[dict]]:
    """Build paragraph-level claim_feedbacks from heuristic detections.

    Maps each detection to claim-level feedback with specific reason.
    """
    feedbacks: dict[str, list[dict]] = {}

    # Index detections that target specific paragraphs
    para_detections: dict[str, list[FailureModeDetection]] = {}
    section_detections: dict[str, list[FailureModeDetection]] = {}
    for d in detections:
        if d.mode == FailureMode.FM12_PARAGRAPH_DUPLICATION:
            para_detections.setdefault(d.section_key, []).append(d)
        elif d.mode == FailureMode.FM13_PARAGRAPH_MISSING_CITATION:
            para_detections.setdefault(d.section_key, []).append(d)
        else:
            section_detections.setdefault(d.section_key, []).append(d)

    blocking_sections = {d.section_key for d in detections if d.severity == "blocking"}

    for key in blocking_sections:
        paras = section_paragraphs.get(key, [])
        section_fbs: list[dict] = []
        if paras:
            for p in paras:
                claim_id = p.get("claim_id", "")
                body = p.get("body", "")

                if not body:
                    section_fbs.append(
                        {
                            "claim_id": claim_id,
                            "action": "regenerate",
                            "reason": "body empty — regenerate with claim evidence",
                        }
                    )
                    continue

                # Check paragraph-level detections
                p_dets = [d for d in para_detections.get(key, []) if claim_id and claim_id in d.description]
                if p_dets:
                    for pd in p_dets:
                        if pd.mode == FailureMode.FM12_PARAGRAPH_DUPLICATION:
                            section_fbs.append(
                                {
                                    "claim_id": claim_id,
                                    "action": "regenerate",
                                    "reason": f"duplicate content — {pd.description}",
                                }
                            )
                        elif pd.mode == FailureMode.FM13_PARAGRAPH_MISSING_CITATION:
                            section_fbs.append(
                                {
                                    "claim_id": claim_id,
                                    "action": "regenerate",
                                    "reason": f"missing citation — {pd.suggested_fix}",
                                }
                            )
                    continue

                if p.get("status") == "rejected":
                    section_fbs.append(
                        {
                            "claim_id": claim_id,
                            "action": "regenerate",
                            "reason": "paragraph rejected — regenerate with claim evidence",
                        }
                    )
        else:
            section_fbs.append(
                {
                    "claim_id": "",
                    "action": "regenerate",
                    "reason": "section has blocking issues — regenerate all paragraphs",
                }
            )
        if section_fbs:
            feedbacks[key] = section_fbs

    return feedbacks


WARNING_ACCUMULATION_THRESHOLD = 3


CONCLUSION_DUPLICATION_THRESHOLD = 0.20  # Jaccard bigram overlap


def _bigrams(text: str) -> set[tuple[str, str]]:
    """Extract character bigrams from text, ignoring whitespace."""
    chars = text.replace(" ", "").replace("\n", "")
    return {(chars[i], chars[i + 1]) for i in range(len(chars) - 1)} if len(chars) >= 2 else set()


def detect_conclusion_analysis_duplication(
    sections: dict[str, str],
    outline: list[dict],
) -> list[FailureModeDetection]:
    """FM8: Detect if conclusion substantially repeats analysis content via bigram Jaccard overlap."""
    analysis_keys = {s.get("key", "") for s in outline if s.get("section_role") == "analysis"}
    conclusion_keys = {s.get("key", "") for s in outline if s.get("section_role") == "conclusion"}

    if not analysis_keys or not conclusion_keys:
        return []

    analysis_text = " ".join(sections.get(k, "") for k in analysis_keys)
    conclusion_text = " ".join(sections.get(k, "") for k in conclusion_keys)

    if not analysis_text.strip() or not conclusion_text.strip():
        return []

    a_bigrams = _bigrams(analysis_text)
    c_bigrams = _bigrams(conclusion_text)

    if not a_bigrams or not c_bigrams:
        return []

    intersection = a_bigrams & c_bigrams
    union = a_bigrams | c_bigrams
    jaccard = len(intersection) / len(union)

    if jaccard > CONCLUSION_DUPLICATION_THRESHOLD:
        return [
            FailureModeDetection(
                mode=FailureMode.FM8_CONCLUSION_ANALYSIS_DUPLICATION,
                severity="blocking",
                section_key=next(iter(conclusion_keys)),
                description=f"Conclusion-Analysis bigram Jaccard overlap {jaccard:.2f} exceeds threshold {CONCLUSION_DUPLICATION_THRESHOLD}",
                suggested_fix="Conclusion should synthesize analysis findings, not repeat them. Rewrite to focus on implications, priorities, and recommendations.",
            )
        ]
    return []


NOVELTY_VIOLATION_THRESHOLD = 0.20  # Same Jaccard threshold as FM8


def detect_insufficient_citations(
    outline: list[dict],
    section_citations: dict[str, list[dict]],
) -> list[FailureModeDetection]:
    """FM9: Detect sections with fewer citations than min_citations contract."""
    detections: list[FailureModeDetection] = []
    for section in outline:
        key = section.get("key", "")
        min_cites = section.get("min_citations", 0)
        if min_cites <= 0:
            continue
        actual = len(section_citations.get(key, []))
        if actual < min_cites:
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM9_INSUFFICIENT_CITATIONS,
                    severity="warning",
                    section_key=key,
                    description=f"Section '{key}' has {actual} citations, minimum required is {min_cites}",
                    suggested_fix=f"Add more cited evidence to section '{key}' to meet the {min_cites} citation minimum.",
                )
            )
    return detections


def detect_novelty_violation(
    sections: dict[str, str],
    outline: list[dict],
) -> list[FailureModeDetection]:
    """FM10: Detect sections that violate novelty_against contract via bigram Jaccard overlap."""
    detections: list[FailureModeDetection] = []
    for section_def in outline:
        key = section_def.get("key", "")
        novelty_against = section_def.get("novelty_against", [])
        if not novelty_against:
            continue

        section_text = sections.get(key, "")
        if not section_text.strip():
            continue

        s_bigrams = _bigrams(section_text)
        if not s_bigrams:
            continue

        for against_key in novelty_against:
            against_text = sections.get(against_key, "")
            if not against_text.strip():
                continue
            a_bigrams = _bigrams(against_text)
            if not a_bigrams:
                continue

            intersection = s_bigrams & a_bigrams
            union = s_bigrams | a_bigrams
            jaccard = len(intersection) / len(union)

            if jaccard > NOVELTY_VIOLATION_THRESHOLD:
                detections.append(
                    FailureModeDetection(
                        mode=FailureMode.FM10_NOVELTY_VIOLATION,
                        severity="blocking",
                        section_key=key,
                        description=f"Section '{key}' has {jaccard:.2f} bigram Jaccard overlap with '{against_key}' (threshold {NOVELTY_VIOLATION_THRESHOLD})",
                        suggested_fix=f"Rewrite section '{key}' to avoid repeating content from '{against_key}'.",
                    )
                )
                break  # One detection per section is enough

    return detections


PARAGRAPH_DUPLICATION_THRESHOLD = 0.30


def detect_paragraph_duplication(
    section_paragraphs: dict[str, list[dict]],
    outline: list[dict],
) -> list[FailureModeDetection]:
    """FM12: Detect paragraph-level duplication within and across sections."""
    detections: list[FailureModeDetection] = []

    # Build novelty_against map from outline
    novelty_map: dict[str, list[str]] = {}
    for section_def in outline:
        key = section_def.get("key", "")
        novelty_map[key] = section_def.get("novelty_against", [])

    for key, paras in section_paragraphs.items():
        # Intra-section: compare paragraphs within same section
        for i, p1 in enumerate(paras):
            b1 = p1.get("body", "")
            if not b1.strip():
                continue
            bg1 = _bigrams(b1)
            if not bg1:
                continue

            for j, p2 in enumerate(paras):
                if j <= i:
                    continue
                b2 = p2.get("body", "")
                if not b2.strip():
                    continue
                bg2 = _bigrams(b2)
                if not bg2:
                    continue
                intersection = bg1 & bg2
                union = bg1 | bg2
                jaccard = len(intersection) / len(union)
                if jaccard > PARAGRAPH_DUPLICATION_THRESHOLD:
                    detections.append(
                        FailureModeDetection(
                            mode=FailureMode.FM12_PARAGRAPH_DUPLICATION,
                            severity="blocking",
                            section_key=key,
                            description=f"Paragraph '{p2.get('claim_id', '')}' duplicates '{p1.get('claim_id', '')}' (Jaccard {jaccard:.2f})",
                            suggested_fix=f"Rephrase paragraph '{p2.get('claim_id', '')}' to differentiate from '{p1.get('claim_id', '')}'.",
                        )
                    )

        # Cross-section: compare against novelty_against targets
        against_keys = novelty_map.get(key, [])
        for against_key in against_keys:
            against_paras = section_paragraphs.get(against_key, [])
            for p in paras:
                body = p.get("body", "")
                if not body.strip():
                    continue
                p_bg = _bigrams(body)
                if not p_bg:
                    continue
                for ap in against_paras:
                    a_body = ap.get("body", "")
                    if not a_body.strip():
                        continue
                    a_bg = _bigrams(a_body)
                    if not a_bg:
                        continue
                    intersection = p_bg & a_bg
                    union = p_bg | a_bg
                    jaccard = len(intersection) / len(union)
                    if jaccard > PARAGRAPH_DUPLICATION_THRESHOLD:
                        detections.append(
                            FailureModeDetection(
                                mode=FailureMode.FM12_PARAGRAPH_DUPLICATION,
                                severity="blocking",
                                section_key=key,
                                description=f"Paragraph '{p.get('claim_id', '')}' overlaps with '{against_key}/{ap.get('claim_id', '')}' (Jaccard {jaccard:.2f})",
                                suggested_fix=f"Rephrase paragraph '{p.get('claim_id', '')}' to avoid repeating '{against_key}' content.",
                            )
                        )
                        break  # one detection per paragraph per against_key

    return detections


def detect_paragraph_missing_citation(
    section_paragraphs: dict[str, list[dict]],
    claim_plans: dict[str, list[dict]],
) -> list[FailureModeDetection]:
    """FM13: Detect paragraphs missing citations when claim has must_cite=True."""
    detections: list[FailureModeDetection] = []

    for key, paras in section_paragraphs.items():
        plans = claim_plans.get(key, [])
        plan_by_id = {p.get("claim_id", ""): p for p in plans}

        for para in paras:
            claim_id = para.get("claim_id", "")
            plan = plan_by_id.get(claim_id, {})
            if not plan.get("must_cite", False):
                continue
            citations = para.get("citations", [])
            if not citations:
                detections.append(
                    FailureModeDetection(
                        mode=FailureMode.FM13_PARAGRAPH_MISSING_CITATION,
                        severity="warning",
                        section_key=key,
                        description=f"Paragraph '{claim_id}' has must_cite=True but no citations",
                        suggested_fix=f"Include citation reference for claim '{claim_id}'.",
                    )
                )

    return detections


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
        brief = state.get("brief") or state.get("scope") or {}
        outline = state.get("outline", [])
        topic = brief.get("topic", "")

        claim_plans = state.get("claim_plans", {})
        extracted_facts = state.get("extracted_facts", [])
        section_paragraphs = state.get("section_paragraphs", {})

        all_detections: list[FailureModeDetection] = []

        # FM4: Empty body (blocking)
        all_detections.extend(detect_empty_body(sections, outline))

        # FM5: Zero claims (blocking when evidence exists)
        all_detections.extend(detect_zero_claims(claim_plans, outline, extracted_facts))

        # FM11: ES numeric absence (warning)
        all_detections.extend(detect_es_numeric_absence(claim_plans, outline))

        # FM2: Meta-statement heuristic (no LLM)
        all_detections.extend(detect_meta_statements(sections))

        # FM3: Incomplete sections heuristic (no LLM) — per-role thresholds
        all_detections.extend(detect_incomplete_sections(sections, outline))

        # FM8: Conclusion-Analysis duplication heuristic (no LLM)
        all_detections.extend(detect_conclusion_analysis_duplication(sections, outline))

        # FM9: Insufficient citations (contract-driven)
        section_citations = state.get("section_citations", {})
        all_detections.extend(detect_insufficient_citations(outline, section_citations))

        # FM10: Novelty violation (contract-driven)
        all_detections.extend(detect_novelty_violation(sections, outline))

        # FM12: Paragraph-level duplication (within and cross-section)
        all_detections.extend(detect_paragraph_duplication(section_paragraphs, outline))

        # FM13: Paragraph-level missing citation
        all_detections.extend(detect_paragraph_missing_citation(section_paragraphs, claim_plans))

        # Warning accumulation: 3+ warnings on same section → promote to blocking
        warning_counts: dict[str, int] = {}
        for d in all_detections:
            if d.severity == "warning":
                warning_counts[d.section_key] = warning_counts.get(d.section_key, 0) + 1
        for section_key, count in warning_counts.items():
            if count >= WARNING_ACCUMULATION_THRESHOLD:
                all_detections.append(
                    FailureModeDetection(
                        mode=FailureMode.FM3_INCOMPLETE_INFORMATION,
                        severity="blocking",
                        section_key=section_key,
                        description=f"Section '{section_key}' has {count} warnings — promoted to blocking",
                        suggested_fix=f"Address accumulated quality issues in section '{section_key}'.",
                    )
                )

        blocking = [d for d in all_detections if d.severity == "blocking"]

        if blocking:
            # Skip LLM call — heuristic failures are sufficient to trigger revision
            revise_sections = list({d.section_key for d in blocking})
            feedback = {d.section_key: d.suggested_fix or d.description for d in blocking}
            critique = {
                "reasoning": "Heuristic checks detected blocking issues",
                "verdict": "revise",
                "failure_modes": [
                    {"mode": d.mode.value, "section": d.section_key, "description": d.description}
                    for d in all_detections
                ],
                "revise_sections": revise_sections,
                "feedback": feedback,
            }
        else:
            # FM1 + FM7: LLM-based relevancy and grounding check
            sections_text = "\n\n".join(f"## {k}\n{v}" for k, v in sections.items())
            prompt = CRITIC_PROMPT.format(topic=topic, sections=sections_text)

            response = await self._llm.generate(
                prompt,
                num_predict=512,
                temperature=0,
                format=_CRITIC_FORMAT,
                mode=LLMMode.STRUCTURED,
            )

            try:
                critique = json.loads(response.text)
            except json.JSONDecodeError:
                # Parse failure → revise, never silent accept
                critique = {
                    "reasoning": "Critic output malformed, requesting revision",
                    "verdict": "revise",
                    "failure_modes": [],
                    "revise_sections": list(sections.keys()),
                    "feedback": dict.fromkeys(sections, "Revision requested due to critic parse failure"),
                }

            # Merge heuristic detections into LLM response
            if all_detections:
                raw_fms = critique.get("failure_modes", [])
                existing_fms: list[dict] = list(raw_fms) if isinstance(raw_fms, list) else []  # type: ignore[arg-type]
                for d in all_detections:
                    existing_fms.append(
                        {
                            "mode": d.mode.value,
                            "section": d.section_key,
                            "description": d.description,
                        }
                    )
                critique["failure_modes"] = existing_fms

        # Build claim-level feedbacks from detections + paragraph status
        claim_fbs: dict[str, list[dict]] = _build_claim_feedbacks(all_detections, section_paragraphs)
        critique["claim_feedbacks"] = claim_fbs  # type: ignore[assignment]

        logger.info("Critic completed", verdict=critique.get("verdict"), detections=len(all_detections))
        return {
            "critique": critique,
            "claim_feedbacks": claim_fbs,
            "failure_modes": [
                {"mode": d.mode.value, "section": d.section_key, "description": d.description} for d in all_detections
            ],
        }
