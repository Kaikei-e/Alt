"""Gatherer node — searches for evidence based on scope and outline."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.evidence_provider import EvidenceProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)


class GathererNode:
    def __init__(self, evidence: EvidenceProviderPort) -> None:
        self._evidence = evidence

    async def __call__(self, state: ReportGenerationState) -> dict:
        scope = state.get("scope", {})
        query = scope.get("topic", "technology trends")

        articles = []
        recaps = []

        try:
            articles = await self._evidence.search_articles(query, limit=20)
        except Exception as exc:
            logger.warning("Gatherer: article search failed, continuing with empty", error=str(exc))

        try:
            recaps = await self._evidence.search_recaps(query, limit=10)
        except Exception as exc:
            logger.warning("Gatherer: recap search failed, continuing with empty", error=str(exc))

        evidence = [{"type": "article", "id": a.article_id, "title": a.title, "score": a.score} for a in articles] + [
            {"type": "recap", "id": r.recap_id, "title": r.title, "score": r.score} for r in recaps
        ]

        logger.info("Gatherer completed", evidence_count=len(evidence))
        return {"evidence": evidence}
