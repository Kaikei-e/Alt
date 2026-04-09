"""Curator node — ranks and filters evidence for relevance."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

CURATOR_PROMPT = """You are an evidence curator. Given these evidence items, select the top {limit} most relevant for a report about: {topic}

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
        scope = state.get("scope", {})

        if len(evidence) <= self._max_evidence:
            logger.info("Curator: evidence within limit, keeping all", count=len(evidence))
            return {"curated": evidence}

        prompt = CURATOR_PROMPT.format(
            limit=self._max_evidence,
            topic=scope.get("topic", ""),
            evidence=json.dumps(evidence[:30]),
        )
        response = await self._llm.generate(prompt)

        try:
            selected_ids = json.loads(response.text)
            id_set = set(selected_ids)
            curated = [e for e in evidence if e.get("id") in id_set]
        except json.JSONDecodeError, TypeError:
            curated = evidence[: self._max_evidence]

        logger.info("Curator completed", curated_count=len(curated))
        return {"curated": curated}
