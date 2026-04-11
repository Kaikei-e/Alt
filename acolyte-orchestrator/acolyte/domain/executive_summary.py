"""Executive Summary renderer — deterministic generation from accepted claims.

Replaces LLM-based ES generation. Guarantees completion and key point coverage
by rendering accepted claims into a structured Japanese summary.
"""

from __future__ import annotations


class ExecutiveSummaryRenderer:
    """Render executive summary from accepted synthesis claims."""

    def render(self, claims: list[dict], *, topic: str = "") -> str:
        """Render claims into a Japanese summary paragraph.

        Returns empty string if no claims are provided.
        """
        if not claims:
            return ""

        # Prioritize claims with numeric_facts
        with_numeric = [c for c in claims if c.get("numeric_facts")]
        without_numeric = [c for c in claims if not c.get("numeric_facts")]
        ordered = with_numeric + without_numeric

        sentences: list[str] = []
        for claim in ordered:
            text = claim.get("claim", "")
            if not text:
                continue

            numeric_facts = claim.get("numeric_facts", [])
            if numeric_facts:
                # Ensure numeric data appears in the sentence
                nums_in_text = all(n in text for n in numeric_facts)
                if not nums_in_text:
                    text = f"{text}（{', '.join(numeric_facts)}）"

            # Ensure sentence ends with period
            if not text.endswith("。"):
                text += "。"

            sentences.append(text)

        return "".join(sentences)

    def build_citations(self, claims: list[dict]) -> list[dict]:
        """Build citation list from claims' evidence_ids."""
        citations: list[dict] = []
        for claim in claims:
            claim_id = claim.get("claim_id", "")
            for eid in claim.get("evidence_ids", []):
                quote = ""
                quotes = claim.get("supporting_quotes", [])
                if quotes:
                    quote = quotes[0]
                citations.append(
                    {
                        "claim_id": claim_id,
                        "source_id": eid,
                        "source_type": "article",
                        "quote": quote,
                        "offset_start": -1,
                        "offset_end": -1,
                    }
                )
        return citations
