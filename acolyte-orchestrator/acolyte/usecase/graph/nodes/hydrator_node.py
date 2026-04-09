"""Hydrator node — fetches full article bodies from ContentStore for curated evidence.

Only hydrates article-type evidence (not recaps).
Outputs hydrated_evidence: dict[article_id, body_text].
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.content_store import ContentStorePort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)


class HydratorNode:
    def __init__(self, content_store: ContentStorePort) -> None:
        self._content_store = content_store

    async def __call__(self, state: ReportGenerationState) -> dict:
        curated_by_section = state.get("curated_by_section")

        # Collect all unique article IDs to hydrate
        article_ids: set[str] = set()

        if curated_by_section:
            for items in curated_by_section.values():
                for item in items:
                    if item.get("type") == "article":
                        article_ids.add(item["id"])
        else:
            # Fallback: use flat curated list
            curated = state.get("curated", [])
            for item in curated:
                if item.get("type") == "article":
                    article_ids.add(item["id"])

        if not article_ids:
            return {"hydrated_evidence": {}}

        hydrated = await self._content_store.fetch_many(list(article_ids))

        logger.info(
            "Hydrator completed",
            requested=len(article_ids),
            hydrated=len(hydrated),
        )
        return {"hydrated_evidence": hydrated}
