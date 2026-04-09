"""Extractor node — extracts atomic facts from hydrated evidence.

Processes each curated evidence item with full body text,
using LLM structured output to extract up to max_facts_per_item
atomic factual claims with source attribution.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from acolyte.domain.fact import ExtractorOutput
from acolyte.usecase.graph.llm_parse import generate_validated

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

EXTRACTOR_PROMPT = """Extract up to {max_facts} atomic factual claims from this article.
For each claim, include the exact quote from the source that supports it.

Article ID: {source_id}
Article Title: {source_title}
Article Body:
{body}

Return JSON with "facts" array. Each fact: {{"claim": "text", "source_id": "{source_id}", "source_title": "{source_title}", "verbatim_quote": "exact quote", "confidence": 0.0-1.0, "data_type": "statistic|date|quote|trend|comparison"}}"""


class ExtractorNode:
    def __init__(self, llm: LLMProviderPort, *, max_facts_per_item: int = 5) -> None:
        self._llm = llm
        self._max_facts = max_facts_per_item

    async def __call__(self, state: ReportGenerationState) -> dict:
        curated_by_section = state.get("curated_by_section", {})
        hydrated = state.get("hydrated_evidence", {})

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
            body = hydrated.get(item_id, "")
            if not body:
                continue

            prompt = EXTRACTOR_PROMPT.format(
                max_facts=self._max_facts,
                source_id=item_id,
                source_title=item.get("title", ""),
                body=body[:2000],  # Truncate for context window
            )

            fallback = ExtractorOutput(facts=[])
            result = await generate_validated(
                self._llm, prompt, ExtractorOutput,
                temperature=0, num_predict=4096, fallback=fallback,
            )

            for fact in result.facts[:self._max_facts]:
                all_facts.append(fact.model_dump())

        logger.info("Extractor completed", fact_count=len(all_facts), articles_processed=len(items_to_extract))
        return {"extracted_facts": all_facts}
