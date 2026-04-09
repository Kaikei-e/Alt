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

MIN_SECTION_LENGTH = 100

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
    """FM3: Detect sections that are too short or missing."""
    detections: list[FailureModeDetection] = []
    for section in outline:
        key = section.get("key", "")
        body = sections.get(key, "")
        if not body:
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM3_INCOMPLETE_INFORMATION,
                    severity="warning",
                    section_key=key,
                    description=f"Section '{key}' is missing",
                )
            )
        elif len(body) < MIN_SECTION_LENGTH:
            detections.append(
                FailureModeDetection(
                    mode=FailureMode.FM3_INCOMPLETE_INFORMATION,
                    severity="warning",
                    section_key=key,
                    description=f"Section '{key}' is too short ({len(body)} chars, min {MIN_SECTION_LENGTH})",
                )
            )
    return detections


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

        all_detections: list[FailureModeDetection] = []

        # FM2: Meta-statement heuristic (no LLM)
        all_detections.extend(detect_meta_statements(sections))

        # FM3: Incomplete sections heuristic (no LLM)
        all_detections.extend(detect_incomplete_sections(sections, outline))

        # FM8: Conclusion-Analysis duplication heuristic (no LLM)
        all_detections.extend(detect_conclusion_analysis_duplication(sections, outline))

        # FM9: Insufficient citations (contract-driven)
        section_citations = state.get("section_citations", {})
        all_detections.extend(detect_insufficient_citations(outline, section_citations))

        # FM10: Novelty violation (contract-driven)
        all_detections.extend(detect_novelty_violation(sections, outline))

        # Check if heuristic checks already found blocking issues
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
                    "feedback": {k: "Revision requested due to critic parse failure" for k in sections},
                }

            # Merge heuristic detections into LLM response
            if all_detections:
                existing_fms = critique.get("failure_modes", [])
                for d in all_detections:
                    existing_fms.append(
                        {
                            "mode": d.mode.value,
                            "section": d.section_key,
                            "description": d.description,
                        }
                    )
                critique["failure_modes"] = existing_fms

        logger.info("Critic completed", verdict=critique.get("verdict"), detections=len(all_detections))
        return {
            "critique": critique,
            "failure_modes": [
                {"mode": d.mode.value, "section": d.section_key, "description": d.description} for d in all_detections
            ],
        }
