"""QuoteSelectorNode — selects verbatim quotes per article per section.

Stage 1 of 2-stage extraction pipeline (Issue 2, exec3 Issue 1).
Operates per article x per section for section-conditioned recall.

3-tier degradation:
  1. Primary: heuristic extraction (no LLM) using section-conditioned scoring
  2. Secondary: LLM structured output (only when heuristic returns empty)
  3. Tertiary: deterministic sentence fallback (position-based, always produces output)
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.domain.compressed_evidence import select_top_sentences
from acolyte.domain.query_facet import render_query_string
from acolyte.domain.quote_selection import QuoteSelectorOutput, SelectedQuote
from acolyte.port.llm_provider import LLMMode
from acolyte.usecase.graph.llm_parse import generate_validated
from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)

_QUOTE_PROMPT = """Select 1-3 verbatim quotes from this article relevant to the section topic.
Each quote must be an exact substring of the article text.

Section topic: {section_queries}
Article ID: {source_id}
Article Title: {source_title}
Article Body:
{body}

Return JSON: {{"reasoning": "...", "quotes": [{{"text": "exact substring", "source_id": "{source_id}", "source_title": "{source_title}"}}]}}"""


def _resolve_queries_for_section(section: dict) -> list[str]:
    """Resolve section queries: query_facets > search_queries > section key."""
    # Priority 1: query_facets (ADR-667/669)
    facets = section.get("query_facets", [])
    if facets:
        queries = []
        for facet in facets:
            rendered = render_query_string(facet)
            if rendered:
                queries.append(rendered)
        if queries:
            return queries

    # Priority 2: search_queries (legacy)
    search_queries = section.get("search_queries", [])
    if search_queries:
        return list(search_queries)

    # Priority 3: section key as fallback
    key = section.get("key", "")
    return [key] if key else []


def _resolve_raw_body(
    item_id: str,
    hydrated: dict[str, str],
) -> str:
    """Resolve raw article body from hydrated evidence (for offset verification)."""
    return hydrated.get(item_id, "")


def _resolve_prompt_body(
    item_id: str,
    compressed: dict[str, list[dict]],
    hydrated: dict[str, str],
) -> str:
    """Resolve body for LLM prompt: compressed > truncated hydrated."""
    if item_id in compressed:
        spans = compressed[item_id]
        if spans:
            return "\n".join(s["text"] for s in spans)

    body = hydrated.get(item_id, "")
    if len(body) > 2000:
        body = body[:2000]
    return body


def _spans_to_quotes(
    spans: list,
    raw_body: str,
    source_id: str,
    source_title: str,
    section_key: str,
) -> list[dict]:
    """Convert CompressedSpan list to SelectedQuote dicts with raw-body offset verification."""
    quotes: list[dict] = []
    for span in spans:
        text = span.text
        # Verify offset against raw body
        offset = raw_body.find(text)
        quotes.append(
            SelectedQuote(
                text=text,
                source_id=source_id,
                source_title=source_title,
                section_key=section_key,
                start_offset=offset,
                end_offset=offset + len(text) if offset >= 0 else -1,
            ).model_dump()
        )
    return quotes


def should_continue_quote_selection(state: ReportGenerationState) -> str:
    """Conditional edge for checkpoint-safe per-article quote selection."""
    work_items = state.get("quote_selector_work_items", [])
    cursor = state.get("quote_selector_cursor", 0)
    return "more" if cursor < len(work_items) else "done"


class QuoteSelectorNode:
    def __init__(self, llm: LLMProviderPort, *, incremental: bool = False) -> None:
        self._llm = llm
        self._incremental = incremental

    async def __call__(self, state: ReportGenerationState) -> dict:
        if self._incremental:
            return await self._process_incremental(state)
        return await self._process_all(state)

    async def _process_all(self, state: ReportGenerationState) -> dict:
        curated_by_section = state.get("curated_by_section", {})
        hydrated = state.get("hydrated_evidence", {})
        compressed = state.get("compressed_evidence", {})
        outline = state.get("outline", [])

        # Build section query lookup: section_key → list[str]
        section_queries_map: dict[str, list[str]] = {}
        for section in outline:
            key = section.get("key", "")
            section_queries_map[key] = _resolve_queries_for_section(section)

        all_quotes: list[dict] = []

        for section_key, items in curated_by_section.items():
            query_list = section_queries_map.get(section_key, [section_key])

            for item in items:
                item_id = item.get("id", "")
                title = item.get("title", "")

                raw_body = _resolve_raw_body(item_id, hydrated)
                if not raw_body:
                    continue

                quotes = await self._select_quotes(
                    item_id,
                    title,
                    raw_body,
                    section_key,
                    query_list,
                    compressed,
                    hydrated,
                )
                all_quotes.extend(quotes)

        logger.info(
            "QuoteSelector completed",
            quote_count=len(all_quotes),
            sections=len(curated_by_section),
        )
        return {"selected_quotes": all_quotes}

    async def _process_incremental(self, state: ReportGenerationState) -> dict:
        """Checkpoint-safe path: process one section/article pair per invocation."""
        hydrated = state.get("hydrated_evidence", {})
        compressed = state.get("compressed_evidence", {})
        selected_quotes = list(state.get("selected_quotes", []))

        work_items = state.get("quote_selector_work_items")
        if work_items is None:
            work_items = self._build_work_items(state)

        cursor = state.get("quote_selector_cursor", 0)
        if cursor >= len(work_items):
            logger.info("QuoteSelector completed", quote_count=len(selected_quotes), sections=len(work_items))
            return {
                "selected_quotes": selected_quotes,
                "quote_selector_work_items": work_items,
                "quote_selector_cursor": cursor,
            }

        item = work_items[cursor]
        raw_body = _resolve_raw_body(item["source_id"], hydrated)
        if raw_body:
            quotes = await self._select_quotes(
                item["source_id"],
                item["source_title"],
                raw_body,
                item["section_key"],
                item["section_queries"],
                compressed,
                hydrated,
            )
            selected_quotes.extend(quotes)

        next_cursor = cursor + 1
        if next_cursor >= len(work_items):
            logger.info("QuoteSelector completed", quote_count=len(selected_quotes), sections=len(work_items))
        else:
            logger.info(
                "QuoteSelector progress",
                processed=next_cursor,
                total=len(work_items),
            )

        return {
            "selected_quotes": selected_quotes,
            "quote_selector_work_items": work_items,
            "quote_selector_cursor": next_cursor,
        }

    def _build_work_items(self, state: ReportGenerationState) -> list[dict]:
        curated_by_section = state.get("curated_by_section", {})
        outline = state.get("outline", [])

        section_queries_map: dict[str, list[str]] = {}
        for section in outline:
            key = section.get("key", "")
            section_queries_map[key] = _resolve_queries_for_section(section)

        work_items: list[dict] = []
        for section_key, items in curated_by_section.items():
            query_list = section_queries_map.get(section_key, [section_key])
            for item in items:
                work_items.append(
                    {
                        "section_key": section_key,
                        "section_queries": list(query_list),
                        "source_id": item.get("id", ""),
                        "source_title": item.get("title", ""),
                    }
                )
        return work_items

    async def _select_quotes(
        self,
        source_id: str,
        source_title: str,
        raw_body: str,
        section_key: str,
        section_queries: list[str],
        compressed: dict[str, list[dict]],
        hydrated: dict[str, str],
    ) -> list[dict]:
        """Select quotes: heuristic primary -> LLM secondary -> sentence fallback."""
        # Primary: deterministic heuristic (section-conditioned, raw body)
        try:
            spans = select_top_sentences(
                raw_body,
                section_queries,
                max_sentences=3,
                max_len=200,
                position_fallback=False,
            )
            if spans:
                quotes = _spans_to_quotes(spans, raw_body, source_id, source_title, section_key)
                logger.debug(
                    "QuoteSelector heuristic succeeded",
                    article_id=source_id,
                    quote_count=len(quotes),
                )
                return quotes
        except Exception as exc:
            logger.warning(
                "QuoteSelector heuristic failed, trying LLM",
                article_id=source_id,
                error=str(exc),
            )

        # Secondary: LLM (only when heuristic returns empty)
        try:
            prompt_body = _resolve_prompt_body(source_id, compressed, hydrated)
            query_context_str = ", ".join(section_queries)
            prompt = _QUOTE_PROMPT.format(
                section_queries=query_context_str,
                source_id=source_id,
                source_title=source_title,
                body=prompt_body,
            )

            fallback = QuoteSelectorOutput(reasoning="fallback", quotes=[])
            result = await generate_validated(
                self._llm,
                prompt,
                QuoteSelectorOutput,
                temperature=0,
                num_predict=768,
                fallback=fallback,
                mode=LLMMode.STRUCTURED,
            )

            if result.quotes:
                quotes = []
                for q in result.quotes:
                    d = q.model_dump()
                    d["section_key"] = section_key
                    # Verify offsets against raw body
                    text = d.get("text", "")
                    offset = raw_body.find(text)
                    d["start_offset"] = offset
                    d["end_offset"] = offset + len(text) if offset >= 0 else -1
                    quotes.append(d)
                return quotes

            logger.warning(
                "QuoteSelector LLM returned empty",
                article_id=source_id,
            )
        except Exception as exc:
            logger.warning(
                "QuoteSelector LLM failed, using sentence fallback",
                article_id=source_id,
                error=str(exc),
            )

        # Tertiary: deterministic sentence fallback (position-based, always produces)
        return self._sentence_fallback(raw_body, source_id, source_title, section_key, section_queries)

    def _sentence_fallback(
        self,
        raw_body: str,
        source_id: str,
        source_title: str,
        section_key: str,
        section_queries: list[str],
    ) -> list[dict]:
        """Tertiary fallback: position-based sentence selection using shared splitter."""
        spans = select_top_sentences(
            raw_body,
            section_queries,
            max_sentences=3,
            max_len=200,
            position_fallback=True,
        )
        return _spans_to_quotes(spans, raw_body, source_id, source_title, section_key)
