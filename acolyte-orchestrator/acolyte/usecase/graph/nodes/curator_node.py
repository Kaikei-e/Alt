"""Curator node — per-section evidence ranking and filtering.

For each section in the outline, filters evidence by section_key tag
and curates to max_evidence items. Uses LLM for sections exceeding
the limit. Outputs both curated_by_section and backward-compat curated.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

CURATOR_PROMPT = """You are an evidence curator. Given these evidence items, select the top {limit} most relevant for a report section about: {topic} — {section_title}

Evidence:
{evidence}

Return a JSON array of the selected item IDs in order of relevance.
"""


class CuratorNode:
    def __init__(self, llm: LLMProviderPort, *, max_evidence: int = 10) -> None:
        self._llm = llm
        self._max_evidence = max_evidence

    async def __call__(self, state: ReportGenerationState) -> dict:
        evidence = state.get("evidence", [])
        brief = state.get("brief") or state.get("scope") or {}
        outline = state.get("outline", [])
        topic = brief.get("topic", "")

        # Per-section curation
        curated_by_section: dict[str, list[dict]] = {}

        for section in outline:
            section_key = section.get("key", "")
            section_title = section.get("title", section_key)

            # Filter evidence tagged for this section
            section_evidence = [
                e for e in evidence if section_key in e.get("section_keys", [])
            ]

            if len(section_evidence) <= self._max_evidence:
                curated_by_section[section_key] = section_evidence
            else:
                # LLM curation for sections exceeding limit
                curated_by_section[section_key] = await self._curate_with_llm(
                    section_evidence, topic, section_title
                )

        # Backward compat: flatten curated_by_section into a deduplicated curated list
        seen_ids: set[str] = set()
        curated_flat: list[dict] = []
        for items in curated_by_section.values():
            for item in items:
                item_id = item.get("id", "")
                if item_id not in seen_ids:
                    seen_ids.add(item_id)
                    curated_flat.append(item)

        # If no outline-based curation happened, fall back to global curation
        if not curated_by_section and evidence:
            if len(evidence) <= self._max_evidence:
                curated_flat = evidence
            else:
                curated_flat = evidence[: self._max_evidence]

        logger.info(
            "Curator completed",
            sections_curated=len(curated_by_section),
            total_curated=len(curated_flat),
        )
        return {"curated_by_section": curated_by_section, "curated": curated_flat}

    async def _curate_with_llm(
        self, section_evidence: list[dict], topic: str, section_title: str
    ) -> list[dict]:
        """Use LLM to select top evidence items for a section."""
        prompt = CURATOR_PROMPT.format(
            limit=self._max_evidence,
            topic=topic,
            section_title=section_title,
            evidence=json.dumps(section_evidence[:30]),
        )
        response = await self._llm.generate(prompt)

        try:
            selected_ids = json.loads(response.text)
            id_set = set(selected_ids)
            return [e for e in section_evidence if e.get("id") in id_set]
        except (json.JSONDecodeError, TypeError):
            return section_evidence[: self._max_evidence]
