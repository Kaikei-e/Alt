"""Extractor node — extracts atomic facts from hydrated evidence.

Uses article-level degradation ladder (resolve quality hotfix):
  Pass 1 (full): max_facts=3, quote=120 chars, num_predict=4000
  Pass 2 (light): max_facts=2, quote=80 chars, body truncated, num_predict=3000
  Pass 3 (quote-only): No LLM call — use compressed spans directly as facts
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.domain.fact import ExtractorOutput
from acolyte.usecase.graph.llm_parse import generate_validated

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

_FULL_PROMPT = """Extract up to {max_facts} atomic factual claims from this article.
For each claim, include the exact quote from the source that supports it.

Article ID: {source_id}
Article Title: {source_title}
Article Body:
{body}

Return JSON with "reasoning" (one sentence) and "facts" array.
Each fact: {{"claim": "text", "source_id": "{source_id}", "source_title": "{source_title}", "verbatim_quote": "exact quote (max {max_quote} chars)", "confidence": 0.0-1.0, "data_type": "statistic|date|quote|trend|comparison"}}
Keep reasoning to one sentence. Keep verbatim_quote to {max_quote} characters max."""

# Degradation ladder configuration
_PASSES = [
    {"max_facts": 3, "max_quote": 120, "num_predict": 4000, "max_body": 1200},
    {"max_facts": 2, "max_quote": 80, "num_predict": 3000, "max_body": 800},
]


def _quote_only_facts(
    body: str,
    source_id: str,
    source_title: str,
    *,
    max_sentences: int = 3,
) -> list[dict]:
    """Pass 3: extract facts directly from body text without LLM call."""
    sentences = [s.strip() for s in body.split("\n") if s.strip()]
    if not sentences:
        sentences = [body[:200]]

    facts = []
    for sent in sentences[:max_sentences]:
        facts.append(
            {
                "claim": sent[:200],
                "source_id": source_id,
                "source_title": source_title,
                "verbatim_quote": sent[:120],
                "confidence": 0.3,
                "data_type": "quote",
            }
        )
    return facts


class ExtractorNode:
    def __init__(self, llm: LLMProviderPort, *, max_facts_per_item: int = 5) -> None:
        self._llm = llm
        self._max_facts = max_facts_per_item

    async def __call__(self, state: ReportGenerationState) -> dict:
        curated_by_section = state.get("curated_by_section", {})
        hydrated = state.get("hydrated_evidence", {})
        compressed = state.get("compressed_evidence", {})

        # Collect unique article IDs across all sections
        seen_ids: set[str] = set()
        items_to_extract: list[dict] = []
        for items in curated_by_section.values():
            for item in items:
                item_id = item.get("id", "")
                if item_id not in seen_ids and item_id in hydrated:
                    seen_ids.add(item_id)
                    items_to_extract.append(item)

        all_facts: list[dict] = []

        for item in items_to_extract:
            item_id = item.get("id", "")
            title = item.get("title", "")

            # Resolve body: compressed > hydrated
            body = self._resolve_body(item_id, compressed, hydrated)
            if not body:
                continue

            facts = await self._extract_with_degradation(item_id, title, body)
            all_facts.extend(facts)

        logger.info("Extractor completed", fact_count=len(all_facts), articles_processed=len(items_to_extract))
        return {"extracted_facts": all_facts}

    def _resolve_body(
        self,
        item_id: str,
        compressed: dict[str, list[dict]],
        hydrated: dict[str, str],
    ) -> str:
        """Resolve article body with tiered fallback."""
        if item_id in compressed:
            spans = compressed[item_id]
            if spans:
                return "\n".join(s["text"] for s in spans)
            # Compression returned empty — fall back to hydrated
            logger.warning("Compression returned empty, falling back to hydrated", article_id=item_id)

        body = hydrated.get(item_id, "")
        if len(body) > 2000:
            body = body[:2000]
        return body

    async def _extract_with_degradation(
        self,
        source_id: str,
        source_title: str,
        body: str,
    ) -> list[dict]:
        """Try extraction with degradation ladder: full → light → quote-only."""
        for pass_idx, config in enumerate(_PASSES):
            try:
                pass_body = body[: config["max_body"]]
                prompt = _FULL_PROMPT.format(
                    max_facts=config["max_facts"],
                    max_quote=config["max_quote"],
                    source_id=source_id,
                    source_title=source_title,
                    body=pass_body,
                )

                fallback = ExtractorOutput(facts=[])
                result = await generate_validated(
                    self._llm,
                    prompt,
                    ExtractorOutput,
                    temperature=0,
                    num_predict=config["num_predict"],
                    fallback=fallback,
                )

                if result.facts:
                    return [f.model_dump() for f in result.facts[: config["max_facts"]]]

                # LLM returned empty facts — degrade to next pass
                logger.warning(
                    "Extraction returned no facts, degrading",
                    article_id=source_id,
                    pass_idx=pass_idx,
                )
            except Exception as exc:
                logger.warning(
                    "Extraction failed, degrading",
                    article_id=source_id,
                    pass_idx=pass_idx,
                    error=str(exc),
                )

        # Pass 3: quote-only fallback (no LLM)
        logger.warning("Using quote-only fallback", article_id=source_id)
        return _quote_only_facts(body, source_id, source_title)
