"""Gatherer node — multi-query search with section tagging, deduplication, and RRF fusion.

For each section's query_facets (Issue 6) or search_queries (legacy),
searches evidence and tags results with the originating section_key.
Deduplicates by article_id (merging section_keys).
Falls back to topic-based search when no queries present.

Issue 7: Multi-query retrieval generates variants per facet and fuses
results with RRF (Reciprocal Rank Fusion). FusionStrategy is injectable
for future CC (Convex Combination) migration.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.domain.fusion import RRFFusion, ScoredHit
from acolyte.domain.query_facet import WEAK_FACET_THRESHOLD
from acolyte.domain.query_variant import generate_query_variants

if TYPE_CHECKING:
    from acolyte.domain.fusion import FusionStrategy
    from acolyte.port.content_store import ContentStorePort
    from acolyte.port.evidence_provider import EvidenceProviderPort
    from acolyte.port.hyde_generator import HyDEGeneratorPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)


def _detect_topic_language(topic: str) -> str:
    """Cheap CJK vs ASCII detector. Mirrors the Go language_detector so the
    Gatherer can decide whether to request a JP→EN or EN→JA HyDE pass."""
    if not topic:
        return "und"
    cjk = 0
    latin = 0
    for ch in topic:
        code = ord(ch)
        if 0x3040 <= code <= 0x309F or 0x30A0 <= code <= 0x30FF or 0x4E00 <= code <= 0x9FFF:
            cjk += 1
        elif ch.isascii() and ch.isalpha():
            latin += 1
    if cjk >= 2 and cjk * 3 >= cjk + latin:
        return "ja"
    if latin >= 3 and latin > cjk * 2:
        return "en"
    return "und"


class GathererNode:
    def __init__(
        self,
        evidence: EvidenceProviderPort,
        *,
        content_store: ContentStorePort | None = None,
        fusion: FusionStrategy | None = None,
        hyde_generator: HyDEGeneratorPort | None = None,
    ) -> None:
        self._evidence = evidence
        self._content_store = content_store
        self._fusion = fusion or RRFFusion()
        self._hyde = hyde_generator

    async def __call__(self, state: ReportGenerationState) -> dict:
        brief = state.get("brief") or state.get("scope") or {}
        topic = brief.get("topic", "")
        if not topic:
            return {"evidence": [], "error": "No topic in brief — cannot gather evidence"}

        outline = state.get("outline", [])

        # Resolve the cross-lingual HyDE passage once per call and reuse it on
        # whichever retrieval path ends up running. ADR-000695 established
        # that Japanese→English (and vice versa) recall only works when an
        # explicit translated variant is queued alongside the original.
        hyde_variant = await self._resolve_hyde_variant(topic)

        # Detect faceted vs legacy path
        has_facets = any(section.get("query_facets") for section in outline)

        if has_facets:
            evidence_map, weak_facets = await self._search_by_facets(outline, topic, brief, hyde_variant)
        else:
            evidence_map = await self._search_by_queries(outline, topic, hyde_variant)
            weak_facets = []

        # Also search recaps with the main topic
        recap_map: dict[str, dict] = {}
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

        logger.info("Gatherer completed", evidence_count=len(evidence), faceted=has_facets)
        return {"evidence": evidence, "weak_facets": weak_facets}

    async def _resolve_hyde_variant(self, topic: str) -> tuple[str, str] | None:
        """Resolve a cross-lingual HyDE passage once per call.

        When the topic is Japanese we request an English HyDE (cross-lingual
        recall expansion); for English topics we request Japanese. A None
        generator surfaces as a warning — cross-lingual retrieval is a
        design-level invariant (see PM-2026-021) so silent omission is
        the worst failure mode.
        """
        topic_lang = _detect_topic_language(topic)
        target_lang: str | None = None
        if topic_lang == "ja":
            target_lang = "en"
        elif topic_lang == "en":
            target_lang = "ja"

        if target_lang is None:
            return None

        if self._hyde is None:
            logger.warning(
                "Gatherer: cross-lingual HyDE generator not wired — "
                "retrieval will not request translated recall variants",
                topic_lang=topic_lang,
                target_lang=target_lang,
            )
            return None

        hyde_doc = await self._hyde.generate_hypothetical_doc(topic, target_lang)
        if not hyde_doc:
            return None
        return (hyde_doc, f"hyde_{target_lang}")

    async def _search_by_facets(
        self,
        outline: list[dict],
        topic: str,
        brief: dict,
        hyde_variant: tuple[str, str] | None,
    ) -> tuple[dict[str, dict], list[dict]]:
        """Search using structured QueryFacet objects from outline with multi-query RRF fusion."""
        evidence_map: dict[str, dict] = {}
        weak_facets: list[dict] = []

        for section in outline:
            section_key = section.get("key", "")
            facets = section.get("query_facets", [])

            if not facets:
                continue

            for facet_idx, facet in enumerate(facets):
                # Issue 7: Generate query variants for multi-query retrieval
                variants = generate_query_variants(facet, topic, brief)
                if hyde_variant is not None:
                    variants = list(variants) + [hyde_variant]

                ranked_lists: list[list[ScoredHit]] = []
                total_hits = 0

                for query, source_label in variants:
                    if not query:
                        query = topic

                    try:
                        articles = await self._evidence.search_articles(query, limit=10)
                        scored = [
                            ScoredHit(
                                article_id=a.article_id,
                                title=a.title,
                                tags=a.tags,
                                score=a.score,
                                source=source_label,
                            )
                            for a in articles
                        ]
                        ranked_lists.append(scored)
                        total_hits += len(articles)
                    except Exception as exc:
                        logger.warning(
                            "Gatherer: variant search failed",
                            query=query,
                            source=source_label,
                            error=str(exc),
                        )

                # Fuse results from all successful variants
                fused = self._fusion.fuse(ranked_lists) if ranked_lists else []

                # Track weak facets (based on total fused results)
                if len(fused) < WEAK_FACET_THRESHOLD:
                    weak_facets.append(
                        {
                            "section_key": section_key,
                            "facet_index": facet_idx,
                            "intent": facet.get("intent", ""),
                            "raw_query": facet.get("raw_query", ""),
                            "hit_count": len(fused),
                            "threshold": WEAK_FACET_THRESHOLD,
                        }
                    )

                for hit in fused:
                    if hit.article_id in evidence_map:
                        existing_keys = evidence_map[hit.article_id]["section_keys"]
                        if section_key not in existing_keys:
                            existing_keys.append(section_key)
                    else:
                        evidence_map[hit.article_id] = {
                            "type": "article",
                            "id": hit.article_id,
                            "title": hit.title,
                            "score": hit.score,
                            "section_keys": [section_key],
                        }

        return evidence_map, weak_facets

    async def _search_by_queries(
        self,
        outline: list[dict],
        topic: str,
        hyde_variant: tuple[str, str] | None,
    ) -> dict[str, dict]:
        """Legacy search using plain search_queries strings."""
        evidence_map: dict[str, dict] = {}

        # Build list of (section_key, query) pairs
        query_pairs: list[tuple[str, str]] = []
        for section in outline:
            section_key = section.get("key", "")
            queries = section.get("search_queries", [])
            if queries:
                for q in queries:
                    query_pairs.append((section_key, q))
            else:
                query_pairs.append((section_key, topic))

        if not query_pairs:
            query_pairs = [("_global", topic)]

        # Append the cross-lingual HyDE passage once under a synthetic
        # section key so the legacy path matches _search_by_facets' recall.
        if hyde_variant is not None:
            hyde_doc, _ = hyde_variant
            query_pairs.append(("_hyde", hyde_doc))

        for section_key, query in query_pairs:
            try:
                articles = await self._evidence.search_articles(query, limit=5)
            except Exception as exc:
                logger.warning("Gatherer: article search failed", query=query, error=str(exc))
                articles = []

            for a in articles:
                if a.article_id in evidence_map:
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

        return evidence_map
