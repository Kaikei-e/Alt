"""Compressor node — extractive compression of hydrated evidence.

Stage 1 of the 2-stage extraction pipeline (resolve doc).
Sits between hydrator and extractor. Heuristic only — no LLM calls.

Query context priority: query_facets (ADR-667/669) > search_queries > topic.
"""

from __future__ import annotations

from dataclasses import asdict
from typing import TYPE_CHECKING

import structlog

from acolyte.domain.compressed_evidence import compress_article
from acolyte.domain.query_facet import render_query_string

if TYPE_CHECKING:
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)


def _collect_queries_for_articles(
    curated_by_section: dict[str, list[dict]],
    outline: list[dict],
    brief: dict,
) -> dict[str, list[str]]:
    """Build per-article query context. Prioritises query_facets over search_queries."""
    # Build section_key → queries lookup from outline
    section_queries: dict[str, list[str]] = {}
    for section in outline:
        key = section.get("key", "")
        queries: list[str] = []
        # Priority 1: query_facets (ADR-667/669)
        facets = section.get("query_facets", [])
        if facets:
            for facet in facets:
                rendered = render_query_string(facet)
                if rendered:
                    queries.append(rendered)
        # Priority 2: search_queries (legacy)
        if not queries:
            queries = list(section.get("search_queries", []))
        section_queries[key] = queries

    # Map article_id → union of queries from all sections that curated it
    article_queries: dict[str, list[str]] = {}
    for section_key, items in curated_by_section.items():
        sq = section_queries.get(section_key, [])
        for item in items:
            aid = item.get("id", "")
            if aid:
                existing = article_queries.setdefault(aid, [])
                for q in sq:
                    if q not in existing:
                        existing.append(q)

    # Always include topic as fallback context
    topic = brief.get("topic", "")
    if topic:
        for aid in article_queries:
            if topic not in article_queries[aid]:
                article_queries[aid].append(topic)

    return article_queries


class CompressorNode:
    def __init__(self, *, char_budget: int = 1000) -> None:
        self._char_budget = char_budget

    async def __call__(self, state: ReportGenerationState) -> dict:
        hydrated = state.get("hydrated_evidence", {})
        curated_by_section = state.get("curated_by_section", {})
        outline = state.get("outline", [])
        brief = state.get("brief") or state.get("scope") or {}

        article_queries = _collect_queries_for_articles(curated_by_section, outline, brief)

        compressed: dict[str, list[dict]] = {}
        total_original = 0
        total_compressed = 0

        for article_id, body in hydrated.items():
            queries = article_queries.get(article_id, [brief.get("topic", "")])
            spans = compress_article(body, queries, char_budget=self._char_budget)
            compressed[article_id] = [asdict(s) for s in spans]
            total_original += len(body)
            total_compressed += sum(len(s.text) for s in spans)

        ratio = f"{total_compressed / total_original:.0%}" if total_original > 0 else "n/a"

        empty_count = sum(1 for spans in compressed.values() if not spans)
        if empty_count == len(compressed) and compressed:
            logger.warning(
                "All articles returned empty compression — scoring may be broken",
                articles=len(compressed),
            )

        logger.info(
            "Compressor completed",
            articles=len(compressed),
            empty_articles=empty_count,
            original_chars=total_original,
            compressed_chars=total_compressed,
            ratio=ratio,
        )
        return {"compressed_evidence": compressed}
