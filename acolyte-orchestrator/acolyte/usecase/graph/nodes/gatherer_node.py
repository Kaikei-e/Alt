"""Gatherer node — multi-query search with section tagging and deduplication.

For each section's search_queries, searches evidence and tags results
with the originating section_key. Deduplicates by article_id (merging
section_keys). Falls back to topic-based search when no search_queries present.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.content_store import ContentStorePort
    from acolyte.port.evidence_provider import EvidenceProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)


class GathererNode:
    def __init__(
        self,
        evidence: EvidenceProviderPort,
        *,
        content_store: ContentStorePort | None = None,
    ) -> None:
        self._evidence = evidence
        self._content_store = content_store

    async def __call__(self, state: ReportGenerationState) -> dict:
        brief = state.get("brief") or state.get("scope") or {}
        topic = brief.get("topic", "")
        if not topic:
            return {"evidence": [], "error": "No topic in brief — cannot gather evidence"}

        outline = state.get("outline", [])

        # Collect per-query search results tagged with section keys
        # Key: article_id → evidence dict with merged section_keys
        evidence_map: dict[str, dict] = {}
        recap_map: dict[str, dict] = {}

        # Build list of (section_key, query) pairs
        query_pairs: list[tuple[str, str]] = []
        for section in outline:
            section_key = section.get("key", "")
            queries = section.get("search_queries", [])
            if queries:
                for q in queries:
                    query_pairs.append((section_key, q))
            else:
                # Fallback: use topic as query for sections without search_queries
                query_pairs.append((section_key, topic))

        # If no outline at all, fall back to single topic query
        if not query_pairs:
            query_pairs = [("_global", topic)]

        for section_key, query in query_pairs:
            try:
                articles = await self._evidence.search_articles(query, limit=5)
            except Exception as exc:
                logger.warning("Gatherer: article search failed", query=query, error=str(exc))
                articles = []

            for a in articles:
                if a.article_id in evidence_map:
                    # Merge section_keys
                    existing_keys = evidence_map[a.article_id]["section_keys"]
                    if section_key not in existing_keys:
                        existing_keys.append(section_key)
                else:
                    evidence_map[a.article_id] = {
                        "type": "article",
                        "id": a.article_id,
                        "title": a.title,
                        "score": a.score,
                        "section_keys": [section_key],
                    }

        # Also search recaps with the main topic
        try:
            recaps = await self._evidence.search_recaps(topic, limit=10)
        except Exception as exc:
            logger.warning("Gatherer: recap search failed", error=str(exc))
            recaps = []

        for r in recaps:
            if r.recap_id not in recap_map:
                recap_map[r.recap_id] = {
                    "type": "recap",
                    "id": r.recap_id,
                    "title": r.title,
                    "score": r.score,
                    "section_keys": ["_global"],
                }

        evidence = list(evidence_map.values()) + list(recap_map.values())

        logger.info("Gatherer completed", evidence_count=len(evidence), query_count=len(query_pairs))
        return {"evidence": evidence}
