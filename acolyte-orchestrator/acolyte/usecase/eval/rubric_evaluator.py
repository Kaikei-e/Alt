"""Rubric evaluator — LLM-based claim decomposition and verification.

Uses LLMProviderPort as adapter (not a separate gateway).
Evaluates Factual Consistency and Citation Association.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from acolyte.domain.eval import EvalDimension

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)

CLAIM_EXTRACTION_PROMPT = """Analyze the following report text and extract atomic factual claims.
For each claim, determine if it is supported by any of the provided evidence sources.

Report text:
{text}

Evidence sources:
{evidence}

Return JSON with "claims" array. Each claim: {{"claim": "text", "supported": true/false, "source_id": "id or empty"}}"""


class RubricEvaluator:
    """LLM-based rubric evaluator for factual consistency and citations."""

    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def _extract_claims(
        self,
        sections: dict[str, str],
        evidence: list[dict],
    ) -> list[dict]:
        """Extract and verify claims from report sections."""
        text = "\n\n".join(f"## {k}\n{v}" for k, v in sections.items())
        evidence_text = json.dumps([{"id": e.get("id", ""), "title": e.get("title", "")} for e in evidence])

        prompt = CLAIM_EXTRACTION_PROMPT.format(text=text[:3000], evidence=evidence_text)
        response = await self._llm.generate(prompt, temperature=0, num_predict=1024, think=False)

        try:
            parsed = json.loads(response.text)
            return parsed.get("claims", [])
        except (json.JSONDecodeError, TypeError):  # fmt: skip
            logger.warning("Claim extraction failed", raw_len=len(response.text))
            return []

    async def evaluate_factual_consistency(
        self,
        sections: dict[str, str],
        evidence: list[dict],
    ) -> EvalDimension:
        """FActScore-inspired: ratio of supported claims."""
        claims = await self._extract_claims(sections, evidence)
        if not claims:
            return EvalDimension(name="factual_consistency", score=0.0, protocol="rubric")

        supported = sum(1 for c in claims if c.get("supported"))
        score = supported / len(claims)
        return EvalDimension(
            name="factual_consistency",
            score=score,
            protocol="rubric",
            details={"total_claims": len(claims), "supported": supported},
        )

    async def evaluate_citation_association(
        self,
        sections: dict[str, str],
        evidence: list[dict],
    ) -> EvalDimension:
        """Ratio of claims with a source_id."""
        claims = await self._extract_claims(sections, evidence)
        if not claims:
            return EvalDimension(name="citation_association", score=0.0, protocol="rubric")

        cited = sum(1 for c in claims if c.get("source_id"))
        score = cited / len(claims)
        return EvalDimension(
            name="citation_association",
            score=score,
            protocol="rubric",
            details={"total_claims": len(claims), "cited": cited},
        )

    async def evaluate(
        self,
        sections: dict[str, str],
        evidence: list[dict],
    ) -> list[EvalDimension]:
        """Run all rubric evaluations."""
        return [
            await self.evaluate_factual_consistency(sections, evidence),
            await self.evaluate_citation_association(sections, evidence),
        ]
